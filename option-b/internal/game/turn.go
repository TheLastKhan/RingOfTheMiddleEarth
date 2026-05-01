// Package game — turn.go implements the 13-step turn processing engine.
// All logic is config-driven — no hardcoded unit IDs in the game logic.
package game

import (
	"encoding/json"
	"log"
	"time"

	"rotr/internal/config"
)

// ═══════════════════════════════════════════════════════
// TYPES
// ═══════════════════════════════════════════════════════

// TurnState holds mutable game state for one turn of processing.
type TurnState struct {
	Turn        int
	Units       map[string]*UnitRuntime
	Regions     map[string]*RegionRuntime
	Paths       map[string]*PathRuntime
	LightView   *LightView
	DarkView    *DarkViewData
	Config      *config.GameConfig
	Graph       *GameGraph
}

// UnitRuntime is the mutable runtime state of a unit during turn processing.
type UnitRuntime struct {
	ID            string   `json:"id"`
	CurrentRegion string   `json:"currentRegion"`
	Strength      int      `json:"strength"`
	Status        string   `json:"status"` // ACTIVE | DESTROYED | RESPAWNING
	RespawnTimer  int      `json:"respawnTurns"`
	Cooldown      int      `json:"cooldown"`
	Route         []string `json:"route,omitempty"`
	RouteIdx      int      `json:"routeIdx"`
	Config        config.UnitConfig
}

// RegionRuntime is the mutable runtime state of a region.
type RegionRuntime struct {
	ID           string `json:"id"`
	Controller   string `json:"controller"`
	ThreatLevel  int    `json:"threatLevel"`
	Fortified    bool   `json:"fortified"`
	FortifyTimer int    `json:"fortifyTurns"`
}

// PathRuntime is the mutable runtime state of a path.
type PathRuntime struct {
	ID                string `json:"id"`
	Status            string `json:"status"` // OPEN | BLOCKED | THREATENED | TEMPORARILY_OPEN
	SurveillanceLevel int    `json:"surveillanceLevel"`
	TempOpenTurns     int    `json:"tempOpenTurns"`
	BlockedBy         string `json:"blockedBy"`
}

// LightView holds Light Side view of the Ring Bearer.
type LightView struct {
	RingBearerRegion string
}

// DarkViewData holds Dark Side view data.
type DarkViewData struct {
	LastDetectedRegion string
	LastDetectedTurn   int
}

// Order represents a single player order.
type Order struct {
	OrderType    string `json:"orderType"`
	PlayerID     string `json:"playerId"`
	UnitID       string `json:"unitId"`
	PathID       string `json:"pathId,omitempty"`
	PathIDs      []string `json:"pathIds,omitempty"`
	TargetRegion string `json:"targetRegion,omitempty"`
	TargetPathID string `json:"targetPathId,omitempty"`
}

// GameEvent is a produced event from turn processing.
type GameEvent struct {
	Topic     string          `json:"topic"`
	Key       string          `json:"key"`
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
}

// ═══════════════════════════════════════════════════════
// TURN PROCESSOR — 13 steps (Section 6)
// ═══════════════════════════════════════════════════════

// TurnProcessor executes the 13-step turn processing pipeline.
type TurnProcessor struct {
	cfg   *config.GameConfig
	graph *GameGraph
}

// NewTurnProcessor creates a new turn processor.
func NewTurnProcessor(cfg *config.GameConfig, graph *GameGraph) *TurnProcessor {
	return &TurnProcessor{cfg: cfg, graph: graph}
}

