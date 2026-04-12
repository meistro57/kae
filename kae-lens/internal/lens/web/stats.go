package web

import (
	"sync"

	"github.com/meistro/kae/collections"
	"github.com/meistro/kae/internal/graph"
)

// StatsTracker maintains in-memory dashboard statistics.
type StatsTracker struct {
	mu             sync.RWMutex
	stats          graph.StatsEvent
	recentFindings []graph.FindingEvent
	maxFindings    int
}

// NewStatsTracker creates a StatsTracker.
func NewStatsTracker() *StatsTracker {
	return &StatsTracker{
		maxFindings: 200,
	}
}

// RecordFinding updates stats and appends to the recent findings ring buffer.
func (t *StatsTracker) RecordFinding(f graph.FindingEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.stats.FindingsInSession++

	switch collections.FindingType(f.Type) {
	case collections.FindingConnection:
		t.stats.FindingsByType.Connections++
	case collections.FindingContradiction:
		t.stats.FindingsByType.Contradictions++
	case collections.FindingCluster:
		t.stats.FindingsByType.Clusters++
	case collections.FindingAnomaly:
		t.stats.FindingsByType.Anomalies++
	}

	// Ring buffer — keep last N findings
	t.recentFindings = append(t.recentFindings, f)
	if len(t.recentFindings) > t.maxFindings {
		t.recentFindings = t.recentFindings[len(t.recentFindings)-t.maxFindings:]
	}
}

// RecordProcessed increments the processed-in-session counter.
func (t *StatsTracker) RecordProcessed(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stats.ProcessedInSession += n
}

// SetActiveBatch updates the batch progress display.
func (t *StatsTracker) SetActiveBatch(active bool, batchID string, count int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stats.ActiveBatch = active
	if active {
		t.stats.BatchProgress = batchID
	} else {
		t.stats.BatchProgress = ""
	}
}

// Current returns a copy of the current stats.
func (t *StatsTracker) Current() graph.StatsEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.stats
}

// RecentFindings returns a copy of the recent findings slice.
func (t *StatsTracker) RecentFindings() []graph.FindingEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]graph.FindingEvent, len(t.recentFindings))
	copy(result, t.recentFindings)
	return result
}
