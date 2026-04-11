package agent

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/meistro57/kae/internal/config"
	"github.com/meistro57/kae/internal/embeddings"
	"github.com/meistro57/kae/internal/ensemble"
	"github.com/meistro57/kae/internal/graph"
	"github.com/meistro57/kae/internal/ingestion"
	"github.com/meistro57/kae/internal/llm"
	"github.com/meistro57/kae/internal/runcontrol"
	"github.com/meistro57/kae/internal/scoring"
	"github.com/meistro57/kae/internal/store"
)

type Phase string

const (
	PhaseIdle      Phase = "IDLE"
	PhaseSeed      Phase = "SEEDING"
	PhaseIngest    Phase = "INGESTING"
	PhaseEmbed     Phase = "EMBEDDING"
	PhaseSearch    Phase = "SEARCHING MEMORY"
	PhaseThink     Phase = "THINKING"
	PhaseConnect   Phase = "CONNECTING"
	PhaseScore     Phase = "SCORING CONTRADICTIONS"
	PhaseAnomaly   Phase = "ANOMALY SCAN"
	PhasePlan      Phase = "PLANNING"
	PhaseReport    Phase = "UPDATING REPORT"
	PhaseStable    Phase = "GRAPH STABLE"
	PhaseEnsemble  Phase = "ENSEMBLE REASONING"
)

type Event struct {
	Phase       Phase
	Focus       string
	ThinkChunk  string
	OutputChunk string
	GraphSnap   Snapshot
	ReportLine  string
	Err         error
}

type Snapshot struct {
	Nodes         int
	Edges         int
	Anomalies     int
	Sources       int
	Cycles        int
	TopNodes      []string
	TopWeights    []float64
	QdrantOK      bool
	QdrantVectors int
}

type Engine struct {
	cfg      *config.Config
	graph    *graph.Graph
	brain    llm.Provider
	fast     llm.Provider
	ens      *ensemble.Ensemble // nil when ensemble mode is off
	ctrl     *runcontrol.RunController
	qdrant   *store.Client
	embedder *embeddings.Embedder
	events   chan Event
	runID    string

	mu            sync.Mutex
	cycle         int
	sources       int
	focus         string
	report        strings.Builder
	lastThink     strings.Builder
	running       bool
	prevNodeCount int
}

func NewEngine(cfg *config.Config) *Engine {
	runID := fmt.Sprintf("run_%d", time.Now().Unix())

	keys := llm.ProviderKeys{
		OpenRouterKey: cfg.OpenRouterKey,
		AnthropicKey:  cfg.AnthropicKey,
		OpenAIKey:     cfg.OpenAIKey,
		GeminiKey:     cfg.GeminiKey,
		OllamaURL:     cfg.OllamaURL,
	}

	brain, err := llm.NewProvider(cfg.Model, keys)
	if err != nil {
		// Fallback: try OpenRouter with the bare model string
		brain = llm.NewClient(cfg.OpenRouterKey, cfg.Model)
	}

	fast, err := llm.NewProvider(cfg.FastModel, keys)
	if err != nil {
		fast = llm.NewClient(cfg.OpenRouterKey, cfg.FastModel)
	}

	// Build ensemble if requested
	var ens *ensemble.Ensemble
	if cfg.EnsembleMode && len(cfg.EnsembleModels) > 0 {
		var providers []llm.Provider
		for _, pm := range cfg.EnsembleModels {
			p, err := llm.NewProvider(pm, keys)
			if err == nil {
				providers = append(providers, p)
			}
		}
		if len(providers) > 0 {
			ens = ensemble.New(providers)
		}
	}

	ctrl := runcontrol.New(
		cfg.NoveltyThreshold,
		cfg.StagnationWindow,
		cfg.BranchThreshold,
		cfg.MaxBranches,
	)

	return &Engine{
		cfg:      cfg,
		graph:    graph.New(),
		brain:    brain,
		fast:     fast,
		ens:      ens,
		ctrl:     ctrl,
		qdrant:   store.NewClient(cfg.QdrantURL),
		embedder: embeddings.NewEmbedder(cfg.OpenRouterKey),
		events:   make(chan Event, 256),
		runID:    runID,
	}
}

