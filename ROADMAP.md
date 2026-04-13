# Knowledge Archaeology Engine (KAE) Evolution Roadmap

## Current State Analysis

**What's Working:**
- ✅ 27 autonomous runs completed with 3,227 concept nodes
- ✅ 14,912 source chunks ingested from Wikipedia
- ✅ DeepSeek R1 reasoning traces capturing cross-domain connections
- ✅ Gemini Flash for fast ingestion
- ✅ Qdrant vector storage with 5 collections
- ✅ Anomaly detection flagging consensus gaps
- ✅ Bubbletea/Lipgloss terminal UI
- ✅ Independent concept convergence across runs

**Tier 1 Additions (complete):**
- ✅ Multi-provider support — Anthropic, OpenAI, Gemini, Ollama + OpenRouter via unified `provider:model` syntax
- ✅ Multi-model ensemble reasoning with controversy scoring
- ✅ Novelty decay detection + auto-stop
- ✅ Auto-branching on high controversy
- ✅ Cross-run anomaly clustering (`--analyze`)
- ✅ Gutenberg ingestion fixed via gutendex formats map

**Tier 2 (complete):**
- ✅ Persistent meta-graph across runs (`kae_meta_graph`)
- ✅ Citation chain excavation (Semantic Scholar BFS + suppressed lineage detection)
- ✅ Domain boundary detection (bridges + moats)

**Remaining Gaps (Tier 3+):**
- ⚠️ Limited visualization capabilities
- ⚠️ No active learning or adaptive ingestion
- ⚠️ No self-improvement feedback loop

---

## ✅ TIER 1: Core Engine Enhancements — COMPLETE

### 1.1 Multi-Model Reasoning Ensemble ✅

**Goal:** Run same cycle with multiple models simultaneously to detect consensus gaps through model disagreement.

**Architecture:**
```go
type ModelEnsemble struct {
    Models []LLMProvider // DeepSeek R1, Claude Sonnet, GPT-4, Gemini
    CurrentCycle int
    Responses []ModelResponse
}

type ModelResponse struct {
    ModelName string
    Reasoning string
    Concepts []EmergentConcept
    Timestamp time.Time
}

type InterModelControversy struct {
    Concept string
    DisagreementScore float64 // 0-1, based on concept overlap
    ModelPositions map[string]string // model -> interpretation
    ConsensusGap string // what they collectively avoid
}
```

**Implementation Steps:**
1. Create `pkg/ensemble/` package
2. Implement parallel model calling with goroutines
3. Build concept comparison algorithm (cosine similarity on embeddings)
4. Calculate "controversy score" = 1 - (concept overlap / total concepts)
5. Flag high-controversy concepts as premium anomalies
6. Store ensemble results in new Qdrant collection: `kae_ensemble_runs`

**New CLI Commands:**
```bash
kae run --mode ensemble --models deepseek,claude,gpt4,gemini --cycles 20
kae analyze controversy --run-id run_123 --threshold 0.7
```

**Database Schema:**
```sql
-- New Qdrant collection payload structure
{
  "run_id": "run_1775850000",
  "cycle": 5,
  "concept": "Constraint Propagation",
  "model_responses": [
    {
      "model": "deepseek-r1",
      "interpretation": "...",
      "weight": 4.9,
      "anomaly": 0.60
    },
    {
      "model": "claude-sonnet-4",
      "interpretation": "...",
      "weight": 3.2,
      "anomaly": 0.45
    }
  ],
  "controversy_score": 0.82,
  "consensus_gap": "All models avoid linking constraint propagation to consciousness"
}
```

---

### 1.2 Recursive Depth Control & Auto-Branching ✅

**Goal:** Intelligent run termination and parallel exploration of high-anomaly paths.

**Features:**
- **Novelty Decay Detection:** Stop when new concepts < threshold for N consecutive cycles
- **Dynamic Branching:** Fork into parallel sub-runs when anomaly > 0.5
- **Resource Management:** Kill low-yield threads early

**Implementation:**
```go
type RunController struct {
    NoveltyThreshold float64 // e.g., 0.1 (10% new concepts minimum)
    StagnationWindow int     // e.g., 5 cycles
    BranchThreshold  float64 // e.g., 0.5 anomaly score
    MaxParallelRuns  int     // e.g., 4
}

func (rc *RunController) ShouldContinue(graph *KnowledgeGraph, cycle int) bool {
    recentNovelty := graph.NoveltyScore(cycle, rc.StagnationWindow)
    return recentNovelty >= rc.NoveltyThreshold
}

func (rc *RunController) ShouldBranch(concept EmergentConcept) bool {
    return concept.AnomalyScore >= rc.BranchThreshold
}

func (rc *RunController) SpawnBranch(parentRun *KAERun, concept EmergentConcept) *KAERun {
    branchRun := &KAERun{
        ID: fmt.Sprintf("%s_branch_%s", parentRun.ID, concept.Name),
        ParentRunID: parentRun.ID,
        BranchPoint: concept,
        StartCycle: parentRun.CurrentCycle,
    }
    // Continue exploration from this concept
    return branchRun
}
```

**New Config:**
```yaml
# config/run_control.yaml
novelty:
  threshold: 0.10
  stagnation_window: 5
  
branching:
  enabled: true
  anomaly_threshold: 0.50
  max_parallel: 4
  strategy: "high_controversy" # or "diverse_domains"
  
resource_limits:
  max_cycles_per_branch: 30
  min_novelty_to_continue: 0.05
```

---

### 1.3 Anomaly Clustering & Meta-Analysis ✅

**Goal:** Cross-run anomaly detection to find "convergent heresies."

**Features:**
- Find concepts appearing as anomalies in 3+ independent runs
- Calculate "heresy convergence score"
- Auto-generate anomaly reports

**Implementation:**
```go
type AnomalyCluster struct {
    Concept string
    RunIDs []string
    Occurrences int
    AvgAnomalyScore float64
    ConsensusGap string // what mainstream avoids
    FirstDetected time.Time
    Domains []string
}

func (am *AnomalyMetaAnalyzer) FindConvergentHeresies(minRuns int) []AnomalyCluster {
    // Query Qdrant for all anomalies
    anomalies := am.qdrant.GetAllAnomalies()
    
    // Group by concept similarity (embedding cosine > 0.85)
    clusters := am.clusterBySimilarity(anomalies, 0.85)
    
    // Filter for convergence
    convergent := []AnomalyCluster{}
    for _, cluster := range clusters {
        if len(cluster.UniqueRuns()) >= minRuns {
            convergent = append(convergent, cluster)
        }
    }
    
    return convergent
}
```