// ProcessTurn executes all 13 steps and returns produced events.
func (tp *TurnProcessor) ProcessTurn(state *TurnState, orders []Order) []GameEvent {
	var events []GameEvent

	log.Printf("⚙️  Processing turn %d with %d orders", state.Turn, len(orders))

	// Step 1: Collect and validate orders
	validOrders := tp.step1CollectOrders(state, orders)

	// Step 2: Process route assignments
	events = append(events, tp.step2ProcessRoutes(state, validOrders)...)

	// Step 3: Process path blocking
	events = append(events, tp.step3ProcessBlocking(state, validOrders)...)

	// Step 4: Process reinforcements / redirects
	events = append(events, tp.step4ProcessReinforcements(state, validOrders)...)

	// Step 5: Process fortification
	events = append(events, tp.step5ProcessFortification(state, validOrders)...)

	// Step 6: Process Maia abilities
	events = append(events, tp.step6ProcessMaiaAbilities(state, validOrders)...)

	// Step 7: Auto-advance units along assigned routes
	events = append(events, tp.step7AutoAdvanceUnits(state)...)

	// Step 8: Resolve combat
	events = append(events, tp.step8ResolveCombat(state)...)

	// Step 9: Update path timers
	events = append(events, tp.step9UpdatePathTimers(state)...)

	// Step 10: Update fortification timers
	tp.step10UpdateFortTimers(state)

	// Step 11: Handle respawning units
	events = append(events, tp.step11HandleRespawns(state)...)

	// Step 12: Detection phase
	events = append(events, tp.step12Detection(state)...)

	// Step 13: Check win conditions
	events = append(events, tp.step13CheckWinConditions(state)...)

	// Produce world state snapshot
	events = append(events, tp.produceWorldSnapshot(state))

	state.Turn++

	return events
}

// ═══════════════════════════════════════════════════════
// STEP IMPLEMENTATIONS
// ═══════════════════════════════════════════════════════

func (tp *TurnProcessor) step1CollectOrders(_ *TurnState, orders []Order) []Order {
	// Orders are already validated by the validation topology
	return orders
}

func (tp *TurnProcessor) step2ProcessRoutes(state *TurnState, orders []Order) []GameEvent {
	var events []GameEvent
	for _, order := range orders {
		if order.OrderType != "ASSIGN_ROUTE" {
			continue
		}
		unit, ok := state.Units[order.UnitID]
		if !ok || unit.Status != "ACTIVE" {
			continue
		}
		unit.Route = order.PathIDs
		unit.RouteIdx = 0
	}
	return events
}

func (tp *TurnProcessor) step3ProcessBlocking(state *TurnState, orders []Order) []GameEvent {
	var events []GameEvent
	for _, order := range orders {
		if order.OrderType != "BLOCK_PATH" {
			continue
		}
		unit, ok := state.Units[order.UnitID]
		if !ok || unit.Status != "ACTIVE" {
			continue
		}
		path, ok := state.Paths[order.PathID]
		if !ok {
			continue
		}

		// FellowshipGuard at endpoint → block path for Nazgul
		unitCfg := unit.Config
		if unitCfg.Class == "FellowshipGuard" && tp.graph.IsEndpointOf(unit.CurrentRegion, order.PathID) {
			path.Status = "BLOCKED"
			path.BlockedBy = unit.ID
			events = append(events, makeEvent("game.events.path", order.PathID, map[string]interface{}{
				"pathId":    order.PathID,
				"newStatus": "BLOCKED",
				"turn":      state.Turn,
			}))
		}
	}
	return events
}

func (tp *TurnProcessor) step4ProcessReinforcements(state *TurnState, orders []Order) []GameEvent {
	var events []GameEvent
	for _, order := range orders {
		if order.OrderType != "REDIRECT_UNIT" {
			continue
		}
		unit, ok := state.Units[order.UnitID]
		if !ok || unit.Status != "ACTIVE" {
			continue
		}

		// Move unit to target region if adjacent
		if order.TargetRegion != "" {
			for _, edge := range tp.graph.Neighbors(unit.CurrentRegion) {
				if edge.To == order.TargetRegion {
					oldRegion := unit.CurrentRegion
					unit.CurrentRegion = order.TargetRegion
					events = append(events, makeEvent("game.events.unit", order.UnitID, map[string]interface{}{
						"unitId": order.UnitID,
						"from":   oldRegion,
						"to":     order.TargetRegion,
						"turn":   state.Turn,
					}))
					break
				}
			}
		}
	}
	return events
}

func (tp *TurnProcessor) step5ProcessFortification(state *TurnState, orders []Order) []GameEvent {
	var events []GameEvent
	for _, order := range orders {
		if order.OrderType != "FORTIFY_REGION" {
			continue
		}
		unit, ok := state.Units[order.UnitID]
		if !ok || unit.Status != "ACTIVE" || !unit.Config.CanFortify {
			continue
		}
		region, ok := state.Regions[unit.CurrentRegion]
		if !ok {
			continue
		}
		region.Fortified = true
		region.FortifyTimer = 3 // Fortification lasts 3 turns
		events = append(events, makeEvent("game.events.region", region.ID, map[string]interface{}{
			"regionId":  region.ID,
			"fortified": true,
			"turn":      state.Turn,
		}))
	}
	return events
}

