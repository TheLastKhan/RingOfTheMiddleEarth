// Package main is the entry point for the Ring of the Middle Earth game engine.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
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

type controlEvent struct {
	Type      string `json:"type"`
	Owner     string `json:"owner,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	Timestamp int64  `json:"timestamp"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
}

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.Println("=======================================")
	log.Println("    RING OF THE MIDDLE EARTH")
	log.Println("    Game Engine - Option B (Go)")
	log.Println("=======================================")

	// Bootstrap the engine from the embedded demo configuration.
	// This gives every Go instance the same unit, region, path, and route data.
	cfg := config.DefaultConfig()
	log.Printf("Config loaded: %d units, %d regions, %d paths, %d routes",
		len(cfg.Units), len(cfg.Regions), len(cfg.Paths), len(cfg.CanonicalRoutes))

	// Runtime collaborators:
	// - graph answers map-distance and path-endpoint questions,
	// - worldCache serves side-specific HTTP state,
	// - eventRouter fans events out to Light SSE, Dark SSE, and cache updates.
	graph := game.NewGameGraph(cfg)
	worldCache := cache.NewWorldStateCache(cfg)
	eventRouter := router.NewEventRouter()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// The HTTP server accepts browser commands, but the main goroutine owns
	// mutable turn state. Browser orders cross that boundary through channels.
	server := api.NewServer(cfg, worldCache, eventRouter, graph, port)
	turnProcessor := game.NewTurnProcessor(cfg, graph)
	turnState := game.InitTurnState(cfg, graph)
	pendingOrders := make(map[string]validation.Order)
	// Normal events use a lightweight producer. Terminal GameOver events use a
	// transactional producer so read_committed smoke tests can verify duplicates
	// are not committed.
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

	// Channels are the engine's internal wiring. They keep HTTP handlers,
	// Kafka polling, cache updates, turn advancement, and shutdown decoupled.
	kafkaConsumerCh := make(chan router.Event, 100)
	sessionReplayCh := make(chan []byte, 10)
	orderReplayCh := make(chan validation.Order, 100)
	controlCh := make(chan controlEvent, 100)
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
	go pollValidatedOrders(ctx, sessionConsumer, orderReplayCh)
	go pollControlEvents(ctx, sessionConsumer, controlCh)

	var lastSessionTimestamp int64
	ownerID := ""
	ownerExpiresAt := int64(0)
	ownerTTL := 8 * time.Second
	ownerHeartbeat := time.NewTicker(2 * time.Second)
	defer ownerHeartbeat.Stop()
	ownerCheck := time.NewTicker(time.Second)
	defer ownerCheck.Stop()
	startedAt := time.Now().UnixMilli()

	nowMillis := func() int64 {
		return time.Now().UnixMilli()
	}

	isOwner := func() bool {
		return ownerID == instanceID && ownerExpiresAt > nowMillis()
	}

	claimDelay := func() time.Duration {
		switch instanceID {
		case "go-1":
			return 0
		case "go-2":
			return time.Second
		case "go-3":
			return 2 * time.Second
		default:
			return 1500 * time.Millisecond
		}
	}

	produceControl := func(event controlEvent) {
		data, _ := json.Marshal(event)
		if err := producer.Produce("game.broadcast", "turn-control", data); err != nil {
			log.Printf("Kafka turn-control produce failed: %v", err)
		}
	}

	claimOwner := func(reason string) {
		now := nowMillis()
		ownerID = instanceID
		ownerExpiresAt = now + ownerTTL.Milliseconds()
		produceControl(controlEvent{
			Type:      "TURN_OWNER",
			Owner:     instanceID,
			Timestamp: now,
			ExpiresAt: ownerExpiresAt,
		})
		log.Printf("Turn processor owner=%s reason=%s expiresAt=%d", instanceID, reason, ownerExpiresAt)
	}

	refreshOwner := func() {
		if !isOwner() {
			return
		}
		now := nowMillis()
		ownerExpiresAt = now + ownerTTL.Milliseconds()
		produceControl(controlEvent{
			Type:      "TURN_OWNER",
			Owner:     instanceID,
			Timestamp: now,
			ExpiresAt: ownerExpiresAt,
		})
	}

	var applyControl func(controlEvent)

	addPendingOrder := func(order validation.Order, source string) {
		key := orderFingerprint(order)
		if _, exists := pendingOrders[key]; exists {
			return
		}
		pendingOrders[key] = order
		log.Printf("Pending order accepted source=%s turn=%d unit=%s type=%s total=%d",
			source, order.Turn, order.UnitID, order.OrderType, len(pendingOrders))
	}

	// publishEvent is the central exit point for events produced by turn
	// processing. WORLD_STATE broadcasts are also mirrored to game.session for
	// restart/replay convergence.
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

	// processTurn drains accepted orders into the deterministic 13-step turn
	// processor, publishes resulting events, and resets per-turn validation.
	processTurn := func(reason string) {
		currentTurn := turnState.Turn
		currentOrders := drainTurnOrders(pendingOrders, currentTurn)
		log.Printf("Turn %d ended by %s owner=%s with %d pending orders",
			currentTurn, reason, instanceID, len(currentOrders))
		events := turnProcessor.ProcessTurn(turnState, currentOrders)
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

	applyControl = func(event controlEvent) {
		switch event.Type {
		case "TURN_OWNER":
			if event.ExpiresAt >= ownerExpiresAt {
				ownerID = event.Owner
				ownerExpiresAt = event.ExpiresAt
			}
		case "ADVANCE_REQUEST":
			if event.Timestamp < startedAt || nowMillis()-event.Timestamp > int64((15*time.Second).Milliseconds()) {
				return
			}
			if isOwner() {
				stopTurnTimer()
				processTurn("remote advance request")
				if !turnState.GameOver {
					turnTimer.Reset(turnDuration)
				}
			}
		}
	}

	handleStart := func() {
		if !gameStarted {
			gameStarted = true
			if ownerID == "" || ownerExpiresAt <= nowMillis() {
				claimOwner("game start")
			}
			turnTimer.Reset(turnDuration)
			log.Printf("Turn timer started: first turn ends in %s", turnDuration)
		}
	}

	// handleReset rebuilds both mutable TurnState and read-side cache from
	// config, publishes an initial snapshot, and restarts the timer at Turn 1.
	handleReset := func(ack chan struct{}) {
		stopTurnTimer()
		turnState = game.InitTurnState(cfg, graph)
		worldCache.ResetFromConfig(cfg)
		pendingOrders = make(map[string]validation.Order)
		server.ResetTurn()
		claimOwner("game reset")
		publishEvent(game.MakeWorldSnapshotEvent(turnState))
		gameStarted = true
		turnTimer.Reset(turnDuration)
		log.Printf("New game started: turn reset to 1, first turn ends in %s", turnDuration)
		close(ack)
	}

	// handleAdvance powers the End Turn button and smoke tests. It resolves the
	// current turn immediately and restarts the timer only if the game continues.
	handleAdvance := func(ack chan struct{}) {
		if !gameStarted {
			gameStarted = true
		}
		if !isOwner() {
			if ownerID == "" || ownerExpiresAt <= nowMillis() {
				claimOwner("manual advance no active owner")
			} else {
				produceControl(controlEvent{
					Type:      "ADVANCE_REQUEST",
					RequestID: fmt.Sprintf("%s-%d", instanceID, nowMillis()),
					Timestamp: nowMillis(),
				})
				log.Printf("Forwarded manual advance request to owner=%s", ownerID)
				close(ack)
				return
			}
		}
		stopTurnTimer()
		processTurn("manual advance")
		if !turnState.GameOver {
			turnTimer.Reset(turnDuration)
		}
		close(ack)
	}

	// Main engine loop. The small non-blocking select at the top gives manual
	// start/reset/advance signals priority over background Kafka/cache traffic.
	for {
		select {
		case <-server.StartCh:
			handleStart()
			continue
		case ack := <-server.ResetCh:
			handleReset(ack)
			continue
		case ack := <-server.AdvanceCh:
			handleAdvance(ack)
			continue
		default:
		}

		select {
		case msg := <-kafkaConsumerCh:
			eventRouter.Route(msg)

		case order := <-orderReplayCh:
			addPendingOrder(order, "kafka")

		case control := <-controlCh:
			applyControl(control)

		case data := <-sessionReplayCh:
			// Rebuild mutable state from the newest game.session snapshot.
			// Older snapshots are ignored by timestamp so they cannot rewind state.
			restored, ts, err := game.InitTurnStateFromJSON(cfg, graph, data)
			if err != nil {
				log.Printf("Session replay error: %v", err)
				break
			}
			if restored == nil || ts <= lastSessionTimestamp {
				break
			}
			turnState = restored
			pendingOrders = make(map[string]validation.Order)
			if err := worldCache.UpdateFromJSON(data); err != nil {
				log.Printf("Session cache update error: %v", err)
			}
			lastSessionTimestamp = ts
			server.ResetTurn()
			if !restored.GameOver {
				gameStarted = true
				stopTurnTimer()
				turnTimer.Reset(turnDuration)
			}
			log.Printf("Session replay restored turn %d from Kafka", turnState.Turn)

		case playerID := <-newConnectionCh:
			log.Printf("Player connected: %s", playerID)

		case playerID := <-disconnectCh:
			log.Printf("Player disconnected: %s", playerID)

		case <-server.StartCh:
			handleStart()

		case ack := <-server.ResetCh:
			handleReset(ack)

		case ack := <-server.AdvanceCh:
			handleAdvance(ack)

		case reqType := <-analysisRequestCh:
			log.Printf("Analysis requested: %s", reqType)

		case event := <-cacheUpdateCh:
			if err := worldCache.UpdateFromJSON(event.Data); err != nil {
				log.Printf("Cache update error: %v", err)
			}

		case order := <-server.OrderCh:
			// Accepted HTTP orders enter the current turn here and are also
			// mirrored to Kafka so the demo can inspect order topics.
			addPendingOrder(order, "http")
			data, _ := json.Marshal(order)
			if err := producer.Produce("game.orders.raw", order.PlayerID, data); err != nil {
				log.Printf("Kafka raw order produce failed: %v", err)
			}
			if err := producer.Produce("game.orders.validated", order.UnitID, data); err != nil {
				log.Printf("Kafka validated order produce failed: %v", err)
			}

		case <-turnTimer.C:
			if isOwner() {
				processTurn("timer")
			} else {
				log.Printf("Timer tick skipped by non-owner instance=%s owner=%s expiresAt=%d", instanceID, ownerID, ownerExpiresAt)
			}
			if !turnState.GameOver {
				turnTimer.Reset(turnDuration)
			}

		case <-ownerHeartbeat.C:
			refreshOwner()

		case <-ownerCheck.C:
			if gameStarted && !turnState.GameOver && (ownerID == "" || ownerExpiresAt <= nowMillis()) {
				if time.Since(time.UnixMilli(ownerExpiresAt)) >= claimDelay() {
					claimOwner("owner lease expired")
				}
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
	// Polling keeps the demo implementation simple: every engine periodically
	// reads game.session and forwards the newest unseen snapshot to the main loop.
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

func pollValidatedOrders(ctx context.Context, consumer *kafkalite.Consumer, out chan<- validation.Order) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	nextOffset, err := consumer.LatestOffset("game.orders.validated", 0)
	if err != nil {
		log.Printf("Kafka validated-order latest offset lookup failed: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			messages, err := consumer.FetchAll("game.orders.validated", 0, nextOffset)
			if err != nil {
				log.Printf("Kafka validated-order fetch failed: %v", err)
				continue
			}
			if len(messages) == 0 {
				continue
			}
			nextOffset = messages[len(messages)-1].Offset + 1
			for _, msg := range messages {
				var order validation.Order
				if err := json.Unmarshal(msg.Value, &order); err != nil {
					log.Printf("Kafka validated-order parse failed offset=%d: %v", msg.Offset, err)
					continue
				}
				select {
				case out <- order:
				default:
					log.Printf("Kafka validated-order channel full; skipping offset %d", msg.Offset)
				}
			}
		}
	}
}

func pollControlEvents(ctx context.Context, consumer *kafkalite.Consumer, out chan<- controlEvent) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	nextOffset, err := consumer.LatestOffset("game.broadcast", 0)
	if err != nil {
		log.Printf("Kafka turn-control latest offset lookup failed: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			messages, err := consumer.FetchAll("game.broadcast", 0, nextOffset)
			if err != nil {
				log.Printf("Kafka turn-control fetch failed: %v", err)
				continue
			}
			if len(messages) == 0 {
				continue
			}
			nextOffset = messages[len(messages)-1].Offset + 1
			for _, msg := range messages {
				var event controlEvent
				if err := json.Unmarshal(msg.Value, &event); err != nil || event.Type == "" {
					continue
				}
				if event.Type != "TURN_OWNER" && event.Type != "ADVANCE_REQUEST" {
					continue
				}
				select {
				case out <- event:
				default:
					log.Printf("Kafka turn-control channel full; skipping offset %d", msg.Offset)
				}
			}
		}
	}
}

func drainTurnOrders(pending map[string]validation.Order, currentTurn int) []game.Order {
	keys := make([]string, 0, len(pending))
	for key, order := range pending {
		if order.Turn <= currentTurn {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	orders := make([]game.Order, 0, len(keys))
	for _, key := range keys {
		order := pending[key]
		delete(pending, key)
		if order.Turn == currentTurn {
			orders = append(orders, toGameOrder(order))
		}
	}
	return orders
}

func orderFingerprint(order validation.Order) string {
	data, _ := json.Marshal(struct {
		Turn         int      `json:"turn"`
		UnitID       string   `json:"unitId"`
		OrderType    string   `json:"orderType"`
		PathID       string   `json:"pathId,omitempty"`
		PathIDs      []string `json:"pathIds,omitempty"`
		TargetRegion string   `json:"targetRegion,omitempty"`
		TargetPathID string   `json:"targetPathId,omitempty"`
	}{
		Turn:         order.Turn,
		UnitID:       order.UnitID,
		OrderType:    order.OrderType,
		PathID:       order.PathID,
		PathIDs:      order.PathIDs,
		TargetRegion: order.TargetRegion,
		TargetPathID: order.TargetPathID,
	})
	return string(data)
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