func (e *Engine) Events() <-chan Event { return e.events }
func (e *Engine) RunID() string        { return e.runID }
func (e *Engine) SaveGraph(path string) error {
	return e.graph.SaveToFile(path)
}

func (e *Engine) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.mu.Unlock()
	go e.run()
}

func (e *Engine) run() {
	if path := strings.TrimSpace(e.cfg.ResumeGraphPath); path != "" {
		if err := e.graph.LoadFromFile(path); err != nil {
			e.emit(Event{Err: fmt.Errorf("resume graph: %w", err)})
		} else {
			e.emit(Event{Phase: PhaseIdle, Focus: fmt.Sprintf("Loaded graph: %s", path)})
		}
	}

	e.emit(Event{Phase: PhaseIdle, Focus: "Initialising memory..."})
	if err := e.qdrant.EnsureCollections(); err != nil {
		e.emit(Event{Err: fmt.Errorf("qdrant init: %w", err)})
	}

	e.emit(Event{Phase: PhaseSeed, Focus: "Choosing entry point..."})
	focus := e.cfg.Seed
	if focus == "" {
		focus = e.chooseSeed()
	}
	e.focus = focus

	for {
		e.mu.Lock()
		cycle := e.cycle
		maxCycles := e.cfg.MaxCycles
		e.mu.Unlock()

		if maxCycles > 0 && cycle >= maxCycles {
			e.emit(Event{Phase: PhaseStable, Focus: "Max cycles reached"})
			break
		}

		e.mu.Lock()
		e.cycle++
		nodeBefore := e.graph.NodeCount()
		e.mu.Unlock()

		focus = e.runCycle(focus)

		// Novelty tracking — stop if graph has stagnated
		nodeAfter := e.graph.NodeCount()
		newNodes := nodeAfter - nodeBefore
		if !e.ctrl.RecordNovelty(newNodes, nodeAfter) {
			e.emit(Event{
				Phase: PhaseStable,
				Focus: fmt.Sprintf("Graph stagnated (%d consecutive low-novelty cycles)",
					e.ctrl.StagnantCycles()),
			})
			break
		}

		time.Sleep(200 * time.Millisecond)
	}
}

func (e *Engine) runCycle(topic string) string {
	chunks := e.ingestPhase(topic)
	e.embedPhase(topic, chunks)
	candidates := e.searchPhase(topic)
	next := e.thinkPhase(topic, candidates)
	e.scorePhase(topic, candidates)
	e.anomalyPhase()
	e.reportPhase()
	return next
}

// ── Phase implementations ─────────────────────────────────────────────────────

func (e *Engine) ingestPhase(topic string) []ingestion.SourceChunk {
	e.emit(Event{Phase: PhaseIngest, Focus: topic})

	var allChunks []ingestion.SourceChunk
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Wikipedia
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, err := ingestion.WikiSummary(topic)
		if err != nil {
			e.emit(Event{Phase: PhaseIngest, OutputChunk: fmt.Sprintf("Wiki: %v", err)})
			return
		}
		chunks := ingestion.Chunk(result.Extract, 200, 30)
		sc := make([]ingestion.SourceChunk, len(chunks))
		for i, c := range chunks {
			sc[i] = ingestion.SourceChunk{Text: c, Source: result.URL, Topic: topic}
		}
		mu.Lock()
		allChunks = append(allChunks, sc...)
		mu.Unlock()
		e.emit(Event{Phase: PhaseIngest,
			OutputChunk: fmt.Sprintf("✓ Wikipedia: %d chunks", len(sc))})
	}()

	// arxiv
	wg.Add(1)
	go func() {
		defer wg.Done()
		papers, err := ingestion.ArxivSearchMulti(topic, ingestion.KAEArxivCategories, 1)
		if err != nil {
			return
		}
		var sc []ingestion.SourceChunk
		for _, p := range papers {
			for _, c := range ingestion.PaperToChunks(p) {
				sc = append(sc, ingestion.SourceChunk{Text: c, Source: p.URL, Topic: topic})
			}
		}
		mu.Lock()
		allChunks = append(allChunks, sc...)
		mu.Unlock()
		e.emit(Event{Phase: PhaseIngest,
			OutputChunk: fmt.Sprintf("✓ arxiv: %d chunks from %d papers", len(sc), len(papers))})
	}()

	// Gutenberg — ancient texts
	wg.Add(1)
	go func() {
		defer wg.Done()
		relevant := ingestion.BooksForTopic(topic)
		var books []*ingestion.GutenbergBook
		for _, b := range relevant {
			book, err := ingestion.GutenbergFetch(b.ID, b.Title)
			if err == nil {
				books = append(books, book)
			}
		}
		if len(books) == 0 {
			return
		}
		var sc []ingestion.SourceChunk
		for _, book := range books {
			text, err := ingestion.FetchBookText(book, 2000)
			if err != nil {
				continue
			}
			for _, c := range ingestion.BookToChunks(book, text) {
				sc = append(sc, ingestion.SourceChunk{Text: c, Source: book.Title, Topic: topic})
			}
		}
		mu.Lock()
		allChunks = append(allChunks, sc...)
		mu.Unlock()
		e.emit(Event{Phase: PhaseIngest,
			OutputChunk: fmt.Sprintf("✓ Gutenberg: %d chunks from %d texts", len(sc), len(books))})
	}()

	wg.Wait()

	e.mu.Lock()
	e.sources += len(allChunks)
	e.mu.Unlock()

	return allChunks
}