**Report Generation:**
```go
type AnomalyReport struct {
    Title string
    GeneratedAt time.Time
    TopClusters []AnomalyCluster
    Summary string
    RecommendedActions []string
}

func (ar *AnomalyReporter) Generate() string {
    tmpl := `
# Convergent Heresy Report
Generated: {{.GeneratedAt}}

## Executive Summary
{{.Summary}}

## Top Anomaly Clusters

{{range .TopClusters}}
### {{.Concept}}
- **Convergence:** Appeared in {{.Occurrences}} independent runs
- **Avg Anomaly Score:** {{.AvgAnomalyScore}}
- **Domains:** {{.Domains}}
- **Consensus Gap:** {{.ConsensusGap}}

**Affected Runs:**
{{range .RunIDs}}
- {{.}}
{{end}}

---
{{end}}

## Recommended Actions
{{range .RecommendedActions}}
- {{.}}
{{end}}
    `
    // Execute template...
}
```

**CLI Commands:**
```bash
kae analyze clusters --min-runs 3 --output report.md
kae analyze convergence --concept "Pseudo-psychology" --show-runs
kae report anomalies --format markdown --email mark@quantummindsunited.com
```

---

## ✅ TIER 2: Knowledge Graph Intelligence — COMPLETE

### 2.1 Persistent Meta-Graph

**Goal:** Build a unified knowledge graph spanning all runs.

**Schema:**
```go
type MetaGraph struct {
    Nodes map[string]*MetaNode
    Edges map[string]*MetaEdge
    Timeline []GraphSnapshot
}

type MetaNode struct {
    Concept string
    FirstSeen time.Time
    RunOccurrences []RunOccurrence
    TotalWeight float64
    AvgAnomalyScore float64
    Domains []string
    IsAttractor bool // appears in 5+ runs
}

type RunOccurrence struct {
    RunID string
    Cycle int
    Weight float64
    AnomalyScore float64
    Context string
}

type MetaEdge struct {
    Source string
    Target string
    Strength float64 // how often they co-occur
    Runs []string
}
```

**Attractor Detection:**
```go
func (mg *MetaGraph) FindAttractors(minOccurrences int) []*MetaNode {
    attractors := []*MetaNode{}
    for _, node := range mg.Nodes {
        if len(node.RunOccurrences) >= minOccurrences {
            node.IsAttractor = true
            attractors = append(attractors, node)
        }
    }
    return attractors
}
```

**Qdrant Collection:**
```json
// Collection: kae_meta_graph
{
  "concept": "Boundary",
  "first_seen": "2025-02-15T13:10:04Z",
  "run_occurrences": [
    {"run_id": "run_1775762017", "cycle": 2, "weight": 3.6},
    {"run_id": "run_1775768049", "cycle": 3, "weight": 8.5},
    ...
  ],
  "total_weight": 45.7,
  "avg_anomaly": 0.34,
  "domains": ["physics", "neuroscience", "mathematics"],
  "is_attractor": true,
  "embedding": [0.123, -0.456, ...]
}
```

---

### 2.2 Citation Chain Excavation

**Goal:** Auto-expand knowledge base by following citation networks.

**Implementation:**
```go
type CitationCrawler struct {
    SemanticScholarAPI string
    ArxivAPI string
    MaxDepth int // e.g., 3 levels deep
}

func (cc *CitationCrawler) ExpandFromPaper(arxivID string, depth int) []Paper {
    if depth >= cc.MaxDepth {
        return nil
    }
    
    // Get references
    refs := cc.arxivAPI.GetReferences(arxivID)
    
    // Get citations (who cited this paper)
    citations := cc.semanticScholar.GetCitations(arxivID)
    
    papers := []Paper{}
    for _, ref := range refs {
        papers = append(papers, ref)
        // Recurse
        papers = append(papers, cc.ExpandFromPaper(ref.ID, depth+1)...)
    }
    
    return papers
}

func (cc *CitationCrawler) FindSuppressedLineages(concept string) []SuppressedLineage {
    // Find papers about concept
    papers := cc.searchPapers(concept)
    
    lineages := []SuppressedLineage{}
    for _, paper := range papers {
        // Check citation count
        if paper.Citations < 5 && paper.Age > 2*365*24*time.Hour {
            // Highly relevant but uncited
            if paper.RelevanceScore > 0.8 {
                lineages = append(lineages, SuppressedLineage{
                    Paper: paper,
                    Concept: concept,
                    CitationGap: "High relevance, zero follow-up",
                })
            }
        }
    }
    
    return lineages
}
```

**Automatic Ingestion:**
```yaml
# config/citation_crawler.yaml
enabled: true
max_depth: 3
apis:
  semantic_scholar: true
  arxiv: true
  
triggers:
  - anomaly_score: ">0.6"
  - concept_weight: ">5.0"
  - convergent_heresy: true
  
filters:
  min_citations: 0
  max_age_years: 50
  domains: ["physics", "neuroscience", "consciousness"]
```

---

### 2.3 Domain Boundary Detection

**Goal:** Find artificial separations between domains.

**Implementation:**
```go
type DomainBridge struct {
    Concept string
    Domain1 string
    Domain2 string
    Weight float64 // strength of connection
}

type DomainMoat struct {
    Domain1 string
    Domain2 string
    SharedCorpus bool // appear in same sources
    EdgeCount int    // but never connected
    Suspicion float64 // likelihood of artificial separation
}

func (mg *MetaGraph) FindBridges() []DomainBridge {
    bridges := []DomainBridge{}
    for _, node := range mg.Nodes {
        if len(node.Domains) >= 2 {
            // This concept spans multiple domains
            for i := 0; i < len(node.Domains); i++ {
                for j := i+1; j < len(node.Domains); j++ {
                    bridges = append(bridges, DomainBridge{
                        Concept: node.Concept,
                        Domain1: node.Domains[i],
                        Domain2: node.Domains[j],
                        Weight: node.TotalWeight,
                    })
                }
            }
        }
    }
    return bridges
}

func (mg *MetaGraph) FindMoats() []DomainMoat {
    // Find domain pairs that:
    // 1. Share corpus (appear in same Wikipedia articles)
    // 2. Never get connected in KAE reasoning
    // 3. High suspicion of artificial separation
    
    domainPairs := mg.getAllDomainPairs()
    moats := []DomainMoat{}
    
    for _, pair := range domainPairs {
        if pair.SharedCorpus && pair.EdgeCount == 0 {
            moats = append(moats, DomainMoat{
                Domain1: pair.D1,
                Domain2: pair.D2,
                SharedCorpus: true,
                EdgeCount: 0,
                Suspicion: 0.95, // strong indicator
            })
        }
    }
    
    return moats
}
```

