package graph

import (
	"fmt"
	"sort"
	"sync"
)

type Node struct {
	ID      string
	Label   string
	Domain  string
	Sources []string
	Weight  float64
	Anomaly bool // consensus gap detected
}

type Edge struct {
	From       string
	To         string
	Relation   string
	Confidence float64
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

// score returns a node's composite ranking score.
// Combines accumulated weight with degree (edge count) so concepts that
// are repeatedly connected across many cycles rise above one-time mentions.
func (g *Graph) score(n *Node) float64 {
	return n.Weight + float64(len(g.adj[n.ID]))*0.7
}

func (g *Graph) TopNodes(n int) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	list := make([]*Node, 0, len(g.nodes))
	for _, node := range g.nodes {
		list = append(list, node)
	}
	sort.Slice(list, func(i, j int) bool {
		return g.score(list[i]) > g.score(list[j])
	})
	if n > len(list) {
		n = len(list)
	}
	return list[:n]
}

// NodeScore returns the composite score for a node ID (0 if not found).
func (g *Graph) NodeScore(id string) float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[id]
	if !ok {
		return 0
	}
	return g.score(n)
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

func (g *Graph) Summary() string {
	return fmt.Sprintf("Nodes: %d | Edges: %d | Anomalies: %d",
		g.NodeCount(), g.EdgeCount(), g.AnomalyCount())
}