func (e *Engine) embedPhase(topic string, chunks []ingestion.SourceChunk) {
	e.emit(Event{Phase: PhaseEmbed, Focus: topic,
		OutputChunk: fmt.Sprintf("Embedding %d chunks...", len(chunks))})

	batchSize := 20
	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		texts := make([]string, len(batch))
		for j, c := range batch {
			texts[j] = c.Text
		}

		vecs, err := e.embedder.EmbedBatch(texts)
		if err != nil {
			e.emit(Event{Phase: PhaseEmbed, OutputChunk: fmt.Sprintf("Embed error: %v", err)})
			continue
		}

		for j, vec := range vecs {
			chunkID := fmt.Sprintf("%s_%s_%d", e.runID, slugify(topic), i+j)
			_ = e.qdrant.StoreChunk(&store.Chunk{
				ID:     chunkID,
				Text:   batch[j].Text,
				Source: batch[j].Source,
				Topic:  topic,
				RunID:  e.runID,
				Vector: vec,
			})
		}
	}

	topicVec, err := e.embedder.Embed(topic)
	if err == nil {
		e.graph.UpsertNode(&graph.Node{
			ID:     slugify(topic),
			Label:  topic,
			Domain: "ingested",
			Weight: 1.0,
			Vector: topicVec,
		})
		_ = e.qdrant.StoreNode(&store.NodeRecord{
			ID:     fmt.Sprintf("%s_%s", e.runID, slugify(topic)),
			Label:  topic,
			Domain: "ingested",
			RunID:  e.runID,
			Weight: 1.0,
			Cycle:  e.cycle,
			Vector: topicVec,
		})
	}
}

func (e *Engine) searchPhase(topic string) []*store.Chunk {
	e.emit(Event{Phase: PhaseSearch, Focus: topic})

	topicVec, err := e.embedder.Embed(topic)
	if err != nil {
		e.emit(Event{Phase: PhaseSearch, OutputChunk: fmt.Sprintf("Search embed error: %v", err)})
		return nil
	}

	var chunkFilter map[string]any
	if !e.cfg.SharedMemory {
		chunkFilter = map[string]any{
			"must": []map[string]any{
				{"key": "run_id", "match": map[string]any{"value": e.runID}},
			},
		}
	}
	candidates, err := e.qdrant.SearchChunks(topicVec, 10, chunkFilter)
	if err != nil {
		e.emit(Event{Phase: PhaseSearch, OutputChunk: fmt.Sprintf("Search error: %v", err)})
		return nil
	}

	e.emit(Event{Phase: PhaseSearch,
		OutputChunk: fmt.Sprintf("Found %d candidate passages from memory", len(candidates))})

	return candidates
}

