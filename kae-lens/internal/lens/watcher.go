package lens

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/meistro/kae/internal/config"
	"github.com/meistro/kae/internal/graph"
	qdrantclient "github.com/meistro/kae/internal/qdrantclient"
)

// Watcher polls kae_knowledge for unprocessed points and dispatches them
// to the Reasoner in batches. It is the entry point of the Lens pipeline.
type Watcher struct {
	cfg      *config.LensConfig
	qc       *qdrantclient.Client
	reasoner *Reasoner
	events   chan<- any // outbound event channel (findings, stats, batch events)

	activeBatches int
	mu            sync.Mutex
}

// NewWatcher creates a Watcher.
func NewWatcher(cfg *config.LensConfig, qc *qdrantclient.Client, reasoner *Reasoner, events chan<- any) *Watcher {
	return &Watcher{
		cfg:      cfg,
		qc:       qc,
		reasoner: reasoner,
		events:   events,
	}
}

// Run starts the polling loop. Blocks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	interval := time.Duration(w.cfg.Watcher.PollIntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Stats ticker - emit stats every 5 seconds
	statsTicker := time.NewTicker(5 * time.Second)
	defer statsTicker.Stop()

	log.Printf("[watcher] started — polling every %s, batch size %d",
		interval, w.cfg.Watcher.BatchSize)

	// Emit initial stats
	w.emitStats(ctx)

	// Run once immediately on start
	w.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("[watcher] shutting down")
			return
		case <-ticker.C:
			w.poll(ctx)
		case <-statsTicker.C:
			w.emitStats(ctx)
		}
	}
}

// poll fetches unprocessed points and dispatches them if capacity allows.
func (w *Watcher) poll(ctx context.Context) {
	w.mu.Lock()
	if w.activeBatches >= w.cfg.Watcher.MaxConcurrentBatches {
		w.mu.Unlock()
		log.Println("[watcher] max concurrent batches reached, skipping poll")
		return
	}
	w.mu.Unlock()

	points, err := w.qc.ScrollUnprocessed(
		ctx,
		w.cfg.Qdrant.KnowledgeCollection,
		uint32(w.cfg.Watcher.BatchSize),
	)
	if err != nil {
		log.Printf("[watcher] error fetching unprocessed points: %v", err)
		return
	}
	if len(points) == 0 {
		return
	}

	log.Printf("[watcher] found %d unprocessed points — dispatching batch", len(points))

	// Extract IDs and mark processed optimistically before reasoning starts.
	// This prevents duplicate processing if the watcher fires again before
	// the batch completes. IDs may be UUIDs or numeric (kae_chunks uses uint64).
	ids := make([]string, 0, len(points))
	for _, p := range points {
		if p.Id != nil {
			ids = append(ids, qdrantclient.PointIDStr(p.Id))
		}
	}

	if err := w.qc.MarkProcessed(ctx, w.cfg.Qdrant.KnowledgeCollection, ids); err != nil {
		log.Printf("[watcher] failed to mark points processed: %v", err)
		// Continue anyway — worst case we process twice, which is harmless
	}

	w.mu.Lock()
	w.activeBatches++
	w.mu.Unlock()

	// Dispatch batch in a goroutine so the watcher loop stays unblocked
	go func() {
		defer func() {
			w.mu.Lock()
			w.activeBatches--
			w.mu.Unlock()
		}()

		batchEvent := graph.BatchStartEvent{
			BatchID:    newBatchID(),
			PointCount: len(points),
		}
		w.emit(batchEvent)

		start := time.Now()
		findingsCount, err := w.reasoner.ProcessBatch(ctx, batchEvent.BatchID, points)
		if err != nil {
			log.Printf("[watcher] batch %s error: %v", batchEvent.BatchID, err)
		}

		w.emit(graph.BatchDoneEvent{
			BatchID:       batchEvent.BatchID,
			FindingsCount: findingsCount,
			DurationMs:    time.Since(start).Milliseconds(),
		})

		log.Printf("[watcher] batch %s done — %d findings in %s",
			batchEvent.BatchID, findingsCount, time.Since(start).Round(time.Millisecond))
	}()
}

func (w *Watcher) emit(event any) {
	select {
	case w.events <- event:
	default:
		log.Println("[watcher] event channel full, dropping event")
	}
}

// emitStats queries Qdrant for current collection stats and emits a StatsEvent.
func (w *Watcher) emitStats(ctx context.Context) {
	// Get knowledge collection stats
	knowledgeInfo, err := w.qc.GetCollectionInfo(ctx, w.cfg.Qdrant.KnowledgeCollection)
	if err != nil {
		log.Printf("[watcher] failed to get knowledge collection stats: %v", err)
		return
	}

	// Get findings collection stats
	findingsInfo, err := w.qc.GetCollectionInfo(ctx, w.cfg.Qdrant.FindingsCollection)
	if err != nil {
		log.Printf("[watcher] failed to get findings collection stats: %v", err)
		return
	}

	stats := graph.StatsEvent{
		TotalKnowledgePoints: int64(knowledgeInfo.GetPointsCount()),
		TotalFindings:        int64(findingsInfo.GetPointsCount()),
		// ProcessedInSession and FindingsInSession are tracked by the TUI itself
		// We just send the totals here
	}

	w.emit(stats)
}

// newBatchID generates a short timestamp-based batch identifier.
func newBatchID() string {
	return time.Now().Format("20060102-150405.000")
}
