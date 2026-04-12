package graph

import "time"

// Node represents a knowledge point in the reasoning graph.
type Node struct {
	ID        string
	Title     string
	Domain    string
	Summary   string
	Score     float32 // similarity score relative to anchor (0 for anchor itself)
	IsAnchor  bool
}

// Edge represents a discovered relationship between two nodes.
type Edge struct {
	FromID     string
	ToID       string
	Type       string  // "connection", "contradiction", "cluster", "anomaly"
	Confidence float64
	Label      string
}

// FindingEvent is the event emitted by Lens when a new finding is produced.
// Flows from the reasoner → TUI channel and SSE broker simultaneously.
type FindingEvent struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	Confidence     float64   `json:"confidence"`
	SourceIDs      []string  `json:"source_ids"`
	Domains        []string  `json:"domains"`
	Summary        string    `json:"summary"`
	ReasoningTrace string    `json:"reasoning_trace"`
	CreatedAt      time.Time `json:"created_at"`
	BatchID        string    `json:"batch_id"`
}

// StatsEvent carries updated dashboard stats.
type StatsEvent struct {
	TotalKnowledgePoints int64   `json:"total_knowledge_points"`
	TotalFindings        int64   `json:"total_findings"`
	FindingsByType       TypeMap `json:"findings_by_type"`
	ProcessedInSession   int     `json:"processed_in_session"`
	FindingsInSession    int     `json:"findings_in_session"`
	ActiveBatch          bool    `json:"active_batch"`
	BatchProgress        string  `json:"batch_progress"`
}

// TypeMap tracks counts per finding type.
type TypeMap struct {
	Connections    int `json:"connections"`
	Contradictions int `json:"contradictions"`
	Clusters       int `json:"clusters"`
	Anomalies      int `json:"anomalies"`
}

// BatchStartEvent is emitted when Lens begins processing a new batch.
type BatchStartEvent struct {
	BatchID    string `json:"batch_id"`
	PointCount int    `json:"point_count"`
}

// BatchDoneEvent is emitted when a batch completes.
type BatchDoneEvent struct {
	BatchID       string `json:"batch_id"`
	FindingsCount int    `json:"findings_count"`
	DurationMs    int64  `json:"duration_ms"`
}

// EventType constants for SSE event naming.
const (
	EventTypeFinding    = "finding"
	EventTypeStats      = "stats"
	EventTypeBatchStart = "batch_start"
	EventTypeBatchDone  = "batch_done"
)
