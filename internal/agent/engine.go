package agent

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/meistro/kae/internal/config"
	"github.com/meistro/kae/internal/embeddings"
	"github.com/meistro/kae/internal/graph"
	"github.com/meistro/kae/internal/ingestion"
	"github.com/meistro/kae/internal/llm"
	"github.com/meistro/kae/internal/store"
)



// Phase tracks what the agent is currently doing
type Phase string

const (
	PhaseIdle     Phase = "IDLE"
	PhaseSeed     Phase = "SEEDING"
	PhaseIngest   Phase = "INGESTING"
	PhaseThink    Phase = "THINKING"
	PhaseConnect  Phase = "CONNECTING"
	PhaseAnomaly  Phase = "ANOMALY SCAN"
	PhasePlan     Phase = "PLANNING"
	PhaseReport   Phase = "UPDATING REPORT"
	PhaseStable   Phase = "GRAPH STABLE"
)

// Event is sent to the UI to communicate state changes
type Event struct {
	Phase       Phase
	Focus       string   // current topic
	ThinkChunk  string   // R1 thinking text (streamed)
	OutputChunk string   // R1 response text (streamed)
	GraphSnap   Snapshot // graph stats
	ReportLine  string   // new line added to report
	Err         error
}

// Snapshot holds a point-in-time view of the graph for the UI
type Snapshot struct {
	Nodes     int
	Edges     int
	Anomalies int
	Sources   int
	Cycles    int
	TopNodes   []string
	TopWeights []float64
	// Qdrant
	QdrantOK      bool
	QdrantVectors int64
}

// Engine runs the knowledge archaeology loop
type Engine struct {
	cfg    *config.Config
	graph  *graph.Graph
	brain  *llm.Client // R1 — deep thinking
	fast   *llm.Client // Gemini Flash — bulk passes
	qdrant *store.Client
	events chan Event

	mu            sync.Mutex
	cycle         int
	sources       int
	focus         string
	report        strings.Builder
	running       bool
	qdrantOK      bool
	qdrantVectors int64

	thinkFile *os.File
	thinkW    *bufio.Writer
}

func NewEngine(cfg *config.Config) *Engine {
	e := &Engine{
		cfg:    cfg,
		graph:  graph.New(),
		brain:  llm.NewClient(cfg.OpenRouterKey, cfg.Model),
		fast:   llm.NewClient(cfg.OpenRouterKey, cfg.FastModel),
		qdrant: store.NewClient(cfg.QdrantURL),
		events: make(chan Event, 256),
	}
	// best-effort: init Qdrant collection
	go func() {
		if err := e.qdrant.Ping(); err != nil {
			return
		}
		_ = e.qdrant.EnsureCollection(store.Collection, embeddings.Dim)
		e.mu.Lock()
		e.qdrantOK = true
		e.mu.Unlock()
		e.refreshQdrantStats()
	}()
	return e
}

// Events returns the channel the UI reads from
func (e *Engine) Events() <-chan Event   { return e.events }
func (e *Engine) MaxCycles() int        { return e.cfg.MaxCycles }
func (e *Engine) Report() string        { e.mu.Lock(); defer e.mu.Unlock(); return e.report.String() }
func (e *Engine) Focus() string         { e.mu.Lock(); defer e.mu.Unlock(); return e.focus }

// ThinkLogPath returns the path of the live think log file (empty if not open).
func (e *Engine) ThinkLogPath() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.thinkFile == nil {
		return ""
	}
	return e.thinkFile.Name()
}

// Start kicks off the agent loop in a goroutine
func (e *Engine) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.mu.Unlock()

	// open the live think log — named now so the UI can show the path
	slug := slugify(e.cfg.Seed)
	if slug == "" {
		slug = "kae"
	}
	logPath := fmt.Sprintf("think_%s_%s.log", slug, time.Now().Format("20060102_150405"))
	f, err := os.Create(logPath)
	if err == nil {
		e.mu.Lock()
		e.thinkFile = f
		e.thinkW = bufio.NewWriter(f)
		e.mu.Unlock()
	}

	go e.run()
}

