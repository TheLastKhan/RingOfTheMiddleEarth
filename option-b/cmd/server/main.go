// Package main is the entry point for the Ring of the Middle Earth game engine.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rotr/internal/api"
	"rotr/internal/cache"
	"rotr/internal/config"
	"rotr/internal/game"
	"rotr/internal/kafkalite"
	"rotr/internal/router"
	"rotr/internal/validation"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.Println("=======================================")
	log.Println("    RING OF THE MIDDLE EARTH")
	log.Println("    Game Engine - Option B (Go)")
	log.Println("=======================================")

	cfg := config.DefaultConfig()
	log.Printf("Config loaded: %d units, %d regions, %d paths, %d routes",
		len(cfg.Units), len(cfg.Regions), len(cfg.Paths), len(cfg.CanonicalRoutes))

	graph := game.NewGameGraph(cfg)
	worldCache := cache.NewWorldStateCache(cfg)
	eventRouter := router.NewEventRouter()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := api.NewServer(cfg, worldCache, eventRouter, graph, port)
	turnProcessor := game.NewTurnProcessor(cfg, graph)
	turnState := game.InitTurnState(cfg, graph)
	pendingOrders := make([]game.Order, 0)
	producer := kafkalite.NewProducer(os.Getenv("KAFKA_BROKERS"))

	kafkaConsumerCh := make(chan router.Event, 100)
	newConnectionCh := make(chan string, 10)
	disconnectCh := make(chan string, 10)
	analysisRequestCh := make(chan string, 10)
	cacheUpdateCh := eventRouter.CacheUpdateCh

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	turnDuration := time.Duration(cfg.TurnDurationSeconds) * time.Second
	turnTimer := time.NewTimer(turnDuration)
	if !turnTimer.Stop() {
		<-turnTimer.C
	}
	gameStarted := false

	log.Printf("Game engine ready on port %s", port)

	for {
		select {
		case msg := <-kafkaConsumerCh:
			eventRouter.Route(msg)

		case playerID := <-newConnectionCh:
			log.Printf("Player connected: %s", playerID)

		case playerID := <-disconnectCh:
			log.Printf("Player disconnected: %s", playerID)

		case <-server.StartCh:
			if !gameStarted {
				gameStarted = true
				turnTimer.Reset(turnDuration)
				log.Printf("Turn timer started: first turn ends in %s", turnDuration)
			}

		case ack := <-server.ResetCh:
			if !turnTimer.Stop() {
				select {
				case <-turnTimer.C:
				default:
				}
			}
			turnState = game.InitTurnState(cfg, graph)
			worldCache.ResetFromConfig(cfg)
			pendingOrders = pendingOrders[:0]
			server.ResetTurn()
			gameStarted = true
			turnTimer.Reset(turnDuration)
			log.Printf("New game started: turn reset to 1, first turn ends in %s", turnDuration)
			close(ack)

		case reqType := <-analysisRequestCh:
			log.Printf("Analysis requested: %s", reqType)

		case event := <-cacheUpdateCh:
			if err := worldCache.UpdateFromJSON(event.Data); err != nil {
				log.Printf("Cache update error: %v", err)
			}

		case order := <-server.OrderCh:
			pendingOrders = append(pendingOrders, toGameOrder(order))
			data, _ := json.Marshal(order)
			if err := producer.Produce("game.orders.raw", order.PlayerID, data); err != nil {
				log.Printf("Kafka raw order produce failed: %v", err)
			}
			if err := producer.Produce("game.orders.validated", order.UnitID, data); err != nil {
				log.Printf("Kafka validated order produce failed: %v", err)
			}

		case <-turnTimer.C:
			currentTurn := worldCache.GetSnapshot().Turn
			log.Printf("Turn %d ended with %d pending orders", currentTurn, len(pendingOrders))
			events := turnProcessor.ProcessTurn(turnState, pendingOrders)
			pendingOrders = pendingOrders[:0]
			for _, event := range events {
				routed := router.Event{Topic: event.Topic, Key: event.Key, Data: event.Data}
				eventRouter.Route(routed)
				if err := producer.Produce(event.Topic, event.Key, event.Data); err != nil {
					log.Printf("Kafka event produce failed topic=%s: %v", event.Topic, err)
				}
				if event.Topic == "game.broadcast" && isWorldState(event.Data) {
					if err := producer.Produce("game.session", "session", event.Data); err != nil {
						log.Printf("Kafka session produce failed: %v", err)
					}
				}
			}
			server.ResetTurn()
			if !turnState.GameOver {
				turnTimer.Reset(turnDuration)
			}

		case sig := <-signalCh:
			log.Printf("Received signal %v - shutting down gracefully", sig)
			cancel()
			time.Sleep(2 * time.Second)
			log.Println("Goodbye!")
			return
		}

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func isWorldState(data []byte) bool {
	var event struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return false
	}
	return event.Type == "WORLD_STATE"
}

func toGameOrder(order validation.Order) game.Order {
	return game.Order{
		OrderType:    order.OrderType,
		PlayerID:     order.PlayerID,
		UnitID:       order.UnitID,
		PathID:       order.PathID,
		PathIDs:      order.PathIDs,
		TargetRegion: order.TargetRegion,
		TargetPathID: order.TargetPathID,
	}
}
