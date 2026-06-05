// Package api implements the HTTP REST API and SSE server.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"rotr/internal/cache"
	"rotr/internal/config"
	"rotr/internal/game"
	"rotr/internal/pipeline"
	"rotr/internal/router"
	"rotr/internal/validation"
)

// ═══════════════════════════════════════════════════════
// SERVER
// ═══════════════════════════════════════════════════════

// Server handles all HTTP endpoints and SSE connections.
type Server struct {
	cfg       *config.GameConfig
	cache     *cache.WorldStateCache
	router    *router.EventRouter
	graph     *game.GameGraph
	validator *validation.Validator
	port      string

	// SSE connections
	lightSSEClients map[chan router.Event]struct{}
	darkSSEClients  map[chan router.Event]struct{}
	sseClientsMu    sync.RWMutex

	// Order channel — for sending to Kafka
	OrderCh   chan validation.Order
	StartCh   chan struct{}
	ResetCh   chan chan struct{}
	AdvanceCh chan chan struct{}
}

// NewServer creates a new API server.
func NewServer(cfg *config.GameConfig, c *cache.WorldStateCache, r *router.EventRouter, g *game.GameGraph, port string) *Server {
	s := &Server{
		cfg:             cfg,
		cache:           c,
		router:          r,
		graph:           g,
		validator:       validation.NewValidator(cfg, c),
		port:            port,
		lightSSEClients: make(map[chan router.Event]struct{}),
		darkSSEClients:  make(map[chan router.Event]struct{}),
		OrderCh:         make(chan validation.Order, 100),
		StartCh:         make(chan struct{}, 1),
		ResetCh:         make(chan chan struct{}, 1),
		AdvanceCh:       make(chan chan struct{}, 1),
	}

	go s.fanOutSSE(false, r.LightSSECh)
	go s.fanOutSSE(true, r.DarkSSECh)
	return s
}

func (s *Server) signalGameStarted() {
	select {
	case s.StartCh <- struct{}{}:
	default:
	}
}

func (s *Server) signalGameReset() {
	// Wait for the engine loop to rebuild state before this HTTP request returns.
	// That keeps the UI from immediately reading stale turn data after reset.
	ack := make(chan struct{})
	select {
	case s.ResetCh <- ack:
		select {
		case <-ack:
		case <-time.After(2 * time.Second):
			log.Printf("game reset timed out")
		}
	default:
	}
}

func (s *Server) signalAdvanceTurn() {
	// Manual turn advance can run the full turn processor and publish events,
	// so it gets a longer timeout than reset.
	ack := make(chan struct{})
	select {
	case s.AdvanceCh <- ack:
		select {
		case <-ack:
		case <-time.After(5 * time.Second):
			log.Printf("advance turn timed out")
		}
	default:
	}
}

// ResetTurn clears per-turn validation state.
func (s *Server) ResetTurn() {
	s.validator.ResetTurn()
}

func (s *Server) fanOutSSE(isDarkSide bool, source <-chan router.Event) {
	for event := range source {
		s.sseClientsMu.RLock()
		if isDarkSide {
			for ch := range s.darkSSEClients {
				select {
				case ch <- event:
				default:
				}
			}
		} else {
			for ch := range s.lightSSEClients {
				select {
				case ch <- event:
				default:
				}
			}
		}
		s.sseClientsMu.RUnlock()
	}
}

func (s *Server) registerSSEClient(isDarkSide bool, ch chan router.Event) func() {
	s.sseClientsMu.Lock()
	if isDarkSide {
		s.darkSSEClients[ch] = struct{}{}
	} else {
		s.lightSSEClients[ch] = struct{}{}
	}
	s.sseClientsMu.Unlock()

	return func() {
		s.sseClientsMu.Lock()
		if isDarkSide {
			delete(s.darkSSEClients, ch)
		} else {
			delete(s.lightSSEClients, ch)
		}
		s.sseClientsMu.Unlock()
		close(ch)
	}
}