func (e *Engine) run() {
	e.emit(Event{Phase: PhaseSeed, Focus: "Choosing entry point..."})

	// Phase 0: Seed — let the agent pick its own starting concept
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
		currentCycle := e.cycle
		e.mu.Unlock()

		e.writeThinkHeader(currentCycle, focus)

		// Ingest → Think → Connect → Anomaly → Plan → Report
		content := e.ingestPhase(focus)
		focus = e.thinkAndConnectPhase(focus, content)
		e.anomalyPhase()
		e.reportPhase()

		snap := e.snapshot()
		e.emit(Event{Phase: PhasePlan, Focus: focus, GraphSnap: snap})

		// Tiny breath between cycles so the UI can render
		time.Sleep(200 * time.Millisecond)
	}

	// flush and close the think log
	e.mu.Lock()
	if e.thinkW != nil {
		e.thinkW.Flush()
	}
	if e.thinkFile != nil {
		e.thinkFile.Close()
	}
	e.mu.Unlock()
}

func (e *Engine) chooseSeed() string {
	e.emit(Event{Phase: PhaseSeed, Focus: "Agent choosing seed..."})
	msgs := []llm.Message{{
		Role: "user",
		Content: `You are starting a completely unbiased knowledge archaeology run.
Choose ONE foundational concept that, if followed without bias, would force you
to reconcile the deepest contradictions in human knowledge — science, philosophy,
consciousness, and ancient wisdom. Return ONLY the concept name, nothing else.`,
	}}
	result, err := e.collectStream(e.brain, "", msgs)
	if err != nil {
		e.emit(Event{Err: fmt.Errorf("seed brain-model: %w", err)})
	}
	seed := strings.TrimSpace(result)
	if seed == "" {
		seed = "consciousness"
	}
	return seed
}

func (e *Engine) ingestPhase(topic string) string {
	e.emit(Event{Phase: PhaseIngest, Focus: topic})

	// Step 1: try Wikipedia for grounded, real-world content
	wikiText := ""
	sources := []string{}
	wiki, err := ingestion.WikiSummary(topic)
	if err == nil && wiki.Extract != "" {
		wikiText = wiki.Extract
		sources = append(sources, wiki.URL)
		e.emit(Event{Phase: PhaseIngest, Focus: topic,
			OutputChunk: fmt.Sprintf("Wikipedia: %d chars on %q\n", len(wikiText), wiki.Title)})
	} else {
		e.emit(Event{Phase: PhaseIngest, Focus: topic,
			OutputChunk: fmt.Sprintf("Wikipedia: no article found for %q — using model\n", topic)})
	}

	// Step 2: arxiv — pull recent academic papers on the topic
	arxivDigest := ""
	papers, arxivErr := ingestion.ArxivSearch(topic, 3)
	if arxivErr == nil && len(papers) > 0 {
		arxivDigest = ingestion.ArxivDigest(papers)
		for _, p := range papers {
			if p.URL != "" {
				sources = append(sources, p.URL)
			}
		}
		e.emit(Event{Phase: PhaseIngest, Focus: topic,
			OutputChunk: fmt.Sprintf("arxiv: %d papers found\n", len(papers))})
	} else {
		e.emit(Event{Phase: PhaseIngest, Focus: topic,
			OutputChunk: "arxiv: no papers found\n"})
	}

	// Step 3: Project Gutenberg — translate topic to a classical author/text search first
	gutenbergDigest := ""
	gutQuery := e.gutenbergQuery(topic)
	if gutQuery != "" {
		books, gutErr := ingestion.GutenbergSearch(gutQuery, 2)
		if gutErr == nil && len(books) > 0 {
			gutenbergDigest = ingestion.GutenbergDigest(books, true)
			for _, bk := range books {
				if bk.URL != "" {
					sources = append(sources, bk.URL)
				}
			}
			e.emit(Event{Phase: PhaseIngest, Focus: topic,
				OutputChunk: fmt.Sprintf("Gutenberg: %d texts found (query: %q)\n", len(books), gutQuery)})
		} else {
			e.emit(Event{Phase: PhaseIngest, Focus: topic,
				OutputChunk: fmt.Sprintf("Gutenberg: no texts found for %q\n", gutQuery)})
		}
	}

	// Step 4: fast model supplements all sources (or generates from scratch if all failed)
	var sourceParts []string
	if wikiText != "" {
		sourceParts = append(sourceParts, fmt.Sprintf("Wikipedia extract:\n%s", wikiText))
	}
	if arxivDigest != "" {
		sourceParts = append(sourceParts, fmt.Sprintf("Recent academic papers:\n%s", arxivDigest))
	}
	if gutenbergDigest != "" {
		sourceParts = append(sourceParts, fmt.Sprintf("Classical texts (Project Gutenberg):\n%s", gutenbergDigest))
	}

	var prompt string
	if len(sourceParts) > 0 {
		prompt = fmt.Sprintf(`Sources on "%s":

%s

Now add what these sources omit or underweight: fringe research, cross-domain
philosophical implications, documented anomalies, and suppressed threads.
Be dense and encyclopedic.`, topic, strings.Join(sourceParts, "\n\n"))
	} else {
		prompt = fmt.Sprintf(`Give a dense, factual summary of everything known about "%s"
across physics, neuroscience, philosophy, ancient texts, and fringe research.
Include mainstream consensus AND documented anomalies. Be encyclopedic.`, topic)
	}

	msgs := []llm.Message{{Role: "user", Content: prompt}}
	supplement, err := e.collectStream(e.fast, "", msgs)
	if err != nil {
		e.emit(Event{Phase: PhaseIngest, Focus: topic, Err: fmt.Errorf("ingest fast-model: %w", err)})
		// if supplement failed but we have wiki, don't abort — use wiki alone
		if wikiText == "" {
			return ""
		}
	}

	if len(sources) == 0 {
		sources = []string{"fast-model-summary"}
	}

	e.mu.Lock()
	e.sources++
	e.mu.Unlock()

	n := &graph.Node{
		ID:      slugify(topic),
		Label:   topic,
		Domain:  "ingested",
		Sources: sources,
		Weight:  1.0,
	}
	e.graph.UpsertNode(n)
	e.syncNode(n)

	combined := wikiText
	if arxivDigest != "" {
		combined += "\n\n" + arxivDigest
	}
	if gutenbergDigest != "" {
		combined += "\n\n" + gutenbergDigest
	}
	if supplement != "" {
		combined += "\n\n" + supplement
	}

	e.emit(Event{Phase: PhaseIngest, Focus: topic,
		OutputChunk: fmt.Sprintf("Ingested: %d chars on %q (wiki=%v)\n", len(combined), topic, wikiText != "")})

	return combined
}