func (tp *TurnProcessor) step6ProcessMaiaAbilities(state *TurnState, orders []Order) []GameEvent {
	var events []GameEvent
	for _, order := range orders {
		if order.OrderType != "MAIA_ABILITY" {
			continue
		}
		unit, ok := state.Units[order.UnitID]
		if !ok || unit.Status != "ACTIVE" || !unit.Config.Maia {
			continue
		}
		if unit.Cooldown > 0 {
			continue // on cooldown
		}

		// Saruman: corrupt path (block permanently)
		if len(unit.Config.MaiaAbilityPaths) > 0 {
			targetPath := order.TargetPathID
			if targetPath == "" && len(order.PathIDs) > 0 {
				targetPath = order.PathIDs[0]
			}
			// Verify target is in allowed paths
			allowed := false
			for _, p := range unit.Config.MaiaAbilityPaths {
				if p == targetPath {
					allowed = true
					break
				}
			}
			if allowed {
				if path, ok := state.Paths[targetPath]; ok {
					path.Status = "BLOCKED"
					path.SurveillanceLevel = 5
					events = append(events, makeEvent("game.events.path", targetPath, map[string]interface{}{
						"pathId":    targetPath,
						"newStatus": "BLOCKED",
						"type":      "CORRUPTED",
						"turn":      state.Turn,
					}))
				}
			}
		}

		// Apply cooldown from config
		unit.Cooldown = unit.Config.Cooldown
	}
	return events
}

func (tp *TurnProcessor) step7AutoAdvanceUnits(state *TurnState) []GameEvent {
	var events []GameEvent

	for _, unit := range state.Units {
		if unit.Status != "ACTIVE" || len(unit.Route) == 0 {
			continue
		}
		if unit.RouteIdx >= len(unit.Route) {
			continue
		}

		// Get next path in route
		nextPathID := unit.Route[unit.RouteIdx]
		path, ok := state.Paths[nextPathID]
		if !ok {
			continue
		}

		// Check if path is blocked
		if path.Status == "BLOCKED" {
			continue // Can't advance through blocked path
		}

		// Get destination
		pathCfg := tp.cfg.PathsByID[nextPathID]
		destination := pathCfg.To
		if unit.CurrentRegion == pathCfg.To {
			destination = pathCfg.From
		}

		oldRegion := unit.CurrentRegion
		unit.CurrentRegion = destination
		unit.RouteIdx++

		events = append(events, makeEvent("game.events.unit", unit.ID, map[string]interface{}{
			"unitId": unit.ID,
			"from":   oldRegion,
			"to":     destination,
			"turn":   state.Turn,
		}))

		// If this is the Ring Bearer, update ring position
		if unit.Config.Class == "RingBearer" {
			state.LightView.RingBearerRegion = destination
			events = append(events, makeEvent("game.ring.position", "", map[string]interface{}{
				"trueRegion": destination,
				"turn":       state.Turn,
			}))
		}
	}

	return events
}