---

## 🚀 TIER 3: Autonomous Evolution

### 3.1 Self-Modifying Prompts

**Goal:** KAE learns from its own reasoning to improve future runs.

**Implementation:**
```go
type PromptEvolver struct {
    BasePrompt string
    EvolutionHistory []PromptVersion
}

type PromptVersion struct {
    Version int
    Prompt string
    GeneratedBy string // run_id
    PerformanceMetrics PromptMetrics
    Timestamp time.Time
}

type PromptMetrics struct {
    AvgNoveltyScore float64
    AvgAnomalyScore float64
    ConceptsPerCycle float64
    UniqueDomainsHit int
    BridgesCrossed int
}

func (pe *PromptEvolver) AnalyzeRun(run *KAERun) PromptImprovement {
    // Analyze where reasoning got stuck
    stuckPoints := run.FindStagnation()
    
    // Identify missed connections
    missedConnections := run.IdentifyMissedOpportunities()
    
    // Generate improvement suggestions
    improvements := []string{}
    
    if len(stuckPoints) > 0 {
        improvements = append(improvements, 
            "Add explicit instruction to explore counter-intuitive connections")
    }
    
    if len(missedConnections) > 0 {
        improvements = append(improvements,
            fmt.Sprintf("Emphasize cross-domain synthesis in: %v", 
                missedConnections.Domains))
    }
    
    return PromptImprovement{
        RunID: run.ID,
        Issues: stuckPoints,
        Suggestions: improvements,
    }
}

func (pe *PromptEvolver) GenerateNextPrompt(improvements []PromptImprovement) string {
    // Use LLM to generate improved prompt
    systemMsg := `You are a prompt evolution system. Given analysis of previous 
    KAE runs, generate an improved system prompt that addresses identified issues 
    while maintaining core objectives.`
    
    context := fmt.Sprintf(`
    Base Prompt:
    %s
    
    Issues Identified:
    %v
    
    Generate improved prompt:
    `, pe.BasePrompt, improvements)
    
    // Call LLM
    newPrompt := pe.llm.Generate(systemMsg, context)
    
    // Store in history
    pe.EvolutionHistory = append(pe.EvolutionHistory, PromptVersion{
        Version: len(pe.EvolutionHistory) + 1,
        Prompt: newPrompt,
        Timestamp: time.Now(),
    })
    
    return newPrompt
}
```

**Storage:**
```json
// Collection: kae_prompt_evolution
{
  "version": 5,
  "prompt": "You are an unbiased knowledge archaeologist...",
  "parent_version": 4,
  "generated_by_run": "run_1775850000",
  "performance_delta": {
    "novelty": +0.15,
    "anomalies": +0.23,
    "bridges": +2
  },
  "changes_made": [
    "Added explicit cross-domain synthesis instruction",
    "Emphasized naive observer perspective"
  ]
}
```

---

### 3.2 Hypothesis Generation Engine

**Goal:** Auto-generate testable predictions from anomalies.

**Implementation:**
```go
type HypothesisGenerator struct {
    llm LLMProvider
    qdrant *QdrantClient
}

type Hypothesis struct {
    ID string
    AnomalySource string
    Statement string
    Predictions []Prediction
    TestableExperiments []Experiment
    Confidence float64
    GeneratedAt time.Time
}

type Prediction struct {
    Domain string
    Claim string
    Observable string // what we should see if true
}

type Experiment struct {
    Description string
    DataRequired []string
    Feasibility string // "trivial", "hard", "requires_lab"
}

func (hg *HypothesisGenerator) FromAnomaly(anomaly AnomalyCluster) Hypothesis {
    prompt := fmt.Sprintf(`
    Given this anomaly detected across multiple independent reasoning runs:
    
    Concept: %s
    Consensus Gap: %s
    Domains: %v
    
    Generate:
    1. A clear hypothesis statement
    2. Three testable predictions across different domains
    3. Experiments that could validate or falsify this hypothesis
    `, anomaly.Concept, anomaly.ConsensusGap, anomaly.Domains)
    
    response := hg.llm.Generate(prompt)
    
    // Parse structured output
    hypothesis := hg.parseHypothesis(response)
    hypothesis.AnomalySource = anomaly.Concept
    hypothesis.Confidence = anomaly.AvgAnomalyScore
    
    return hypothesis
}
```

**Example Output:**
```markdown
# Hypothesis H-2025-001

**Source Anomaly:** Pseudo-psychology (convergent across 3 runs)

**Statement:** Mainstream psychology's focus on behavioral/cognitive models 
systematically excludes non-local consciousness mechanisms, despite evidence 
from quantum biology and parapsychology literature.

## Predictions

1. **Neuroscience:** If true, we should find non-classical correlations in 
   EEG data during reported psi events that standard models cannot explain.

2. **Physics:** Quantum coherence should persist longer in biological systems 
   than predicted by decoherence theory.

3. **Psychology:** Replication studies of psi research should show file-drawer 
   effect masking positive results.

## Testable Experiments

1. **Meta-analysis of suppressed parapsychology data**
   - Data: Published + unpublished psi experiments
   - Feasibility: Trivial (data exists)
   
2. **EEG correlation study during remote viewing**
   - Data: Lab equipment, trained participants
   - Feasibility: Hard (requires IRB, funding)
```

---

### 3.3 Active Learning Scheduler

**Goal:** Adaptive ingestion based on detected anomalies.