func (e *Engine) thinkAndConnectPhase(topic, content string) string {
	e.emit(Event{Phase: PhaseThink, Focus: topic})

	system := `You are an unbiased knowledge archaeologist. Your job is to find the
REAL connections in human knowledge — not what consensus says, but what the evidence
actually points to when you follow contradictions without flinching.`

	semCtx := e.semanticContext(topic)
	semSection := ""
	if semCtx != "" {
		semSection = fmt.Sprintf("\nSemantically related concepts already in the graph: %s\n", semCtx)
	}

	contentSection := ""
	if content != "" {
		excerpt := content
		if len(excerpt) > 3000 {
			excerpt = excerpt[:3000] + "..."
		}
		contentSection = fmt.Sprintf("\nIngested knowledge on this topic:\n%s\n", excerpt)
	}

	msgs := []llm.Message{{
		Role: "user",
		Content: fmt.Sprintf(`Current focus: "%s"
Graph state: %s%s%s
1. What are the 3 most important connections you see from this topic to other domains?
2. Where does mainstream consensus go SILENT or CONTRADICTORY on this topic? Only report a genuine anomaly — a real gap, suppression, or unresolved contradiction. If there is none worth flagging, write ANOMALY: none.
3. How severe is that anomaly? Rate 0–10 (0=trivial, 10=field-breaking). Be conservative.
4. What single concept should we research next to pull the most important thread?

Format your answer as:
CONNECTIONS: <concept1> | <concept2> | <concept3>
ANOMALY: <description, or "none">
ANOMALY_SCORE: <0-10>
NEXT: <single next concept>`, topic, e.graph.Summary(), semSection, contentSection),
	}}

	// Stream R1's thinking live to the UI
	ch := e.brain.Stream(system, msgs)
	var output strings.Builder
	for chunk := range ch {
		switch chunk.Type {
		case llm.ChunkThink:
			e.writeThink(chunk.Text)
			e.emit(Event{Phase: PhaseThink, Focus: topic, ThinkChunk: chunk.Text})
		case llm.ChunkText:
			output.WriteString(chunk.Text)
			e.emit(Event{Phase: PhaseConnect, Focus: topic, OutputChunk: chunk.Text})
		case llm.ChunkError:
			e.emit(Event{Err: chunk.Err})
		}
	}

	// Parse the structured response
	response := output.String()
	connections, anomaly, anomalyScore, next := parseThinkResponse(response)

	// Add connections to graph
	for _, conn := range connections {
		conn = strings.TrimSpace(conn)
		if conn == "" {
			continue
		}
		cn := &graph.Node{
			ID:     slugify(conn),
			Label:  conn,
			Domain: "inferred",
			Weight: 0.5,
		}
		e.graph.UpsertNode(cn)
		e.syncNode(cn)
		e.graph.AddEdge(&graph.Edge{
			From:       slugify(topic),
			To:         slugify(conn),
			Relation:   "connects_to",
			Confidence: 0.7,
		})
	}

	// Flag anomaly node only if R1 rates it ≥ 7/10 and it's not "none"
	lowered := strings.ToLower(strings.TrimSpace(anomaly))
	if anomaly != "" && lowered != "none" && anomalyScore >= 7 {
		an := &graph.Node{
			// key on description slug so duplicate anomalies merge rather than multiply
			ID:      "anomaly_" + slugify(anomaly),
			Label:   fmt.Sprintf("[ANOMALY] %s", anomaly),
			Domain:  "anomaly",
			Weight:  float64(anomalyScore) / 5.0, // scale: score 7→1.4, 10→2.0
			Anomaly: true,
		}
		e.graph.UpsertNode(an)
		e.syncNode(an)
	}

	if next == "" {
		next = topic
	}
	return next
}

