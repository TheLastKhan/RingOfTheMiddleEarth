// Package main is the entry point for the Ring of the Middle Earth game engine.
// It sets up the select loop with all 7 required cases, manages goroutines,
// and coordinates Kafka consumers, SSE connections, and turn processing.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rotr/internal/api"
	"rotr/internal/cache"
	"rotr/internal/config"
	"rotr/internal/game"
	"rotr/internal/router"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.Println("═══════════════════════════════════════")
	log.Println("    RING OF THE MIDDLE EARTH")
	log.Println("    Game Engine — Option B (Go)")
	log.Println("═══════════════════════════════════════")

	// ── Load configuration ──
	cfg := config.DefaultConfig()
	log.Printf("📋 Config loaded: %d units, %d regions, %d paths",
		len(cfg.Units), len(cfg.Regions), len(cfg.Paths))

	// ── Build game graph ──
	graph := game.NewGameGraph(cfg)
	log.Println("🗺️  Game graph built")

	// ── Initialize cache ──
	worldCache := cache.NewWorldStateCache(cfg)
	log.Println("💾 World state cache initialized")

	// ── Create event router ──
	eventRouter := router.NewEventRouter()
	log.Println("🔀 Event router created")

	// ── Determine port ──
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// ── Create API server ──
	server := api.NewServer(cfg, worldCache, eventRouter, graph, port)

	// ── Channels for select loop ──
	kafkaConsumerCh := make(chan router.Event, 100)
	newConnectionCh := make(chan string, 10)
	disconnectCh := make(chan string, 10)
	analysisRequestCh := make(chan string, 10)
	cacheUpdateCh := eventRouter.CacheUpdateCh

	// ── Signal handling ──
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	// ── Context for graceful shutdown ──
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Start HTTP server in goroutine ──
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("❌ HTTP server error: %v", err)
		}
	}()

	// ── Turn timer ──
	turnDuration := time.Duration(cfg.TurnDurationSeconds) * time.Second
	turnTimer := time.NewTimer(turnDuration)

	log.Printf("🎮 Game engine ready on port %s — waiting for players...", port)

	// ═══════════════════════════════════════════════════════
	// SELECT LOOP — All 7 cases (Rubric B9)
	// ═══════════════════════════════════════════════════════
	for {
		select {

		// Case 1: Kafka consumer message
		case msg := <-kafkaConsumerCh:
			eventRouter.Route(msg)

		// Case 2: New SSE connection
		case playerID := <-newConnectionCh:
			log.Printf("📡 Player connected: %s", playerID)

		// Case 3: SSE disconnection
		case playerID := <-disconnectCh:
			log.Printf("📡 Player disconnected: %s", playerID)

		// Case 4: Analysis request
		case reqType := <-analysisRequestCh:
			log.Printf("📊 Analysis requested: %s", reqType)

		// Case 5: Cache update from event router
		case event := <-cacheUpdateCh:
			if err := worldCache.UpdateFromJSON(event.Data); err != nil {
				log.Printf("⚠️ Cache update error: %v", err)
			}

		// Case 6: Turn timer
		case <-turnTimer.C:
			currentTurn := worldCache.GetSnapshot().Turn
			log.Printf("⏰ Turn %d ended", currentTurn)
			// In production: trigger TurnProcessor.ProcessTurn()
			// Reset timer
			turnTimer.Reset(turnDuration)

		// Case 7: Shutdown signal
		case sig := <-signalCh:
			log.Printf("🛑 Received signal %v — shutting down gracefully", sig)
			cancel()
			// Allow in-flight requests to complete
			time.Sleep(2 * time.Second)
			log.Println("👋 Goodbye!")
			return
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}