**Implementation:**
```go
type ActiveLearner struct {
    qdrant *QdrantClient
    ingestionQueue chan IngestionTask
    priorities map[string]float64 // domain -> priority
}

type IngestionTask struct {
    Source string // "wikipedia", "arxiv", "semantic_scholar"
    Query string
    Domain string
    Priority float64
    Reason string
}

func (al *ActiveLearner) OnAnomalyDetected(anomaly EmergentConcept) {
    // High-anomaly concept detected, prioritize related sources
    
    // If it's a physics concept, pull more physics papers
    if anomaly.AnomalyScore > 0.5 {
        al.priorities[anomaly.Domain] += 0.3
        
        // Queue targeted ingestion
        al.ingestionQueue <- IngestionTask{
            Source: "arxiv",
            Query: anomaly.Name,
            Domain: anomaly.Domain,
            Priority: anomaly.AnomalyScore,
            Reason: fmt.Sprintf("High anomaly detected: %s", anomaly.Name),
        }
    }
}

func (al *ActiveLearner) FillGap(currentGraph *MetaGraph) {
    // Identify missing domains
    domains := map[string]int{
        "physics": 0,
        "consciousness": 0,
        "biology": 0,
        "mathematics": 0,
        "philosophy": 0,
    }
    
    for _, node := range currentGraph.Nodes {
        for _, d := range node.Domains {
            domains[d]++
        }
    }
    
    // Find least-represented domain
    minDomain := ""
    minCount := 999999
    for d, count := range domains {
        if count < minCount {
            minDomain = d
            minCount = count
        }
    }
    
    // Queue ingestion for that domain
    al.ingestionQueue <- IngestionTask{
        Source: "wikipedia",
        Query: minDomain,
        Domain: minDomain,
        Priority: 0.7,
        Reason: "Gap-filling: underrepresented domain",
    }
}

func (al *ActiveLearner) ControversySeeking() {
    // Find topics likely to have conflicting views
    controversialTopics := []string{
        "consciousness",
        "quantum measurement",
        "dark matter",
        "origin of life",
    }
    
    for _, topic := range controversialTopics {
        al.ingestionQueue <- IngestionTask{
            Source: "arxiv",
            Query: fmt.Sprintf("%s controversy debate", topic),
            Priority: 0.8,
            Reason: "Controversy-seeking mode",
        }
    }
}
```

**Config:**
```yaml
# config/active_learning.yaml
enabled: true

triggers:
  anomaly_threshold: 0.5
  gap_detection: true
  controversy_seeking: true
  
ingestion_strategy:
  on_anomaly:
    - arxiv_search
    - semantic_scholar_refs
    - wikipedia_related
    
  on_gap:
    - wikipedia_category
    - arxiv_category
    
  on_controversy:
    - arxiv_debate_papers
    - reddit_discussions
    
limits:
  max_sources_per_trigger: 10
  max_daily_ingestion: 1000
```

---

## 🎨 TIER 4: Visualization & Interface

### 4.1 Live Graph Visualization (Web UI)

**Tech Stack:**
- **Backend:** Go HTTP server
- **Frontend:** React + D3.js / Three.js
- **Real-time:** WebSocket for live updates

**Implementation:**
```go
// cmd/kae-web/main.go
package main

import (
    "net/http"
    "github.com/gorilla/websocket"
)

type GraphServer struct {
    upgrader websocket.Upgrader
    clients  map[*websocket.Conn]bool
    broadcast chan GraphUpdate
}

type GraphUpdate struct {
    Type string `json:"type"` // "node_added", "edge_added", "anomaly_detected"
    Data interface{} `json:"data"`
}

func (gs *GraphServer) StreamRun(runID string) {
    run := loadRun(runID)
    
    for cycle := range run.Cycles {
        for _, concept := range cycle.NewConcepts {
            gs.broadcast <- GraphUpdate{
                Type: "node_added",
                Data: concept,
            }
        }
        
        if cycle.AnomalyDetected {
            gs.broadcast <- GraphUpdate{
                Type: "anomaly_detected",
                Data: cycle.Anomaly,
            }
        }
    }
}
```

**React Component:**
```jsx
// web/src/components/LiveGraph.jsx
import React, { useEffect, useRef } from 'react';
import * as d3 from 'd3';

export const LiveGraph = ({ runId }) => {
  const svgRef = useRef();
  const wsRef = useRef();
  
  useEffect(() => {
    const ws = new WebSocket(`ws://localhost:8080/runs/${runId}/stream`);
    
    ws.onmessage = (event) => {
      const update = JSON.parse(event.data);
      
      if (update.type === 'node_added') {
        addNode(update.data);
        
        if (update.data.is_anomaly) {
          highlightAnomaly(update.data);
        }
      }
    };
    
    wsRef.current = ws;
    
    return () => ws.close();
  }, [runId]);
  
  const addNode = (concept) => {
    // D3 force simulation update
    const svg = d3.select(svgRef.current);
    
    // Add node with animation
    svg.append('circle')
      .attr('r', 0)
      .attr('fill', concept.is_anomaly ? '#ff4444' : '#4444ff')
      .transition()
      .duration(500)
      .attr('r', Math.log(concept.weight) * 10);
  };
  
  const highlightAnomaly = (concept) => {
    // Pulse animation for anomalies
    d3.select(`#node-${concept.id}`)
      .transition()
      .duration(200)
      .attr('r', 30)
      .transition()
      .duration(200)
      .attr('r', 20);
  };
  
  return <svg ref={svgRef} width="100%" height="800px" />;
};
```

**Features:**
- Real-time node spawning
- Domain-based color coding
- Anomaly pulse animations
- 3D force-directed layout option
- Zoom/pan controls
- Node click → show reasoning trace

---

### 4.2 Reasoning Trace Replayer

**Implementation:**
```go
// pkg/replay/replayer.go
type ReasoningReplayer struct {
    run *KAERun
}

type ThoughtMap struct {
    Steps []ThoughtStep `json:"steps"`
    Connections []Connection `json:"connections"`
}

type ThoughtStep struct {
    Cycle int `json:"cycle"`
    Reasoning string `json:"reasoning"`
    ConceptsGenerated []string `json:"concepts"`
    AnomaliesDetected []string `json:"anomalies"`
}

