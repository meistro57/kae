package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/meistro/kae/collections"
	"github.com/meistro/kae/internal/config"
	"github.com/meistro/kae/internal/graph"
	"github.com/meistro/kae/internal/lens"
	"github.com/meistro/kae/internal/lens/tui"
	"github.com/meistro/kae/internal/lens/web"
	"github.com/meistro/kae/internal/llm"
	qdrantclient "github.com/meistro/kae/internal/qdrantclient"
	"github.com/qdrant/go-client/qdrant"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// ── Flags ──
	configPath := flag.String("config", "config/lens.yaml", "path to lens config file")
	noTUI := flag.Bool("no-tui", false, "disable TUI, log to stdout only")
	once := flag.Bool("once", false, "process all unprocessed points once then exit (no daemon loop)")
	reprocess := flag.Bool("reprocess", false, "clear all processed flags and re-scan everything (implies --once)")
	flag.Parse()

	// ── Logging setup ──
	// If TUI is enabled, redirect logs to file so they don't interfere with display
	if !*noTUI {
		logFile, err := os.OpenFile("lens.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("[lens] failed to open log file: %v", err)
		}
		defer logFile.Close()
		log.SetOutput(logFile)
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	}

	// ── Config ──
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[lens] config error: %v", err)
	}

	// ── Context with graceful shutdown ──
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		log.Println("[lens] shutting down...")
		cancel()
	}()

	// ── Qdrant client ──
	qc, err := qdrantclient.New(qdrantclient.Config{
		Host:   cfg.Qdrant.Host,
		Port:   cfg.Qdrant.Port,
		APIKey: cfg.Qdrant.APIKey,
	})
	if err != nil {
		log.Fatalf("[lens] qdrant client error: %v", err)
	}
	defer qc.Close()

	// ── Ensure collections exist ──
	log.Println("[lens] ensuring Qdrant collections exist...")
	if err := qc.EnsureCollection(ctx, cfg.Qdrant.KnowledgeCollection, uint64(cfg.Embedding.Dimensions)); err != nil {
		log.Fatalf("[lens] ensure knowledge collection: %v", err)
	}
	if err := qc.EnsureCollection(ctx, cfg.Qdrant.FindingsCollection, uint64(cfg.Embedding.Dimensions)); err != nil {
		log.Fatalf("[lens] ensure findings collection: %v", err)
	}

	// Create payload indexes for fast filtering
	ensureIndex := func(collection, field string, ft qdrant.FieldType) {
		if err := qc.CreatePayloadIndex(ctx, collection, field, ft); err != nil {
			log.Printf("[lens] index %s.%s: %v (may already exist)", collection, field, err)
		}
	}
	ensureIndex(cfg.Qdrant.KnowledgeCollection, "lens_processed", qdrant.FieldType_FieldTypeBool)
	ensureIndex(cfg.Qdrant.KnowledgeCollection, "domain", qdrant.FieldType_FieldTypeKeyword)
	ensureIndex(cfg.Qdrant.KnowledgeCollection, "ingested_at", qdrant.FieldType_FieldTypeInteger)
	ensureIndex(cfg.Qdrant.FindingsCollection, "type", qdrant.FieldType_FieldTypeKeyword)
	ensureIndex(cfg.Qdrant.FindingsCollection, "confidence", qdrant.FieldType_FieldTypeFloat)
	ensureIndex(cfg.Qdrant.FindingsCollection, "created_at", qdrant.FieldType_FieldTypeInteger)
	ensureIndex(cfg.Qdrant.FindingsCollection, "reviewed", qdrant.FieldType_FieldTypeBool)
	ensureIndex(cfg.Qdrant.FindingsCollection, "batch_id", qdrant.FieldType_FieldTypeKeyword)

	// ── LLM client ──
	llmClient := llm.New(llm.Config{
		OpenRouterAPIKey:  cfg.LLM.OpenRouterAPIKey,
		OpenRouterBaseURL: cfg.LLM.OpenRouterBaseURL,
		ReasoningModel:    cfg.LLM.ReasoningModel,
		FastModel:         cfg.LLM.FastModel,
		OpenAIAPIKey:      cfg.Embedding.OpenAIAPIKey,
		EmbeddingModel:    cfg.Embedding.Model,
	})

	// ── Internal event bus ──
	// All pipeline components emit to this channel.
	// The event dispatcher reads from it and routes to TUI + web.
	events := make(chan any, 128)

	// ── Build Lens pipeline ──
	density := lens.NewDensityCalculator(cfg, qc)
	synthesizer := lens.NewSynthesizer(llmClient, cfg)
	writer := lens.NewWriter(qc, llmClient, cfg.Qdrant.FindingsCollection, cfg.Qdrant.KnowledgeCollection)
	reasoner := lens.NewReasoner(qc, density, synthesizer, writer, events, cfg.Qdrant.KnowledgeCollection)
	watcher := lens.NewWatcher(cfg, qc, reasoner, events)

	// ── Manual run mode (--once / --reprocess) ────────────────────────────────
	if *once || *reprocess {
		if *reprocess {
			fmt.Fprintf(os.Stderr, "[lens] --reprocess: clearing processed flags on %q...\n",
				cfg.Qdrant.KnowledgeCollection)
			if err := qc.ClearProcessedFlags(ctx, cfg.Qdrant.KnowledgeCollection); err != nil {
				log.Fatalf("[lens] clear processed flags: %v", err)
			}
			fmt.Fprintf(os.Stderr, "[lens] flags cleared — starting full scan\n")
		}

		n, err := watcher.RunOnce(ctx)
		if err != nil && err != context.Canceled {
			log.Printf("[lens] run-once error: %v", err)
		}
		fmt.Fprintf(os.Stderr, "[lens] manual run complete — %d findings written\n", n)
		return
	}

	// ── Web dashboard ──
	broker := web.NewBroker()
	webServer := web.NewServer(cfg, broker, qc)

	// ── TUI model ──
	tuiModel := tui.NewModel(cfg.TUI.MaxFeedItems)

	// ── Event dispatcher goroutine ──
	// Single consumer: routes events to web server and TUI.
	var dispatchWg sync.WaitGroup
	dispatchWg.Add(1)
	go func() {
		defer dispatchWg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				// Route to web
				webServer.HandleEvent(event)

				// Route to TUI
				switch e := event.(type) {
				case graph.FindingEvent:
					tuiModel.SendFinding(e)
				case graph.BatchStartEvent:
					tuiModel.SendBatchStart(e)
				case graph.BatchDoneEvent:
					tuiModel.SendBatchDone(e)
				case graph.StatsEvent:
					tuiModel.SendStats(e)
				}
			}
		}
	}()

	// ── Launch goroutines ──
	var wg sync.WaitGroup

	// Web server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := webServer.Start(ctx); err != nil {
			log.Printf("[lens] web server: %v", err)
		}
	}()

	// Watcher (the main agent loop)
	wg.Add(1)
	go func() {
		defer wg.Done()
		watcher.Run(ctx)
	}()

	// ── TUI or plain logging ──
	if cfg.TUI.Enabled && !*noTUI {
		p := tea.NewProgram(
			tuiModel,
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)
		tuiModel.SetProgram(p)

		if _, err := p.Run(); err != nil {
			log.Printf("[lens] TUI error: %v", err)
		}
		// TUI exited → cancel context to stop everything
		cancel()
	}

	// Wait for all goroutines to finish
	wg.Wait()
	dispatchWg.Wait()
	log.Println("[lens] stopped")

	// suppress unused import
	_ = collections.KnowledgeCollectionName
}