// gutenbergQuery asks the fast model to map an abstract concept to a
// Project Gutenberg-searchable author name or text title.
// Returns empty string if no relevant classical text exists.
func (e *Engine) gutenbergQuery(topic string) string {
	msgs := []llm.Message{{
		Role: "user",
		Content: fmt.Sprintf(`Project Gutenberg contains classical and ancient texts (pre-1930s).
Given the concept "%s", name ONE author or ONE text title from Gutenberg that is most
directly relevant — someone like Plato, Darwin, Kant, William James, Nietzsche, etc.

If no classical Gutenberg text is meaningfully relevant, reply with: NONE

Reply with ONLY the author name or text title. Nothing else.`, topic),
	}}
	result, err := e.collectStream(e.fast, "", msgs)
	if err != nil {
		return ""
	}
	q := strings.TrimSpace(result)
	if q == "" || strings.EqualFold(q, "none") {
		return ""
	}
	// strip any leading/trailing punctuation the model might add
	q = strings.Trim(q, `"'.`)
	return q
}

// writeThinkHeader writes a cycle separator to the think log.
func (e *Engine) writeThinkHeader(cycle int, focus string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.thinkW == nil {
		return
	}
	fmt.Fprintf(e.thinkW, "\n\n═══ Cycle %d — %s — focus: %s ═══\n\n",
		cycle, time.Now().Format("15:04:05"), focus)
	e.thinkW.Flush()
}

// writeThink appends a raw thinking chunk to the think log immediately.
func (e *Engine) writeThink(chunk string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.thinkW == nil {
		return
	}
	e.thinkW.WriteString(chunk)
	e.thinkW.Flush()
}

func (e *Engine) anomalyPhase() {
	e.emit(Event{Phase: PhaseAnomaly, Focus: "Scanning for consensus gaps..."})
	anomalies := e.graph.AnomalyNodes()
	e.emit(Event{Phase: PhaseAnomaly,
		OutputChunk: fmt.Sprintf("Anomaly nodes: %d", len(anomalies))})
}

