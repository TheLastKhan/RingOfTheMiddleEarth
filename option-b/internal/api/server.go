// Package api implements the HTTP REST API and SSE server.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	sseClients   map[string]chan router.Event // playerID → channel
	sseClientsMu sync.RWMutex

	// Order channel — for sending to Kafka
	OrderCh chan validation.Order
}

// NewServer creates a new API server.
func NewServer(cfg *config.GameConfig, c *cache.WorldStateCache, r *router.EventRouter, g *game.GameGraph, port string) *Server {
	return &Server{
		cfg:        cfg,
		cache:      c,
		router:     r,
		graph:      g,
		validator:  validation.NewValidator(cfg, c),
		port:       port,
		sseClients: make(map[string]chan router.Event),
		OrderCh:    make(chan validation.Order, 100),
	}
}

// Start begins the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// ── Endpoints (Section 34) ──
	mux.HandleFunc("/game/start", s.corsMiddleware(s.handleGameStart))
	mux.HandleFunc("/game/state", s.corsMiddleware(s.handleGameState))
	mux.HandleFunc("/order", s.corsMiddleware(s.handleOrder))
	mux.HandleFunc("/orders/available", s.corsMiddleware(s.handleAvailableOrders))
	mux.HandleFunc("/events", s.corsMiddleware(s.handleSSE))
	mux.HandleFunc("/analysis/routes", s.corsMiddleware(s.handleAnalysisRoutes))
	mux.HandleFunc("/analysis/intercept", s.corsMiddleware(s.handleAnalysisIntercept))
	mux.HandleFunc("/health", s.corsMiddleware(s.handleHealth))

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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "started",
		"mode":    req.Mode,
		"message": "Game initialized. Connect via SSE to receive events.",
	})
}

// GET /game/state — World state (Ring Bearer stripped for Dark Side)
func (s *Server) handleGameState(w http.ResponseWriter, r *http.Request) {
	playerID := r.URL.Query().Get("playerId")

	w.Header().Set("Content-Type", "application/json")

	// Determine player side
	isDarkSide := s.isPlayerDarkSide(playerID)

	if isDarkSide {
		w.Write(s.cache.GetDarkState())
	} else {
		w.Write(s.cache.GetLightState())
	}
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

	// Validate
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

	// Send to order channel (for Kafka production)
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
func (s *Server) handleAvailableOrders(w http.ResponseWriter, r *http.Request) {
	unitID := r.URL.Query().Get("unitId")
	playerID := r.URL.Query().Get("playerId")

	unitCfg, ok := s.cfg.UnitsByID[unitID]
	if !ok {
		http.Error(w, "Unit not found", http.StatusNotFound)
		return
	}

	available := []string{}

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

	_ = playerID // Used for side filtering if needed

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"unitId":          unitID,
		"availableOrders": available,
	})
}

// GET /events — SSE stream
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	playerID := r.URL.Query().Get("playerId")

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Create channel for this client
	clientCh := make(chan router.Event, 50)
	s.sseClientsMu.Lock()
	s.sseClients[playerID] = clientCh
	s.sseClientsMu.Unlock()

	defer func() {
		s.sseClientsMu.Lock()
		delete(s.sseClients, playerID)
		s.sseClientsMu.Unlock()
		close(clientCh)
	}()

	log.Printf("📡 SSE connected: %s", playerID)

	// Determine which router channel to listen to
	isDarkSide := s.isPlayerDarkSide(playerID)

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

		case event := <-func() chan router.Event {
			if isDarkSide {
				return s.router.DarkSSECh
			}
			return s.router.LightSSECh
		}():
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
	// Build route inputs from canonical routes
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

	// Build state snapshot
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

	// Find active Nazgul
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
		"status":    "ok",
		"turn":      s.cache.GetSnapshot().Turn,
		"timestamp": time.Now().Unix(),
	})
}

// ═══════════════════════════════════════════════════════
// HELPERS
// ═══════════════════════════════════════════════════════

func (s *Server) isPlayerDarkSide(playerID string) bool {
	// Convention: player IDs containing "dark" are Dark Side
	return playerID == "dark-player" || playerID == "dark-opponent" ||
		len(playerID) > 0 && playerID[0] == 'd'
}
