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
	instanceID := os.Getenv("INSTANCE_ID")
	if instanceID == "" {
		instanceID = "local"
	}
	gameOverProducer, err := kafkalite.NewTransactionalProducer(os.Getenv("KAFKA_BROKERS"), "rotr-gameover-"+instanceID)
	if err != nil {
		log.Printf("⚠️  Transactional Kafka producer unavailable (standalone mode): %v", err)
		// Continue without transactional producer — local/demo mode
	}
	if gameOverProducer != nil {
		defer gameOverProducer.Close()
	}
	sessionConsumer := kafkalite.NewConsumer(os.Getenv("KAFKA_BROKERS"))

	kafkaConsumerCh := make(chan router.Event, 100)
	sessionReplayCh := make(chan []byte, 10)
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

	go pollSessionSnapshots(ctx, sessionConsumer, sessionReplayCh)

	var lastSessionTimestamp int64

	publishEvent := func(event game.GameEvent) {
		routed := router.Event{Topic: event.Topic, Key: event.Key, Data: event.Data}
		if isGameOver(event.Data) {
			if gameOverProducer != nil {
				if err := gameOverProducer.ProduceTransaction(context.Background(), event.Topic, event.Key, event.Data); err != nil {
					log.Printf("Kafka transactional GameOver produce failed (standalone OK): %v", err)
				}
			}
			eventRouter.Route(routed)
			return
		}

		eventRouter.Route(routed)
		if err := producer.Produce(event.Topic, event.Key, event.Data); err != nil {
			log.Printf("Kafka event produce failed topic=%s: %v", event.Topic, err)
		}
		if event.Topic == "game.broadcast" && isWorldState(event.Data) {
			if ts := worldStateTimestamp(event.Data); ts > lastSessionTimestamp {
				lastSessionTimestamp = ts
			}
			if err := producer.Produce("game.session", "session", event.Data); err != nil {
				log.Printf("Kafka session produce failed: %v", err)
			}
		}
	}

	processTurn := func(reason string) {
		currentTurn := worldCache.GetSnapshot().Turn
		log.Printf("Turn %d ended by %s with %d pending orders", currentTurn, reason, len(pendingOrders))
		events := turnProcessor.ProcessTurn(turnState, pendingOrders)
		pendingOrders = pendingOrders[:0]
		for _, event := range events {
			publishEvent(event)
		}
		server.ResetTurn()
	}

	stopTurnTimer := func() {
		if !turnTimer.Stop() {
			select {
			case <-turnTimer.C:
			default:
			}
		}
	}

	for {
		select {
		case msg := <-kafkaConsumerCh:
			eventRouter.Route(msg)

		case data := <-sessionReplayCh:
			restored, ts, err := game.InitTurnStateFromJSON(cfg, graph, data)
			if err != nil {
				log.Printf("Session replay error: %v", err)
				break
			}
			if restored == nil || ts <= lastSessionTimestamp {
				break
			}
			turnState = restored
			pendingOrders = pendingOrders[:0]
			if err := worldCache.UpdateFromJSON(data); err != nil {
				log.Printf("Session cache update error: %v", err)
			}
			lastSessionTimestamp = ts
			server.ResetTurn()
			log.Printf("Session replay restored turn %d from Kafka", turnState.Turn)

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
			stopTurnTimer()
			turnState = game.InitTurnState(cfg, graph)
			worldCache.ResetFromConfig(cfg)
			pendingOrders = pendingOrders[:0]
			server.ResetTurn()
			publishEvent(game.MakeWorldSnapshotEvent(turnState))
			gameStarted = true
			turnTimer.Reset(turnDuration)
			log.Printf("New game started: turn reset to 1, first turn ends in %s", turnDuration)
			close(ack)

		case ack := <-server.AdvanceCh:
			if !gameStarted {
				gameStarted = true
			}
			stopTurnTimer()
			processTurn("manual advance")
			if !turnState.GameOver {
				turnTimer.Reset(turnDuration)
			}
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
			processTurn("timer")
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

func isGameOver(data []byte) bool {
	var event struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return false
	}
	return event.Type == "GAME_OVER"
}

func worldStateTimestamp(data []byte) int64 {
	var event struct {
		Timestamp int64 `json:"timestamp"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return 0
	}
	return event.Timestamp
}

func pollSessionSnapshots(ctx context.Context, consumer *kafkalite.Consumer, out chan<- []byte) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var nextOffset int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			messages, err := consumer.FetchAll("game.session", 0, nextOffset)
			if err != nil {
				log.Printf("Kafka session fetch failed: %v", err)
				continue
			}
			if len(messages) == 0 {
				continue
			}
			latest := messages[len(messages)-1]
			if len(latest.Value) == 0 {
				nextOffset = latest.Offset + 1
				continue
			}
			nextOffset = latest.Offset + 1
			select {
			case out <- latest.Value:
			default:
				log.Printf("Kafka session replay channel full; skipping offset %d", latest.Offset)
			}
		}
	}
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