func (tp *TurnProcessor) step8ResolveCombat(state *TurnState) []GameEvent {
	var events []GameEvent

	// For each region, check if opposing units co-exist
	regionUnits := make(map[string][]*UnitRuntime)
	for _, unit := range state.Units {
		if unit.Status == "ACTIVE" {
			regionUnits[unit.CurrentRegion] = append(regionUnits[unit.CurrentRegion], unit)
		}
	}

	for regionID, units := range regionUnits {
		// Separate sides
		var lightUnits, darkUnits []CombatUnit
		for _, u := range units {
			cu := CombatUnit{ID: u.ID, Strength: u.Strength, Config: u.Config}
			if u.Config.Side == "FREE_PEOPLES" {
				lightUnits = append(lightUnits, cu)
			} else if u.Config.Side == "SHADOW" {
				darkUnits = append(darkUnits, cu)
			}
		}

		if len(lightUnits) == 0 || len(darkUnits) == 0 {
			continue // No opposing forces
		}

		region := state.Regions[regionID]
		regionCfg := tp.cfg.RegionsByID[regionID]

		// Determine attacker/defender based on control
		var attackers, defenders []CombatUnit
		if region.Controller == "SHADOW" || region.Controller == "NEUTRAL" {
			attackers = lightUnits
			defenders = darkUnits
		} else {
			attackers = darkUnits
			defenders = lightUnits
		}

		result := ResolveCombat(attackers, defenders, regionCfg.Terrain, region.Fortified)

		// Apply results to unit runtime
		for _, updated := range result.UpdatedAttackers {
			if u, ok := state.Units[updated.ID]; ok {
				u.Strength = updated.Strength
				if u.Strength <= 0 && !u.Config.Indestructible {
					if u.Config.Respawns {
						u.Status = "RESPAWNING"
						u.RespawnTimer = u.Config.RespawnTurns
					} else {
						u.Status = "DESTROYED"
					}
				}
			}
		}
		for _, updated := range result.UpdatedDefenders {
			if u, ok := state.Units[updated.ID]; ok {
				u.Strength = updated.Strength
				if u.Strength <= 0 && !u.Config.Indestructible {
					if u.Config.Respawns {
						u.Status = "RESPAWNING"
						u.RespawnTimer = u.Config.RespawnTurns
					} else {
						u.Status = "DESTROYED"
					}
				}
			}
		}

		// Update region control
		if result.AttackerWon {
			attackerSide := attackers[0].Config.Side
			region.Controller = attackerSide
			region.Fortified = false // Fortification destroyed on capture
			events = append(events, makeEvent("game.events.region", regionID, map[string]interface{}{
				"regionId":      regionID,
				"newController": attackerSide,
				"attackerWon":   true,
				"turn":          state.Turn,
			}))
		}

		events = append(events, makeEvent("game.events.region", regionID, map[string]interface{}{
			"regionId":    regionID,
			"attackerWon": result.AttackerWon,
			"turn":        state.Turn,
		}))
	}

	return events
}

func (tp *TurnProcessor) step9UpdatePathTimers(state *TurnState) []GameEvent {
	var events []GameEvent
	for _, path := range state.Paths {
		if path.Status == "TEMPORARILY_OPEN" {
			path.TempOpenTurns--
			if path.TempOpenTurns <= 0 {
				path.Status = "BLOCKED"
				events = append(events, makeEvent("game.events.path", path.ID, map[string]interface{}{
					"pathId":    path.ID,
					"newStatus": "BLOCKED",
					"turn":      state.Turn,
				}))
			}
		}
	}
	return events
}

func (tp *TurnProcessor) step10UpdateFortTimers(state *TurnState) {
	for _, region := range state.Regions {
		if region.Fortified {
			region.FortifyTimer--
			if region.FortifyTimer <= 0 {
				region.Fortified = false
			}
		}
	}
}

func (tp *TurnProcessor) step11HandleRespawns(state *TurnState) []GameEvent {
	var events []GameEvent
	for _, unit := range state.Units {
		if unit.Status == "RESPAWNING" {
			unit.RespawnTimer--
			if unit.RespawnTimer <= 0 {
				unit.Status = "ACTIVE"
				unit.Strength = unit.Config.Strength
				unit.CurrentRegion = unit.Config.StartRegion
				events = append(events, makeEvent("game.events.unit", unit.ID, map[string]interface{}{
					"unitId": unit.ID,
					"event":  "RESPAWNED",
					"region": unit.Config.StartRegion,
					"turn":   state.Turn,
				}))
			}
		}
		// Decrease cooldown
		if unit.Cooldown > 0 {
			unit.Cooldown--
		}
	}
	return events
}

func (tp *TurnProcessor) step12Detection(state *TurnState) []GameEvent {
	var events []GameEvent

	// Find Ring Bearer position
	rbRegion := ""
	for _, unit := range state.Units {
		if unit.Config.Class == "RingBearer" && unit.Status == "ACTIVE" {
			rbRegion = unit.CurrentRegion
			break
		}
	}
	if rbRegion == "" {
		return events
	}

	// Build detection input from config
	unitStates := make(map[string]UnitState)
	for _, u := range state.Units {
		unitStates[u.ID] = UnitState{
			CurrentRegion: u.CurrentRegion,
			Status:        u.Status,
		}
	}

	input := BuildDetectionInput(rbRegion, state.Turn, tp.cfg, unitStates)
	result := CheckDetection(tp.graph, input)

	if result.Detected {
		state.DarkView.LastDetectedRegion = result.Region
		state.DarkView.LastDetectedTurn = state.Turn
		events = append(events, makeEvent("game.ring.detection", "", map[string]interface{}{
			"regionId": result.Region,
			"byUnit":   result.ByUnit,
			"turn":     state.Turn,
		}))
	}

	return events
}