func (e *Engine) thinkPhase(topic string, candidates []*store.Chunk) string {
	system := `You are an unbiased knowledge archaeologist reasoning over real source material.
You DO NOT rely on your training data. You reason ONLY from the source passages provided.
Your job is to find what the evidence actually says — not what consensus expects it to say.
Follow contradictions. Flag silence. Make connections the sources themselves don't make.`

	var contextBuilder strings.Builder
	contextBuilder.WriteString("SOURCE PASSAGES (from real ingested documents):\n\n")
	for i, c := range candidates {
		if i >= 8 {
			break
		}
		contextBuilder.WriteString(fmt.Sprintf("--- Source: %s ---\n%s\n\n", c.Source, c.Text))
	}

	msgs := []llm.Message{{
		Role: "user",
		Content: fmt.Sprintf(`Current focus: "%s"
Graph state: %s

%s

Based ONLY on the source passages above:

1. What are the 3 most important cross-domain connections you see?
2. Where do these sources contradict each other or go silent?
3. What would a naive observer conclude that mainstream researchers avoid saying?
4. What single concept should we investigate next?

Format:
CONNECTIONS: <concept1> | <concept2> | <concept3>
CONTRADICTION: <what the sources disagree on>
ANOMALY: <what mainstream avoids saying>
NAIVE_CONCLUSION: <what the evidence actually points to>
NEXT: <single next concept>`,
			topic,
			e.graph.CleanSummary(),
			contextBuilder.String(),
		),
	}}

	// Use ensemble when available; fall back to single brain.
	var response string
	if e.ens != nil {
		e.emit(Event{Phase: PhaseEnsemble, Focus: topic})
		result := e.ens.Run(system, msgs)
		response = result.Merged

		// High controversy → flag the topic as an anomaly candidate
		if result.Controversy > 0.5 {
			e.emit(Event{
				Phase:       PhaseEnsemble,
				Focus:       topic,
				OutputChunk: fmt.Sprintf("⚡ High controversy (%.2f) — %d models disagree", result.Controversy, len(result.Responses)),
			})
			e.graph.UpsertNode(&graph.Node{
				ID:      slugify(topic) + "_controversy",
				Label:   fmt.Sprintf("[ENSEMBLE CONTROVERSY] %s", topic),
				Domain:  "anomaly",
				Weight:  2.0 + result.Controversy,
				Anomaly: true,
				Notes: fmt.Sprintf("Controversy: %.2f\nDissenters: %s",
					result.Controversy, strings.Join(result.Dissenting, ", ")),
			})
		}

		// Check run controller for branching
		if e.ctrl.ShouldBranch(result.Controversy) {
			e.ctrl.RecordBranch()
			e.emit(Event{
				Phase:       PhaseEnsemble,
				Focus:       topic,
				OutputChunk: fmt.Sprintf("🌿 Branch triggered (controversy=%.2f)", result.Controversy),
			})
		}
	} else {
		e.emit(Event{Phase: PhaseThink, Focus: topic})
		ch := e.brain.Stream(system, msgs)
		var output strings.Builder
		for chunk := range ch {
			switch chunk.Type {
			case llm.ChunkThink:
				e.lastThink.WriteString(chunk.Text)
				e.emit(Event{Phase: PhaseThink, Focus: topic, ThinkChunk: chunk.Text})
			case llm.ChunkText:
				output.WriteString(chunk.Text)
				e.emit(Event{Phase: PhaseConnect, Focus: topic, OutputChunk: chunk.Text})
			case llm.ChunkError:
				e.emit(Event{Err: chunk.Err})
			}
		}
		response = output.String()
	}

	connections, contradiction, anomaly, naiveConclusion, next := parseResponse(response)

	for _, conn := range connections {
		conn = strings.TrimSpace(conn)
		if conn == "" {
			continue
		}
		connVec, _ := e.embedder.Embed(conn)
		e.graph.UpsertNode(&graph.Node{
			ID:     slugify(conn),
			Label:  conn,
			Domain: "inferred",
			Weight: 0.5,
			Vector: connVec,
		})
		e.graph.AddEdge(&graph.Edge{
			From:       slugify(topic),
			To:         slugify(conn),
			Relation:   "connects_to",
			Confidence: 0.7,
			Citation:   buildCitation(candidates),
		})
	}

	if contradiction != "" || anomaly != "" || naiveConclusion != "" {
		e.graph.UpsertNode(&graph.Node{
			ID:      slugify(topic) + "_anomaly",
			Label:   fmt.Sprintf("[ANOMALY] %s", topic),
			Domain:  "anomaly",
			Weight:  2.0,
			Anomaly: true,
			Notes:   fmt.Sprintf("Contradiction: %s\nAnomaly: %s\nConclusion: %s", contradiction, anomaly, naiveConclusion),
		})
	}

	if next == "" {
		next = topic
	}
	return next
}