func (e *Engine) reportPhase() {
	e.emit(Event{Phase: PhaseReport})
	top := e.graph.TopNodes(5)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n## Cycle %d — %s\n", e.cycle, time.Now().Format("15:04:05")))
	sb.WriteString(fmt.Sprintf("**Graph:** %s\n\n", e.graph.Summary()))
	sb.WriteString("**Emergent concepts:**\n")
	for _, n := range top {
		anomalyFlag := ""
		if n.Anomaly {
			anomalyFlag = " ⚠"
		}
		sb.WriteString(fmt.Sprintf("- %s (weight: %.1f)%s\n", n.Label, n.Weight, anomalyFlag))
	}

	e.mu.Lock()
	e.report.WriteString(sb.String())
	e.mu.Unlock()

	e.emit(Event{Phase: PhaseReport, ReportLine: sb.String()})
}

// syncNode embeds a node and upserts it into Qdrant asynchronously.
// Errors are silently dropped — Qdrant is best-effort.
func (e *Engine) syncNode(n *graph.Node) {
	if !e.qdrantOK {
		return
	}
	go func() {
		vec := embeddings.Embed(n.Label + " " + n.Domain)
		_ = e.qdrant.Upsert(store.Collection, n.ID, vec, map[string]any{
			"label":  n.Label,
			"domain": n.Domain,
		})
		e.refreshQdrantStats()
	}()
}

// semanticContext queries Qdrant for nodes semantically similar to topic
// and returns them as a comma-separated string (empty if unavailable).
func (e *Engine) semanticContext(topic string) string {
	if !e.qdrantOK {
		return ""
	}
	vec := embeddings.Embed(topic)
	labels, err := e.qdrant.Search(store.Collection, vec, 5)
	if err != nil || len(labels) == 0 {
		return ""
	}
	return strings.Join(labels, ", ")
}

func (e *Engine) refreshQdrantStats() {
	count, err := e.qdrant.VectorCount(store.Collection)
	if err != nil {
		e.mu.Lock()
		e.qdrantOK = false
		e.mu.Unlock()
		return
	}
	e.mu.Lock()
	e.qdrantOK = true
	e.qdrantVectors = count
	e.mu.Unlock()
}

func (e *Engine) emit(ev Event) {
	ev.GraphSnap = e.snapshot()
	select {
	case e.events <- ev:
	default:
		// drop if buffer full — UI is slow
	}
}

func (e *Engine) snapshot() Snapshot {
	e.mu.Lock()
	cycles        := e.cycle
	sources       := e.sources
	qdrantOK      := e.qdrantOK
	qdrantVectors := e.qdrantVectors
	e.mu.Unlock()

	top := e.graph.TopNodes(5)
	labels  := make([]string,  len(top))
	weights := make([]float64, len(top))
	for i, n := range top {
		labels[i]  = n.Label
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
		QdrantOK:      qdrantOK,
		QdrantVectors: qdrantVectors,
	}
}

// collectStream runs a stream and returns the full text output (no think blocks).
// Returns the first error encountered, if any.
func (e *Engine) collectStream(client *llm.Client, system string, msgs []llm.Message) (string, error) {
	var sb strings.Builder
	var firstErr error
	for chunk := range client.Stream(system, msgs) {
		switch chunk.Type {
		case llm.ChunkText:
			sb.WriteString(chunk.Text)
		case llm.ChunkError:
			if firstErr == nil {
				firstErr = chunk.Err
			}
		}
	}
	return sb.String(), firstErr
}

func parseThinkResponse(s string) (connections []string, anomaly string, anomalyScore int, next string) {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "CONNECTIONS:"):
			parts := strings.Split(strings.TrimPrefix(line, "CONNECTIONS:"), "|")
			for _, p := range parts {
				connections = append(connections, strings.TrimSpace(p))
			}
		case strings.HasPrefix(line, "ANOMALY_SCORE:"):
			fmt.Sscanf(strings.TrimPrefix(line, "ANOMALY_SCORE:"), "%d", &anomalyScore)
		case strings.HasPrefix(line, "ANOMALY:"):
			anomaly = strings.TrimSpace(strings.TrimPrefix(line, "ANOMALY:"))
		case strings.HasPrefix(line, "NEXT:"):
			next = strings.TrimSpace(strings.TrimPrefix(line, "NEXT:"))
		}
	}
	return
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
