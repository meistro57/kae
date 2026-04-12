package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/meistro/kae/internal/config"
	"github.com/meistro/kae/internal/graph"
	qdrantclient "github.com/meistro/kae/internal/qdrantclient"
)

//go:embed static
var staticFiles embed.FS

// Server is the KAE Lens web dashboard HTTP server.
type Server struct {
	cfg    *config.LensConfig
	broker *Broker
	qc     *qdrantclient.Client
	stats  *StatsTracker
}

// NewServer creates a new web Server.
func NewServer(cfg *config.LensConfig, broker *Broker, qc *qdrantclient.Client) *Server {
	return &Server{
		cfg:    cfg,
		broker: broker,
		qc:     qc,
		stats:  NewStatsTracker(),
	}
}

// Start launches the HTTP server. Blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Serve embedded static files (dashboard HTML/JS/CSS)
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("sub static fs: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// SSE events stream
	mux.HandleFunc(s.cfg.Web.SSEPath, s.handleSSE)

	// REST endpoints for dashboard data
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/findings", s.handleFindings)
	mux.HandleFunc("/api/health", s.handleHealth)

	addr := fmt.Sprintf(":%d", s.cfg.Web.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Shutdown when context is cancelled
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("[web] shutdown error: %v", err)
		}
	}()

	log.Printf("[web] dashboard running at http://localhost%s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// handleSSE handles SSE client connections.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", s.cfg.Web.CORSOrigin)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Register client
	clientChan := s.broker.NewClientChannel()
	defer s.broker.UnregisterClient(clientChan)

	// Send current stats immediately on connect
	statsData, _ := json.Marshal(s.stats.Current())
	fmt.Fprintf(w, "event: stats\ndata: %s\n\n", statsData)
	flusher.Flush()

	// Heartbeat ticker to keep the connection alive
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	clientGone := r.Context().Done()

	for {
		select {
		case <-clientGone:
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		case msg, ok := <-clientChan:
			if !ok {
				return
			}
			w.Write(msg)
			flusher.Flush()
		}
	}
}

// handleStats returns current stats as JSON.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", s.cfg.Web.CORSOrigin)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats := s.stats.Current()

	// Enrich with live Qdrant counts
	if info, err := s.qc.GetCollectionInfo(ctx, s.cfg.Qdrant.KnowledgeCollection); err == nil {
		if info.PointsCount != nil {
			stats.TotalKnowledgePoints = int64(*info.PointsCount)
		}
	}
	if info, err := s.qc.GetCollectionInfo(ctx, s.cfg.Qdrant.FindingsCollection); err == nil {
		if info.PointsCount != nil {
			stats.TotalFindings = int64(*info.PointsCount)
		}
	}

	json.NewEncoder(w).Encode(stats)
}

// handleFindings returns recent findings from kae_lens_findings as JSON.
func (s *Server) handleFindings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", s.cfg.Web.CORSOrigin)
	json.NewEncoder(w).Encode(s.stats.RecentFindings())
}

// handleHealth returns a simple health check.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"clients": s.broker.ClientCount(),
		"time":    time.Now().UTC(),
	})
}

// HandleEvent processes an event from the internal event bus and routes it
// to the SSE broker and stats tracker.
func (s *Server) HandleEvent(event any) {
	switch e := event.(type) {
	case graph.FindingEvent:
		s.stats.RecordFinding(e)
		s.broker.Publish(graph.EventTypeFinding, e)

	case graph.StatsEvent:
		s.broker.Publish(graph.EventTypeStats, e)

	case graph.BatchStartEvent:
		s.stats.SetActiveBatch(true, e.BatchID, e.PointCount)
		s.broker.Publish(graph.EventTypeBatchStart, e)

	case graph.BatchDoneEvent:
		s.stats.SetActiveBatch(false, "", 0)
		s.broker.Publish(graph.EventTypeBatchDone, e)
	}
}