func (tp *TurnProcessor) step13CheckWinConditions(state *TurnState) []GameEvent {
	var events []GameEvent

	// Win 1: Ring Bearer reaches Mount Doom
	for _, unit := range state.Units {
		if unit.Config.Class == "RingBearer" && unit.Status == "ACTIVE" {
			regionCfg := tp.cfg.RegionsByID[unit.CurrentRegion]
			if regionCfg.SpecialRole == "RING_DESTRUCTION_SITE" {
				events = append(events, makeEvent("game.broadcast", "", map[string]interface{}{
					"type":    "GAME_OVER",
					"winner":  "FREE_PEOPLES",
					"cause":   "Ring destroyed at Mount Doom",
					"turn":    state.Turn,
				}))
				return events
			}
		}
	}

	// Win 2: Ring Bearer destroyed
	for _, unit := range state.Units {
		if unit.Config.Class == "RingBearer" && unit.Status == "DESTROYED" {
			events = append(events, makeEvent("game.broadcast", "", map[string]interface{}{
				"type":   "GAME_OVER",
				"winner": "SHADOW",
				"cause":  "Ring Bearer destroyed",
				"turn":   state.Turn,
			}))
			return events
		}
	}

	// Win 3: Max turns exceeded → Shadow wins
	if state.Turn >= tp.cfg.MaxTurns {
		events = append(events, makeEvent("game.broadcast", "", map[string]interface{}{
			"type":   "GAME_OVER",
			"winner": "SHADOW",
			"cause":  "Maximum turns exceeded — Ring Bearer failed to reach Mount Doom",
			"turn":   state.Turn,
		}))
		return events
	}

	return events
}

func (tp *TurnProcessor) produceWorldSnapshot(state *TurnState) GameEvent {
	snapshot := map[string]interface{}{
		"turn":      state.Turn,
		"type":      "WORLD_STATE",
		"units":     state.Units,
		"regions":   state.Regions,
		"timestamp": time.Now().UnixMilli(),
	}
	return makeEvent("game.broadcast", "", snapshot)
}

// ═══════════════════════════════════════════════════════
// HELPER
// ═══════════════════════════════════════════════════════

func makeEvent(topic, key string, data interface{}) GameEvent {
	jsonData, _ := json.Marshal(data)
	return GameEvent{
		Topic:     topic,
		Key:       key,
		Data:      jsonData,
		Timestamp: time.Now().UnixMilli(),
	}
}

// InitTurnState creates the initial turn state from configuration.
func InitTurnState(cfg *config.GameConfig, graph *GameGraph) *TurnState {
	state := &TurnState{
		Turn:    1,
		Units:   make(map[string]*UnitRuntime),
		Regions: make(map[string]*RegionRuntime),
		Paths:   make(map[string]*PathRuntime),
		LightView: &LightView{},
		DarkView:  &DarkViewData{},
		Config:  cfg,
		Graph:   graph,
	}

	for _, u := range cfg.Units {
		state.Units[u.ID] = &UnitRuntime{
			ID:            u.ID,
			CurrentRegion: u.StartRegion,
			Strength:      u.Strength,
			Status:        "ACTIVE",
			Config:        u,
		}
		if u.Class == "RingBearer" {
			state.LightView.RingBearerRegion = u.StartRegion
		}
	}

	for _, r := range cfg.Regions {
		state.Regions[r.ID] = &RegionRuntime{
			ID:          r.ID,
			Controller:  r.StartControl,
			ThreatLevel: r.StartThreat,
		}
	}

	for _, p := range cfg.Paths {
		state.Paths[p.ID] = &PathRuntime{
			ID:     p.ID,
			Status: "OPEN",
		}
	}

	return state
}
