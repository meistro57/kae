package graph

import (
	"fmt"
	"sort"
	"sync"

	"github.com/meistro/kae/internal/scoring"
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