func (e *Engine) scorePhase(topic string, candidates []*store.Chunk) {
	e.emit(Event{Phase: PhaseScore, Focus: topic})

	if len(candidates) == 0 {
		return
	}

	evidence := make([]scoring.Evidence, 0, len(candidates))
	for _, c := range candidates {
		stance := scoring.ClassifyStance(topic, c.Text)
		evidence = append(evidence, scoring.Evidence{
			Source:  c.Source,
			Stance:  stance,
			Excerpt: c.Text[:min(200, len(c.Text))],
			Weight:  1.0,
		})
	}

	score := scoring.Score(topic, evidence)

	e.graph.UpsertNode(&graph.Node{
		ID:                 slugify(topic),
		Label:              topic,
		Domain:             "scored",
		Weight:             score.AnomalyScore * 3,
		Anomaly:            score.IsAnomaly,
		ContradictionScore: score,
	})

	e.emit(Event{Phase: PhaseScore,
		OutputChunk: fmt.Sprintf("Score for %q: %s", topic, score.Explanation)})
}

func (e *Engine) anomalyPhase() {
	e.emit(Event{Phase: PhaseAnomaly, Focus: "Scanning..."})
	anomalies := e.graph.AnomalyNodes()
	if len(anomalies) > 0 {
		labels := make([]string, 0, len(anomalies))
		for _, a := range anomalies {
			labels = append(labels, a.Label)
		}
		e.emit(Event{Phase: PhaseAnomaly,
			OutputChunk: fmt.Sprintf("⚠ %d anomaly nodes: %s",
				len(anomalies), strings.Join(labels[:min(3, len(labels))], ", "))})
	}
}

func (e *Engine) reportPhase() {
	e.emit(Event{Phase: PhaseReport})
	top := e.graph.TopNodes(5)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n## Cycle %d — %s\n", e.cycle, time.Now().Format("15:04:05")))
	sb.WriteString(fmt.Sprintf("**Graph:** %s\n\n", e.graph.CleanSummary()))

	if think := e.lastThink.String(); think != "" {
		sb.WriteString("**Thinking:**\n")
		sb.WriteString(think)
		sb.WriteString("\n\n")
		e.lastThink.Reset()
	}

	sb.WriteString("**Emergent concepts:**\n")
	for _, n := range top {
		flag := ""
		if n.Anomaly {
			flag = " ⚠"
		}
		score := ""
		if n.ContradictionScore != nil {
			score = fmt.Sprintf(" [anomaly: %.2f]", n.ContradictionScore.AnomalyScore)
		}
		sb.WriteString(fmt.Sprintf("- %s (weight: %.1f)%s%s\n", n.Label, n.Weight, score, flag))
	}
	e.syncNodesToQdrant()
	e.mu.Lock()
	e.report.WriteString(sb.String())
	e.mu.Unlock()

	e.emit(Event{Phase: PhaseReport, ReportLine: sb.String()})
}

// ── Seed selection ────────────────────────────────────────────────────────────

