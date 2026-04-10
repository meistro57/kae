package graph

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/meistro57/kae/internal/scoring"
)

type Node struct {
	ID                 string
	Label              string
	Domain             string
	Sources            []string
	Weight             float64
	Anomaly            bool
	Notes              string
	Vector             []float32
	ContradictionScore *scoring.ContradictionScore
}

type Edge struct {
	From       string
	To         string
	Relation   string
	Confidence float64
	Citation   string // source URLs that support this edge
}

type Graph struct {
	mu    sync.RWMutex
	nodes map[string]*Node
	edges []*Edge
	adj   map[string][]int
}

type snapshot struct {
	Nodes []*Node `json:"nodes"`
	Edges []*Edge `json:"edges"`
}

func New() *Graph {
	return &Graph{
		nodes: make(map[string]*Node),
		edges: make([]*Edge, 0, 256),
		adj:   make(map[string][]int),
	}
}

func (g *Graph) UpsertNode(n *Node) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if ex, ok := g.nodes[n.ID]; ok {
		ex.Weight += n.Weight
		ex.Sources = append(ex.Sources, n.Sources...)
		if n.Anomaly {
			ex.Anomaly = true
		}
		if n.Notes != "" {
			ex.Notes = n.Notes
		}
		if n.ContradictionScore != nil {
			ex.ContradictionScore = n.ContradictionScore
		}
		if len(n.Vector) > 0 {
			ex.Vector = n.Vector
		}
		return
	}
	g.nodes[n.ID] = n
	g.adj[n.ID] = []int{}
}

func (g *Graph) AddEdge(e *Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()
	idx := len(g.edges)
	g.edges = append(g.edges, e)
	g.adj[e.From] = append(g.adj[e.From], idx)
	g.adj[e.To] = append(g.adj[e.To], idx)
	if n, ok := g.nodes[e.From]; ok {
		n.Weight += e.Confidence
	}
	if n, ok := g.nodes[e.To]; ok {
		n.Weight += e.Confidence
	}
}

func (g *Graph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

func (g *Graph) EdgeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.edges)
}

func (g *Graph) AnomalyCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n := 0
	for _, node := range g.nodes {
		if node.Anomaly {
			n++
		}
	}
	return n
}

func (g *Graph) TopNodes(n int) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	list := make([]*Node, 0, len(g.nodes))
	for _, node := range g.nodes {
		list = append(list, node)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Weight > list[j].Weight
	})
	if n > len(list) {
		n = len(list)
	}
	return list[:n]
}

func (g *Graph) AnomalyNodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	result := make([]*Node, 0)
	for _, n := range g.nodes {
		if n.Anomaly {
			result = append(result, n)
		}
	}
	return result
}

// AllNodes returns all nodes — used for cross-run convergence export
func (g *Graph) AllNodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	list := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		list = append(list, n)
	}
	return list
}

func (g *Graph) Summary() string {
	return fmt.Sprintf("Nodes: %d | Edges: %d | Anomalies: %d",
		g.NodeCount(), g.EdgeCount(), g.AnomalyCount())
}

// CleanSummary returns graph stats with junk nodes filtered out
func (g *Graph) CleanSummary() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	junkPhrases := []string{
		"NO SOURCE", "SOURCE VERIFICATION", "SOURCE ACQUISITION",
		"PASSAGES PROVIDED", "PROCESS OR AGENT RESPONSIBLE",
		"NO DOMAINS", "NO CROSS-REFERENCE",
	}

	realNodes := 0
	for _, n := range g.nodes {
		isJunk := false
		upper := strings.ToUpper(n.Label)
		for _, phrase := range junkPhrases {
			if strings.Contains(upper, phrase) {
				isJunk = true
				break
			}
		}
		if !isJunk {
			realNodes++
		}
	}

	return fmt.Sprintf("Nodes: %d | Edges: %d | Anomalies: %d",
		realNodes, len(g.edges), g.AnomalyCount())
}

func (g *Graph) SaveToFile(path string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		copyNode := *n
		nodes = append(nodes, &copyNode)
	}

	edges := make([]*Edge, len(g.edges))
	for i, e := range g.edges {
		copyEdge := *e
		edges[i] = &copyEdge
	}

	data, err := json.MarshalIndent(snapshot{
		Nodes: nodes,
		Edges: edges,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal graph: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write graph snapshot: %w", err)
	}

	return nil
}

func (g *Graph) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read graph snapshot: %w", err)
	}

	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("unmarshal graph snapshot: %w", err)
	}

	nodes := make(map[string]*Node, len(snap.Nodes))
	adj := make(map[string][]int, len(snap.Nodes))
	for _, n := range snap.Nodes {
		copyNode := *n
		nodes[copyNode.ID] = &copyNode
		adj[copyNode.ID] = []int{}
	}

	edges := make([]*Edge, len(snap.Edges))
	for i, e := range snap.Edges {
		copyEdge := *e
		edges[i] = &copyEdge
		adj[e.From] = append(adj[e.From], i)
		adj[e.To] = append(adj[e.To], i)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes = nodes
	g.edges = edges
	g.adj = adj

	return nil
}