// Start begins the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// ── Endpoints (Section 34) ──
	mux.HandleFunc("/game/start", s.corsMiddleware(s.handleGameStart))
	mux.HandleFunc("/game/advance-turn", s.corsMiddleware(s.handleAdvanceTurn))
	mux.HandleFunc("/game/state", s.corsMiddleware(s.handleGameState))
	mux.HandleFunc("/order", s.corsMiddleware(s.handleOrder))
	mux.HandleFunc("/orders/available", s.corsMiddleware(s.handleAvailableOrders))
	mux.HandleFunc("/events", s.corsMiddleware(s.handleSSE))
	mux.HandleFunc("/analysis/routes", s.corsMiddleware(s.handleAnalysisRoutes))
	mux.HandleFunc("/analysis/intercept", s.corsMiddleware(s.handleAnalysisIntercept))
	mux.HandleFunc("/health", s.corsMiddleware(s.handleHealth))
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	log.Printf("🚀 Game server starting on port %s", s.port)
	return http.ListenAndServe(":"+s.port, mux)
}

// corsMiddleware adds CORS headers for browser access.
func (s *Server) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

// ═══════════════════════════════════════════════════════
// HANDLERS
// ═══════════════════════════════════════════════════════

// POST /game/start — Start a new game
func (s *Server) handleGameStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Mode          string `json:"mode"`
		LightPlayerID string `json:"lightPlayerId"`
		DarkPlayerID  string `json:"darkPlayerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("🎮 Game started: mode=%s, light=%s, dark=%s", req.Mode, req.LightPlayerID, req.DarkPlayerID)

	s.signalGameReset()
	s.router.DrainSSE()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "started",
		"mode":    req.Mode,
		"message": "Game initialized. Connect via SSE to receive events.",
	})
}

// POST /game/advance-turn — manually resolve the current turn.
func (s *Server) handleAdvanceTurn(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.signalAdvanceTurn()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "accepted",
		"message": "Turn advanced manually.",
	})
}

// GET /game/state — World state (Ring Bearer stripped for Dark Side)
func (s *Server) handleGameState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Backend-side information hiding: Dark receives sanitized state even if
	// the browser directly calls /game/state.
	isDarkSide := s.isRequestDarkSide(r)

	if isDarkSide {
		s.writeState(w, s.cache.GetDarkState())
	} else {
		s.writeState(w, s.cache.GetLightState())
	}
}

func (s *Server) writeState(w http.ResponseWriter, data []byte) {
	// The cache already returns side-appropriate JSON. This helper adds runtime
	// metadata used by the UI, such as timer length and max turns.
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		w.Write(data)
		return
	}
	payload["turnDurationSeconds"] = s.cfg.TurnDurationSeconds
	payload["maxTurns"] = s.cfg.MaxTurns
	json.NewEncoder(w).Encode(payload)
}

// POST /order — Submit order (→ 202 Accepted)
func (s *Server) handleOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var order validation.Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, "Invalid order format", http.StatusBadRequest)
		return
	}
	normalizeOrderPayload(&order)
	s.signalGameStarted()

	// Validate before the order enters the engine loop. Invalid orders fail fast
	// with a useful HTTP error instead of silently changing turn processing.
	result := s.validator.Validate(order)
	if !result.Valid {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   result.ErrorCode,
			"message": result.ErrorMsg,
		})
		return
	}

	// Send to the engine-owned order channel. If the buffer is full, avoid
	// blocking the HTTP goroutine forever and log the dropped order.
	select {
	case s.OrderCh <- order:
	default:
		log.Printf("⚠️ Order channel full, dropping order from %s", order.PlayerID)
	}

	// 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "accepted",
		"message": fmt.Sprintf("Order %s for unit %s accepted", order.OrderType, order.UnitID),
	})
}

// GET /orders/available — Available orders for a unit
func normalizeOrderPayload(order *validation.Order) {
	// Older UI payloads nested fields under "payload"; newer payloads put the
	// same fields at top level. Normalization keeps both request shapes working.
	if order.Payload == nil {
		return
	}
	if order.PathID == "" {
		if value, ok := order.Payload["pathId"].(string); ok {
			order.PathID = value
		}
	}
	if order.TargetRegion == "" {
		if value, ok := order.Payload["targetRegion"].(string); ok {
			order.TargetRegion = value
		}
	}
	if order.TargetPathID == "" {
		if value, ok := order.Payload["targetPathId"].(string); ok {
			order.TargetPathID = value
		}
	}
	if len(order.PathIDs) == 0 {
		if values, ok := order.Payload["pathIds"].([]interface{}); ok {
			for _, value := range values {
				if pathID, ok := value.(string); ok && pathID != "" {
					order.PathIDs = append(order.PathIDs, pathID)
				}
			}
		}
	}
}

func (s *Server) handleAvailableOrders(w http.ResponseWriter, r *http.Request) {
	unitID := r.URL.Query().Get("unitId")

	unitCfg, ok := s.cfg.UnitsByID[unitID]
	if !ok {
		http.Error(w, "Unit not found", http.StatusNotFound)
		return
	}

	available := []string{}

	if unitID == "sauron" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"unitId":          unitID,
			"availableOrders": available,
		})
		return
	}

	// All units can assign routes and redirect
	available = append(available, "ASSIGN_ROUTE", "REDIRECT_UNIT")

	// Attack is available for combat units
	if unitCfg.Strength > 0 {
		available = append(available, "ATTACK_REGION")
	}

	// Block and search paths — config-driven
	if unitCfg.DetectionRange > 0 || unitCfg.Side == "SHADOW" {
		available = append(available, "BLOCK_PATH")
		if unitCfg.Side == "SHADOW" {
			available = append(available, "SEARCH_PATH")
		}
	} else {
		available = append(available, "BLOCK_PATH")
	}

	// Maia ability — config-driven
	if unitCfg.Maia {
		available = append(available, "MAIA_ABILITY")
	}

	// Fortify — config-driven
	if unitCfg.CanFortify {
		available = append(available, "FORTIFY_REGION")
	}

	// Destroy Ring — only Ring Bearer class
	if unitCfg.Class == "RingBearer" {
		available = append(available, "DESTROY_RING")
	}

	playerSide := s.requestPlayerSide(r)
	if playerSide != "" && unitCfg.Side != playerSide {
		available = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"unitId":          unitID,
		"availableOrders": available,
	})
}

// GET /events — SSE stream
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	playerID := r.URL.Query().Get("playerId")

	// SSE keeps the HTTP response open and streams one-way server events to the
	// browser. Player commands still use normal POST requests.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// This is the live-event side of information hiding: Light and Dark read
	// from different router channels.
	isDarkSide := s.isRequestDarkSide(r)

	// Create channel for this client
	clientCh := make(chan router.Event, 50)
	unregister := s.registerSSEClient(isDarkSide, clientCh)
	defer unregister()

	log.Printf("📡 SSE connected: %s", playerID)

	ctx := r.Context()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// SSE forward loop
	for {
		select {
		case <-ctx.Done():
			log.Printf("📡 SSE disconnected: %s", playerID)
			return

		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()

		case event := <-clientCh:
			data, _ := json.Marshal(map[string]interface{}{
				"topic": event.Topic,
				"data":  json.RawMessage(event.Data),
			})
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Topic, string(data))
			flusher.Flush()
		}
	}
}

// GET /analysis/routes — Route risk analysis (Light Side only)
func (s *Server) handleAnalysisRoutes(w http.ResponseWriter, r *http.Request) {
	// Canonical routes are stored as region IDs in config, so this handler first
	// resolves the path IDs connecting each consecutive region pair.
	var routes []pipeline.RouteRiskInput
	for _, cr := range s.cfg.CanonicalRoutes {
		pathIDs := []string{}
		for i := 0; i < len(cr.Path)-1; i++ {
			// Find path between consecutive regions
			for _, p := range s.cfg.Paths {
				if (p.From == cr.Path[i] && p.To == cr.Path[i+1]) ||
					(p.To == cr.Path[i] && p.From == cr.Path[i+1]) {
					pathIDs = append(pathIDs, p.ID)
					break
				}
			}
		}
		routes = append(routes, pipeline.RouteRiskInput{
			RouteID:   cr.ID,
			PathIDs:   pathIDs,
			RegionIDs: cr.Path,
		})
	}

	// Build pipeline input from the serving cache. The pipeline package receives
	// plain snapshots so it remains testable without the HTTP server.
	snap := s.cache.GetSnapshot()
	state := pipeline.RouteRiskState{
		Regions: make(map[string]pipeline.RegionSnapshot),
		Paths:   make(map[string]pipeline.PathSnapshot),
		Graph:   s.graph,
	}
	for _, r := range snap.Regions {
		state.Regions[r.ID] = pipeline.RegionSnapshot{
			ID: r.ID, Controller: r.Controller, ThreatLevel: r.ThreatLevel,
		}
	}
	for _, p := range snap.Paths {
		state.Paths[p.ID] = pipeline.PathSnapshot{
			ID: p.ID, Status: p.Status, SurveillanceLevel: p.SurveillanceLevel,
		}
	}
	for _, u := range snap.Units {
		cfg, ok := s.cfg.UnitsByID[u.ID]
		if ok && cfg.DetectionRange > 0 {
			state.NazgulUnits = append(state.NazgulUnits, pipeline.UnitSnapshot{
				ID: u.ID, CurrentRegion: u.CurrentRegion, Status: u.Status,
				Side: cfg.Side, Config: cfg,
			})
		}
	}

	result := pipeline.ComputeRouteRisk(context.Background(), routes, state)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GET /analysis/intercept — Interception plan (Dark Side only)
func (s *Server) handleAnalysisIntercept(w http.ResponseWriter, r *http.Request) {
	snap := s.cache.GetSnapshot()

	// Find active Nazgul-like detectors by config, then score each one against
	// every canonical route candidate.
	var inputs []pipeline.InterceptInput
	for _, u := range snap.Units {
		cfg, ok := s.cfg.UnitsByID[u.ID]
		if !ok || cfg.DetectionRange <= 0 || u.Status != "ACTIVE" {
			continue
		}

		// Score against each canonical route
		for _, cr := range s.cfg.CanonicalRoutes {
			costs := []int{}
			for i := 0; i < len(cr.Path)-1; i++ {
				for _, p := range s.cfg.Paths {
					if (p.From == cr.Path[i] && p.To == cr.Path[i+1]) ||
						(p.To == cr.Path[i] && p.From == cr.Path[i+1]) {
						costs = append(costs, p.Cost)
						break
					}
				}
			}
			inputs = append(inputs, pipeline.InterceptInput{
				NazgulID:     u.ID,
				NazgulRegion: u.CurrentRegion,
				RouteRegions: cr.Path,
				RouteCosts:   costs,
			})
		}
	}

	result := pipeline.ComputeInterception(context.Background(), inputs, s.graph)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GET /health — Health check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":              "ok",
		"turn":                s.cache.GetSnapshot().Turn,
		"maxTurns":            s.cfg.MaxTurns,
		"turnDurationSeconds": s.cfg.TurnDurationSeconds,
		"timestamp":           time.Now().Unix(),
	})
}

// ═══════════════════════════════════════════════════════
// HELPERS
// ═══════════════════════════════════════════════════════

func (s *Server) requestPlayerSide(r *http.Request) string {
	// Prefer explicit side query parameters; they are less ambiguous than
	// player IDs and make the browser URLs easy to demo.
	side := r.URL.Query().Get("side")
	if side == "FREE_PEOPLES" || side == "SHADOW" {
		return side
	}
	if side == "free_peoples" {
		return "FREE_PEOPLES"
	}
	if side == "shadow" {
		return "SHADOW"
	}
	return ""
}

func (s *Server) isRequestDarkSide(r *http.Request) bool {
	switch s.requestPlayerSide(r) {
	case "SHADOW":
		return true
	case "FREE_PEOPLES":
		return false
	default:
		return s.isPlayerDarkSide(r.URL.Query().Get("playerId"))
	}
}

func (s *Server) isPlayerDarkSide(playerID string) bool {
	return playerID == "dark-player" || playerID == "dark-opponent" ||
		hasPrefix(playerID, "dark-")
}

func hasPrefix(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}