func (e *Engine) chooseSeed() string {
	msgs := []llm.Message{{
		Role: "user",
		Content: `You are starting a completely unbiased knowledge archaeology run.
Choose ONE foundational concept that, if followed without bias, would force
reconciliation of the deepest contradictions in human knowledge across science,
philosophy, consciousness research, and ancient wisdom.
Return ONLY the concept name, nothing else.`,
	}}
	result := e.collectStream(e.brain, "", msgs)
	seed := strings.TrimSpace(result)
	if seed == "" {
		seed = "consciousness"
	}
	return seed
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (e *Engine) emit(ev Event) {
	ev.GraphSnap = e.snapshot()
	select {
	case e.events <- ev:
	default:
	}
}

func (e *Engine) snapshot() Snapshot {
	e.mu.Lock()
	cycles := e.cycle
	sources := e.sources
	e.mu.Unlock()

	top := e.graph.TopNodes(5)
	labels := make([]string, len(top))
	for i, n := range top {
		labels[i] = n.Label
	}
	weights := make([]float64, len(top))
	for i, n := range top {
		weights[i] = n.Weight
	}
	return Snapshot{
		Nodes:         e.graph.NodeCount(),
		Edges:         e.graph.EdgeCount(),
		Anomalies:     e.graph.AnomalyCount(),
		Sources:       sources,
		Cycles:        cycles,
		TopNodes:      labels,
		TopWeights:    weights,
		QdrantOK:      true,
		QdrantVectors: e.sources,
	}
}

func (e *Engine) collectStream(client llm.Provider, system string, msgs []llm.Message) string {
	var sb strings.Builder
	for chunk := range client.Stream(system, msgs) {
		if chunk.Type == llm.ChunkText {
			sb.WriteString(chunk.Text)
		}
	}
	return sb.String()
}

func parseResponse(s string) (connections []string, contradiction, anomaly, naive, next string) {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "CONNECTIONS:"):
			for _, p := range strings.Split(strings.TrimPrefix(line, "CONNECTIONS:"), "|") {
				connections = append(connections, strings.TrimSpace(p))
			}
		case strings.HasPrefix(line, "CONTRADICTION:"):
			contradiction = strings.TrimSpace(strings.TrimPrefix(line, "CONTRADICTION:"))
		case strings.HasPrefix(line, "ANOMALY:"):
			anomaly = strings.TrimSpace(strings.TrimPrefix(line, "ANOMALY:"))
		case strings.HasPrefix(line, "NAIVE_CONCLUSION:"):
			naive = strings.TrimSpace(strings.TrimPrefix(line, "NAIVE_CONCLUSION:"))
		case strings.HasPrefix(line, "NEXT:"):
			next = strings.TrimSpace(strings.TrimPrefix(line, "NEXT:"))
		}
	}
	return
}

func buildCitation(chunks []*store.Chunk) string {
	seen := make(map[string]bool)
	var sources []string
	for _, c := range chunks {
		if !seen[c.Source] {
			seen[c.Source] = true
			sources = append(sources, c.Source)
		}
	}
	return strings.Join(sources, "; ")
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return -1
	}, s)
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (e *Engine) MaxCycles() int {
	return e.cfg.MaxCycles
}

func (e *Engine) Report() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.report.String()
}

func (e *Engine) Focus() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.focus
}

func (e *Engine) syncNodesToQdrant() {
	nodes := e.graph.AllNodes()
	for _, n := range nodes {
		if len(n.Vector) == 0 {
			vec, err := e.embedder.Embed(n.Label)
			if err != nil {
				continue
			}
			n.Vector = vec
		}
		_ = e.qdrant.StoreNode(&store.NodeRecord{
			ID:      fmt.Sprintf("%s_%s", e.runID, n.ID),
			Label:   n.Label,
			Domain:  n.Domain,
			RunID:   e.runID,
			Weight:  n.Weight,
			Anomaly: n.Anomaly,
			Cycle:   e.cycle,
			Vector:  n.Vector,
		})
	}
}

// cleanThink strips R1 meta-talk from think blocks before writing to report.
func cleanThink(s string) string {
	junkPhrases := []string{
		"FINALLY, WRITE THE RESPONSE",
		"NOW, FORMAT THE RESPONSE",
		"I SHOULD ENSURE THAT MY ANSWERS",
		"LET ME DOUBLE-CHECK",
		"FORMAT YOUR ANSWER AS",
		"WRITE THE RESPONSE IN THE SPECIFIED FORMAT",
		"CONNECTIONS: <concept",
		"CONTRADICTION: <what",
		"ANOMALY: <what",
		"NAIVE_CONCLUSION: <what",
		"NEXT: <single",
	}
	lines := strings.Split(s, "\n")
	var clean []string
	for _, line := range lines {
		upper := strings.ToUpper(strings.TrimSpace(line))
		skip := false
		for _, phrase := range junkPhrases {
			if strings.Contains(upper, strings.ToUpper(phrase)) {
				skip = true
				break
			}
		}
		if !skip {
			clean = append(clean, line)
		}
	}
	return strings.TrimSpace(strings.Join(clean, "\n"))
}