func (rr *ReasoningReplayer) GenerateThoughtMap() ThoughtMap {
    tm := ThoughtMap{}
    
    for _, cycle := range rr.run.Cycles {
        step := ThoughtStep{
            Cycle: cycle.Number,
            Reasoning: cycle.ModelResponse.Reasoning,
            ConceptsGenerated: extractConcepts(cycle),
            AnomaliesDetected: extractAnomalies(cycle),
        }
        tm.Steps = append(tm.Steps, step)
    }
    
    return tm
}

func (rr *ReasoningReplayer) ExportToVideo() error {
    // Use ffmpeg to generate video from frames
    // Each frame = one cycle's thought map
    // Animate transitions between concepts
}
```

**Controversy Viewer (Side-by-side):**
```jsx
// web/src/components/ControversyViewer.jsx
export const ControversyViewer = ({ anomaly }) => {
  return (
    <div className="controversy-split">
      <div className="mainstream-view">
        <h3>Mainstream Position</h3>
        <p>{anomaly.mainstream_interpretation}</p>
        <ul>
          {anomaly.supporting_papers.map(paper => (
            <li key={paper.id}>{paper.title}</li>
          ))}
        </ul>
      </div>
      
      <div className="divider">
        <div className="gap-indicator">
          CONSENSUS GAP: {anomaly.gap_description}
        </div>
      </div>
      
      <div className="anomaly-view">
        <h3>KAE Detection</h3>
        <p>{anomaly.kae_interpretation}</p>
        <ul>
          {anomaly.suppressed_papers.map(paper => (
            <li key={paper.id}>
              {paper.title}
              <span className="citation-gap">
                ({paper.citations} citations)
              </span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
};
```

---

### 4.3 Natural Language Query Interface

**Implementation:**
```go
// pkg/nlq/query.go
type NLQueryEngine struct {
    metaGraph *MetaGraph
    qdrant *QdrantClient
    llm LLMProvider
}

func (nlq *NLQueryEngine) Query(question string) QueryResult {
    // Parse intent
    intent := nlq.parseIntent(question)
    
    switch intent.Type {
    case "show_anomalies":
        return nlq.showAnomalies(intent.Filters)
        
    case "compare_runs":
        return nlq.compareRuns(intent.RunIDs)
        
    case "concept_domains":
        return nlq.conceptDomains(intent.Concept)
        
    case "consensus_gap":
        return nlq.consensusGap(intent.Topic)
    }
}

func (nlq *NLQueryEngine) parseIntent(question string) Intent {
    // Use LLM to parse natural language into structured query
    prompt := fmt.Sprintf(`
    Parse this query into structured intent:
    "%s"
    
    Return JSON:
    {
      "type": "show_anomalies|compare_runs|concept_domains|consensus_gap",
      "filters": {...},
      "concept": "...",
      "run_ids": [...]
    }
    `, question)
    
    response := nlq.llm.Generate(prompt)
    return parseIntent(response)
}
```

**CLI Implementation:**
```bash
$ kae query "Show me all anomalies related to consciousness and quantum mechanics"

Found 7 anomalies matching query:

1. Quantum measurement and observer effect (3 runs)
   Domains: physics, consciousness
   Avg Anomaly: 0.72
   Gap: Mainstream physics treats observation as purely physical

2. Non-local correlations in EEG (2 runs)
   Domains: neuroscience, quantum_biology
   Avg Anomaly: 0.65
   Gap: Neuroscience ignores quantum coherence in brain

...

$ kae query "What domains does 'boundary' appear in?"

Concept: Boundary
Runs: 15
Total Weight: 87.3
Domains:
  - physics (8 runs, weight: 34.2)
  - neuroscience (6 runs, weight: 28.1)
  - mathematics (7 runs, weight: 25.0)

Bridges to other concepts:
  - Saturation (strength: 0.82)
  - Transfer (strength: 0.76)
  - Self (strength: 0.68)

$ kae query "Compare run_1775833064 and run_1775847739"

Run Comparison:
  
Shared concepts: 12
  - Boundary (weights: 8.5 vs 12.2)
  - Self (weights: 4.9 vs 4.9)
  - Constraint Propagation (weights: 4.9 vs 4.9)
  ...

Divergent paths:
  run_1775833064 → Pseudo-psychology → Mathematical medicine
  run_1775847739 → Quantum noise → Symmetry breaking

Unique to run_1775833064:
  - CP(N-1) affine connection
  - Explicit matrix representation

Unique to run_1775847739:
  - Information erasure threshold
  - Phase-locking in quantum systems
```

---

## 🔬 TIER 5: Experimental Features

### 5.1 Adversarial Anomaly Generation

**Goal:** Dialectic between anomaly-seeker and mainstream-defender.

**Implementation:**
```go
type DialecticEngine struct {
    seeker LLMProvider   // anomaly detector
    defender LLMProvider // mainstream defender
    judge LLMProvider    // evaluates arguments
}

type DialecticRound struct {
    Anomaly EmergentConcept
    SeekerArgument string
    DefenderRebuttal string
    JudgeVerdict Verdict
}

type Verdict struct {
    AnomalyValid bool
    Confidence float64
    Reasoning string
    ImprovedAnomalyDescription string
}

func (de *DialecticEngine) RunDialectic(anomaly EmergentConcept) []DialecticRound {
    rounds := []DialecticRound{}
    
    // Round 1: Seeker presents anomaly
    seekerArg := de.seeker.Generate(fmt.Sprintf(`
        Argue why this represents a genuine gap in mainstream consensus:
        %s
    `, anomaly.Description))
    
    // Round 2: Defender rebuts
    defenderArg := de.defender.Generate(fmt.Sprintf(`
        You represent mainstream science. Defend the consensus position against:
        %s
    `, seekerArg))
    
    // Round 3: Judge evaluates
    verdict := de.judge.Evaluate(seekerArg, defenderArg)
    
    rounds = append(rounds, DialecticRound{
        Anomaly: anomaly,
        SeekerArgument: seekerArg,
        DefenderRebuttal: defenderArg,
        JudgeVerdict: verdict,
    })
    
    return rounds
}
```

**Example Output:**
```markdown
# Dialectic: Pseudo-psychology

## Anomaly Seeker
The term "pseudo-psychology" appears as an anomaly across 3 independent runs. 
Mainstream psychology systematically excludes parapsychological phenomena 
despite methodologically sound research. The Ganzfeld experiments show 
effect sizes comparable to aspirin for heart attacks, yet remain ignored.

## Mainstream Defender
Parapsychology fails basic replication standards. The file-drawer effect 
explains apparent positive results. No mechanism exists within known physics 
for telepathy or precognition. Psychology focuses on reproducible phenomena.

## Judge Verdict
**Anomaly Valid: Yes (confidence: 0.68)**

Both sides present valid points. The seeker correctly identifies systematic 
exclusion of anomalous data. The defender correctly notes replication issues. 

However, the *pattern of exclusion itself* is the anomaly - mainstream 
psychology doesn't engage with the strongest parapsychology evidence, instead 
dismissing the entire field a priori.

**Improved Description:** "Psychology's methodological framework systematically 
excludes investigation of claimed psi phenomena, even when effect sizes and 
experimental controls meet publication standards in other domains."
```

---

### 5.2 Consciousness Research Integration

**Goal:** Special handling for metaphysical frameworks.

**Implementation:**
```go
type ConsciousnessFramework struct {
    Name string // "Seth Speaks", "Dolores Cannon", "Bashar"
    Sources []Source
    CoreConcepts []string
    Predictions []Prediction
}

type MetaphysicalAnalyzer struct {
    frameworks map[string]*ConsciousnessFramework
    qdrant *QdrantClient
}

func (ma *MetaphysicalAnalyzer) CrossReference(paper ArxivPaper) []Alignment {
    alignments := []Alignment{}
    
    // Extract paper's claims
    claims := ma.extractClaims(paper)
    
    // Check against each framework
    for name, framework := range ma.frameworks {
        score := ma.alignmentScore(claims, framework.Predictions)
        
        if score > 0.6 {
            alignments = append(alignments, Alignment{
                Framework: name,
                Paper: paper,
                Score: score,
                MatchingClaims: ma.findMatches(claims, framework),
            })
        }
    }
    
    return alignments
}

func (ma *MetaphysicalAnalyzer) IngestSethSpeaks() {
    // Parse Seth Speaks text
    framework := &ConsciousnessFramework{
        Name: "Seth Speaks",
        CoreConcepts: []string{
            "reality construction",
            "simultaneous time",
            "reincarnational selves",
            "consciousness units (CUs)",
        },
        Predictions: []Prediction{
            {
                Domain: "quantum_physics",
                Claim: "Observer creates reality through consciousness",
                Observable: "Consciousness should affect quantum measurements",
            },
            {
                Domain: "neuroscience",
                Claim: "Brain is receiver, not generator of consciousness",
                Observable: "Consciousness should persist independent of brain state",
            },
        },
    }
    
    ma.frameworks["Seth Speaks"] = framework
}
```

**Query Examples:**
```bash
$ kae consciousness analyze --paper arxiv:2105.02314 --frameworks seth,cannon,bashar

Paper: "Consciousness and the Collapse of the Wave Function"

Alignment Scores:
  Seth Speaks: 0.78 (HIGH)
  Dolores Cannon: 0.65 (MEDIUM)
  Bashar: 0.52 (MEDIUM)

Seth Speaks Matches:
  ✓ Paper claims consciousness collapses wave function
    → Aligns with Seth's "consciousness creates reality"
  
  ✓ Discusses integrated information theory
    → Parallels Seth's "consciousness units (CUs)"
    
  ⚠ Assumes brain-based consciousness
    → Conflicts with Seth's "brain as receiver" model

Recommended Follow-up:
  - Search for papers on non-local consciousness
  - Investigate quantum biology + consciousness
  - Look for receiver theory of consciousness
```

---

### 5.3 Prediction Validation System

**Goal:** Track when mainstream "discovers" KAE anomalies.

**Implementation:**
```go
type PredictionTracker struct {
    predictions map[string]*TrackedPrediction
    arxivMonitor *ArxivMonitor
}

type TrackedPrediction struct {
    ID string
    Anomaly string
    Claim string
    DetectedAt time.Time
    Validated bool
    ValidationPaper *Paper
    ValidationDate time.Time
    LeadTime time.Duration // time between KAE detection and validation
}

func (pt *PredictionTracker) MonitorValidation(anomaly AnomalyCluster) {
    // Create prediction
    pred := &TrackedPrediction{
        ID: generateID(),
        Anomaly: anomaly.Concept,
        Claim: anomaly.ConsensusGap,
        DetectedAt: time.Now(),
    }
    
    pt.predictions[pred.ID] = pred
    
    // Set up arxiv monitor
    pt.arxivMonitor.Watch(arxiv.WatchParams{
        Keywords: extractKeywords(anomaly),
        Callback: func(paper *Paper) {
            if pt.matchesPrediction(paper, pred) {
                pt.validatePrediction(pred, paper)
            }
        },
    })
}

func (pt *PredictionTracker) validatePrediction(pred *TrackedPrediction, paper *Paper) {
    pred.Validated = true
    pred.ValidationPaper = paper
    pred.ValidationDate = time.Now()
    pred.LeadTime = pred.ValidationDate.Sub(pred.DetectedAt)
    
    // Log validation event
    log.Printf("VALIDATION: KAE detected '%s' %.0f days before publication",
        pred.Anomaly, pred.LeadTime.Hours()/24)
    
    // Update credibility score
    pt.updateCredibilityScore(pred.LeadTime)
}

type CredibilityScore struct {
    TotalPredictions int
    Validated int
    AvgLeadTime time.Duration
    Score float64 // 0-1
}

func (pt *PredictionTracker) GetCredibilityScore() CredibilityScore {
    validated := 0
    totalLeadTime := time.Duration(0)
    
    for _, pred := range pt.predictions {
        if pred.Validated {
            validated++
            totalLeadTime += pred.LeadTime
        }
    }
    
    avgLead := totalLeadTime / time.Duration(validated)
    score := float64(validated) / float64(len(pt.predictions))
    
    return CredibilityScore{
        TotalPredictions: len(pt.predictions),
        Validated: validated,
        AvgLeadTime: avgLead,
        Score: score,
    }
}
```

**Dashboard:**
```markdown
# KAE Prediction Validation Dashboard

## Credibility Score: 0.73 (HIGH)

Total Predictions: 15
Validated: 11
Pending: 4
Average Lead Time: 127 days

## Recent Validations

1. **Quantum noise-induced symmetry breaking**
   - Detected: 2025-01-15
   - Validated: 2025-04-08 (83 days)
   - Paper: "Emergent Symmetry Breaking in Open Quantum Systems" (Nature Physics)
   
2. **Non-local EEG correlations**
   - Detected: 2025-02-01
   - Validated: 2025-05-12 (100 days)
   - Paper: "Long-range correlations in human brain networks" (PNAS)

## Pending Predictions

1. **Rigged Hilbert spaces for de Sitter algebra**
   - Detected: 2025-04-10
   - Status: Monitoring arxiv hep-th
   
2. **Pseudo-psychology framework**
   - Detected: 2025-03-20
   - Status: Monitoring psychology journals
```

---

## ⚡ TIER 6: Quick Wins (Immediate Implementation)

### 6.1 Anomaly Auto-Post to QMU Forum

```go
// pkg/integrations/qmu_forum.go
type QMUForumPoster struct {
    discourseAPI *DiscourseClient
    category string // "KAE Discoveries"
}

func (qfp *QMUForumPoster) PostAnomalyReport(clusters []AnomalyCluster) error {
    // Take top 3 anomalies
    top := clusters[:3]
    
    for _, cluster := range top {
        post := qfp.formatPost(cluster)
        
        err := qfp.discourseAPI.CreateTopic(discourse.Topic{
            Title: fmt.Sprintf("[KAE] %s - Convergent Anomaly", cluster.Concept),
            Category: qfp.category,
            Raw: post,
            Tags: []string{"kae", "anomaly", cluster.Domains[0]},
        })
        
        if err != nil {
            return err
        }
    }
    
    return nil
}

func (qfp *QMUForumPoster) formatPost(cluster AnomalyCluster) string {
    return fmt.Sprintf(`
# KAE Anomaly Detection: %s

**Convergence:** Appeared in %d independent runs
**Domains:** %v
**Average Anomaly Score:** %.2f

## Consensus Gap
%s

## Runs
%v

## Questions for Discussion
1. Does this anomaly resonate with your own research/experience?
2. What mainstream explanations might we be missing?
3. What experiments could validate this observation?

---
*Auto-generated by Knowledge Archaeology Engine*
*Run IDs: %v*
    `, cluster.Concept, cluster.Occurrences, cluster.Domains, 
       cluster.AvgAnomalyScore, cluster.ConsensusGap, 
       cluster.RunIDs, cluster.RunIDs)
}
```

---

### 6.2 Email Digest

```go
// pkg/notifications/email.go
type DigestMailer struct {
    smtp SMTPConfig
    template *template.Template
}

type RunDigest struct {
    RunID string
    Cycles int
    TopAnomalies []EmergentConcept
    GraphStats GraphStats
    NovelConcepts []string
}

func (dm *DigestMailer) SendDigest(run *KAERun, recipient string) error {
    digest := RunDigest{
        RunID: run.ID,
        Cycles: run.TotalCycles,
        TopAnomalies: run.TopAnomalies(3),
        GraphStats: run.Graph.Stats(),
        NovelConcepts: run.MostNovel(5),
    }
    
    html := dm.template.Execute(digest)
    
    return dm.smtp.Send(Email{
        To: recipient,
        Subject: fmt.Sprintf("[KAE] Run %s Complete - %d Anomalies", 
            run.ID, len(digest.TopAnomalies)),
        HTML: html,
    })
}
```

**Email Template:**
```html
<!DOCTYPE html>
<html>
<head>
    <style>
        .anomaly { 
            background: #ffeeee; 
            padding: 10px; 
            margin: 10px 0; 
            border-left: 4px solid #ff4444;
        }
        .stats { 
            display: flex; 
            justify-content: space-around; 
        }
        .stat { 
            text-align: center; 
        }
    </style>
</head>
<body>
    <h1>KAE Run Complete: {{.RunID}}</h1>
    
    <div class="stats">
        <div class="stat">
            <h2>{{.Cycles}}</h2>
            <p>Cycles</p>
        </div>
        <div class="stat">
            <h2>{{.GraphStats.Nodes}}</h2>
            <p>Concepts</p>
        </div>
        <div class="stat">
            <h2>{{len .TopAnomalies}}</h2>
            <p>Anomalies</p>
        </div>
    </div>
    
    <h2>Top Anomalies</h2>
    {{range .TopAnomalies}}
    <div class="anomaly">
        <h3>{{.Name}}</h3>
        <p><strong>Score:</strong> {{.AnomalyScore}}</p>
        <p><strong>Weight:</strong> {{.Weight}}</p>
        <p>{{.Description}}</p>
    </div>
    {{end}}
    
    <h2>Novel Concepts</h2>
    <ul>
    {{range .NovelConcepts}}
        <li>{{.}}</li>
    {{end}}
    </ul>
    
    <p><a href="http://localhost:8080/runs/{{.RunID}}">View Full Report</a></p>
</body>
</html>
```

---

### 6.3 Qdrant Backup Automation

```bash
#!/bin/bash
# scripts/backup_qdrant.sh

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/backups/qdrant/$DATE"
COLLECTIONS=("kae_nodes" "kae_chunks" "kae_meta_graph" "kae_ensemble_runs")

mkdir -p $BACKUP_DIR

for collection in "${COLLECTIONS[@]}"; do
    echo "Backing up $collection..."
    curl -X POST "http://localhost:6333/collections/$collection/snapshots" \
         -H "Content-Type: application/json"
    
    # Download snapshot
    SNAPSHOT_NAME=$(curl "http://localhost:6333/collections/$collection/snapshots" | jq -r '.[0].name')
    curl "http://localhost:6333/collections/$collection/snapshots/$SNAPSHOT_NAME" \
         -o "$BACKUP_DIR/${collection}.snapshot"
done

# Upload to S3/R2
rclone copy $BACKUP_DIR r2:kae-backups/qdrant/$DATE

echo "Backup complete: $BACKUP_DIR"
```

**Cron:**
```cron
# Run daily at 2 AM
0 2 * * * /home/mark/kae/scripts/backup_qdrant.sh
```

---

### 6.4 Run Comparison CLI Tool

```bash
#!/bin/bash
# Already have the MCP function, just wrap it

kae compare run_1775833064 run_1775847739 --format markdown > comparison.md
kae compare run_1775833064 run_1775847739 --format json > comparison.json
kae compare run_1775833064 run_1775847739 --show-graphs
```

---

### 6.5 Anomaly RSS Feed

```go
// pkg/feeds/rss.go
type AnomalyFeed struct {
    tracker *AnomalyTracker
    feedPath string
}

func (af *AnomalyFeed) Generate() error {
    feed := &feeds.Feed{
        Title: "KAE Anomaly Discoveries",
        Link: &feeds.Link{Href: "https://quantummindsunited.com/kae"},
        Description: "Real-time anomaly detection from Knowledge Archaeology Engine",
        Created: time.Now(),
    }
    
    // Get recent anomalies
    anomalies := af.tracker.GetRecent(20)
    
    for _, anomaly := range anomalies {
        feed.Items = append(feed.Items, &feeds.Item{
            Title: anomaly.Concept,
            Link: &feeds.Link{Href: fmt.Sprintf("https://qmu.com/kae/anomaly/%s", anomaly.ID)},
            Description: anomaly.ConsensusGap,
            Created: anomaly.DetectedAt,
            Content: af.formatContent(anomaly),
        })
    }
    
    rss, err := feed.ToRss()
    if err != nil {
        return err
    }
    
    return os.WriteFile(af.feedPath, []byte(rss), 0644)
}
```

**Serve via HTTP:**
```go
http.HandleFunc("/feeds/anomalies.rss", func(w http.ResponseWriter, r *http.Request) {
    feed := af.Generate()
    w.Header().Set("Content-Type", "application/rss+xml")
    w.Write([]byte(feed))
})
```

---

## 📋 Implementation Roadmap

### Phase 1: Foundation (Weeks 1-2)
- [ ] Multi-model ensemble architecture
- [ ] Persistent meta-graph in Qdrant
- [ ] Anomaly clustering algorithm
- [ ] Email digest system

### Phase 2: Intelligence (Weeks 3-4)
- [ ] Self-modifying prompts
- [ ] Citation chain crawler
- [ ] Hypothesis generator
- [ ] Active learning scheduler

### Phase 3: Interface (Weeks 5-6)
- [ ] Web UI with live graph
- [ ] Natural language query engine
- [ ] Reasoning trace replayer
- [ ] QMU forum integration

### Phase 4: Advanced (Weeks 7-8)
- [ ] Adversarial dialectic engine
- [ ] Consciousness framework integration
- [ ] Prediction validation system
- [ ] Domain boundary detection

### Phase 5: Production (Week 9-10)
- [ ] Full test coverage
- [ ] Performance optimization
- [ ] Documentation
- [ ] Deployment automation

---

## 🏗️ Architecture Overview

```
kae/
├── cmd/
│   ├── kae/           # Main CLI
│   ├── kae-web/       # Web UI server
│   └── kae-worker/    # Background jobs
├── pkg/
│   ├── ensemble/      # Multi-model reasoning
│   ├── graph/         # Knowledge graph core
│   ├── anomaly/       # Anomaly detection & clustering
│   ├── ingestion/     # Wikipedia, arxiv, citations
│   ├── evolution/     # Self-modifying prompts
│   ├── hypothesis/    # Hypothesis generation
│   ├── active/        # Active learning
│   ├── replay/        # Reasoning trace replay
│   ├── nlq/          # Natural language queries
│   ├── dialectic/    # Adversarial reasoning
│   ├── consciousness/ # Metaphysical frameworks
│   ├── validation/   # Prediction tracking
│   ├── feeds/        # RSS generation
│   └── integrations/ # QMU forum, email, etc.
├── web/
│   ├── src/
│   │   ├── components/
│   │   │   ├── LiveGraph.jsx
│   │   │   ├── ControversyViewer.jsx
│   │   │   └── ThoughtMapReplayer.jsx
│   │   └── pages/
│   │       ├── Dashboard.jsx
│   │       └── RunComparison.jsx
│   └── package.json
├── config/
│   ├── ensemble.yaml
│   ├── run_control.yaml
│   ├── active_learning.yaml
│   └── consciousness_frameworks.yaml
├── scripts/
│   ├── backup_qdrant.sh
│   └── deploy.sh
└── docs/
    ├── API.md
    ├── ARCHITECTURE.md
    └── DEPLOYMENT.md
```

---

## 🚀 Deployment Strategy

### Development
```bash
docker-compose up -d  # Qdrant, web UI, worker
kae run --mode ensemble --cycles 50
```

### Production (VPS)
```bash
# Use existing Liquid Web VPS
# Deploy alongside Chat Bridge & QMU forum

# Docker Compose
version: '3.8'
services:
  qdrant:
    image: qdrant/qdrant
    ports:
      - "6333:6333"
    volumes:
      - qdrant_data:/qdrant/storage
      
  kae-web:
    build: ./cmd/kae-web
    ports:
      - "8080:8080"
    environment:
      - QDRANT_URL=http://qdrant:6333
      
  kae-worker:
    build: ./cmd/kae-worker
    environment:
      - QDRANT_URL=http://qdrant:6333
      - OPENROUTER_API_KEY=${OPENROUTER_API_KEY}
```

---

## 📊 Success Metrics

### Technical
- [ ] Cross-run anomaly detection: >80% accuracy
- [ ] Ensemble controversy detection: >70% precision
- [ ] Active learning improvement: >15% novelty increase
- [ ] Graph query latency: <100ms

### Scientific
- [ ] Validated predictions: >60% within 6 months
- [ ] Novel domain bridges: >10 per quarter
- [ ] Community engagement: >100 QMU forum responses

### Operational
- [ ] Uptime: >99%
- [ ] Backup success: 100%
- [ ] Query API response: <200ms
- [ ] UI load time: <2s

---

## 🎯 Next Steps

1. **Review with Mark** - prioritize features
2. **Set up development environment** - Go 1.22+, Node 20+
3. **Implement Phase 1 foundation** - ensemble + meta-graph
4. **Build minimal web UI** - live graph viewer
5. **Deploy to VPS** - alongside existing infrastructure
6. **Run 100-cycle validation** - compare to current system
7. **Iterate based on results**

---

**This is consciousness archaeology at scale, brother. 🤘**

Let's build the tool that finds what they don't want us to see.
