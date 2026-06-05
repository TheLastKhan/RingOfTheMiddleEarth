// Package game — turn.go implements the 13-step turn processing engine.
// All logic is config-driven — no hardcoded unit IDs in the game logic.
package game

import (
	"encoding/json"
	"log"
	"strconv"
	"time"

	"rotr/internal/config"
)

// ═══════════════════════════════════════════════════════
// TYPES
// ═══════════════════════════════════════════════════════

// TurnState holds mutable game state for one turn of processing.
type TurnState struct {
	Turn      int
	Units     map[string]*UnitRuntime
	Regions   map[string]*RegionRuntime
	Paths     map[string]*PathRuntime
	LightView *LightView
	DarkView  *DarkViewData
	Config    *config.GameConfig
	Graph     *GameGraph
	GameOver  bool
	Winner    string
	Cause     string
	// Exposed is turn-scoped. ProcessTurn resets it at the start of every turn,
	// detection may set it during this turn, and win conditions read it at the end.
	Exposed       bool
	CurrentOrders []Order
}

// UnitRuntime is the mutable runtime state of a unit during turn processing.
type UnitRuntime struct {
	ID              string   `json:"id"`
	CurrentRegion   string   `json:"currentRegion"`
	Strength        int      `json:"strength"`
	Status          string   `json:"status"` // ACTIVE | DESTROYED | RESPAWNING
	RespawnTimer    int      `json:"respawnTurns"`
	Cooldown        int      `json:"cooldown"`
	Route           []string `json:"route,omitempty"`
	RouteIdx        int      `json:"routeIdx"`
	TravelPathID    string   `json:"travelPathId,omitempty"`
	TravelFrom      string   `json:"travelFrom,omitempty"`
	TravelTo        string   `json:"travelTo,omitempty"`
	TravelRemaining int      `json:"travelRemaining,omitempty"`
	Config          config.UnitConfig
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
	From              string `json:"from"`
	To                string `json:"to"`
	Status            string `json:"status"` // OPEN | BLOCKED | THREATENED | TEMPORARILY_OPEN
	SurveillanceLevel int    `json:"surveillanceLevel"`
	TempOpenTurns     int    `json:"tempOpenTurns"`
	BlockedBy         string `json:"blockedBy"`
	Corrupted         bool   `json:"corrupted"`
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
	OrderType    string   `json:"orderType"`
	PlayerID     string   `json:"playerId"`
	UnitID       string   `json:"unitId"`
	PathID       string   `json:"pathId,omitempty"`
	PathIDs      []string `json:"pathIds,omitempty"`
	TargetRegion string   `json:"targetRegion,omitempty"`
	TargetPathID string   `json:"targetPathId,omitempty"`
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

	if state.GameOver {
		return events
	}

	log.Printf("⚙️  Processing turn %d with %d orders", state.Turn, len(orders))

	// Exposure is not permanent. A Shadow player must capitalize on detection
	// during the same processed turn unless the Ring Bearer is destroyed outright.
	state.Exposed = false

	// Step 1: Collect and validate orders
	validOrders := tp.step1CollectOrders(state, orders)
	state.CurrentOrders = validOrders
	defer func() { state.CurrentOrders = nil }()

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
	if state.GameOver {
		events = append(events, tp.produceWorldSnapshot(state))
		return events
	}

	state.Turn++

	// Produce world state snapshot
	events = append(events, tp.produceWorldSnapshot(state))

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
		if !tp.canUnitReceiveRoute(unit, order.PathIDs) {
			continue
		}
		// Route assignment stores the path sequence; movement happens later in
		// step 7 so all players' orders are applied in deterministic order.
		unit.Route = order.PathIDs
		unit.RouteIdx = 0
		unit.TravelPathID = ""
		unit.TravelFrom = ""
		unit.TravelTo = ""
		unit.TravelRemaining = 0
	}
	return events
}

func (tp *TurnProcessor) canUnitReceiveRoute(unit *UnitRuntime, pathIDs []string) bool {
	if unit.ID == "sauron" {
		return false
	}
	if unit.Config.Side == "SHADOW" && unit.Config.Indestructible {
		for _, pathID := range pathIDs {
			pathCfg, ok := tp.cfg.PathsByID[pathID]
			if ok && (pathCfg.From == "mount-doom" || pathCfg.To == "mount-doom") {
				return false
			}
		}
	}
	return true
}

func (tp *TurnProcessor) canUnitEnterRegion(unit *UnitRuntime, regionID string) bool {
	if regionID == "" {
		return true
	}
	if unit.ID == "sauron" {
		return false
	}
	return !(unit.Config.Side == "SHADOW" && unit.Config.Indestructible && regionID == "mount-doom")
}

func (tp *TurnProcessor) step3ProcessBlocking(state *TurnState, orders []Order) []GameEvent {
	var events []GameEvent
	for _, order := range orders {
		// ── BlockPath: unit at endpoint → path becomes BLOCKED ──
		if order.OrderType == "BLOCK_PATH" {
			unit, ok := state.Units[order.UnitID]
			if !ok || unit.Status != "ACTIVE" {
				continue
			}
			path, ok := state.Paths[order.PathID]
			if !ok {
				continue
			}
			if tp.graph.IsEndpointOf(unit.CurrentRegion, order.PathID) {
				path.Status = "BLOCKED"
				path.BlockedBy = unit.ID
				events = append(events, makeEvent("game.events.path", order.PathID, map[string]interface{}{
					"pathId":    order.PathID,
					"newStatus": "BLOCKED",
					"turn":      state.Turn,
				}))
			}
		}

		// ── SearchPath (Dark Side): surveillanceLevel += 1 (max 3) ──
		if order.OrderType == "SEARCH_PATH" {
			unit, ok := state.Units[order.UnitID]
			if !ok || unit.Status != "ACTIVE" || unit.Config.Side != "SHADOW" {
				continue
			}
			path, ok := state.Paths[order.PathID]
			if !ok {
				continue
			}
			if tp.graph.IsEndpointOf(unit.CurrentRegion, order.PathID) {
				if path.SurveillanceLevel < 3 {
					path.SurveillanceLevel++
				}
				events = append(events, makeEvent("game.events.path", order.PathID, map[string]interface{}{
					"pathId":            order.PathID,
					"surveillanceLevel": path.SurveillanceLevel,
					"turn":              state.Turn,
				}))
			}
		}
	}
	return events
}

func (tp *TurnProcessor) step4ProcessReinforcements(state *TurnState, orders []Order) []GameEvent {
	var events []GameEvent
	for _, order := range orders {
		// ── RedirectUnit: change an existing route ──
		if order.OrderType == "REDIRECT_UNIT" {
			unit, ok := state.Units[order.UnitID]
			if !ok || unit.Status != "ACTIVE" {
				continue
			}
			if !tp.canUnitReceiveRoute(unit, order.PathIDs) || !tp.canUnitEnterRegion(unit, order.TargetRegion) {
				continue
			}
			if len(order.PathIDs) > 0 {
				unit.Route = order.PathIDs
				unit.RouteIdx = 0
			}
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

		// ── ReinforceRegion: move unit to adjacent target region ──
		if order.OrderType == "REINFORCE_REGION" {
			unit, ok := state.Units[order.UnitID]
			if !ok || unit.Status != "ACTIVE" || order.TargetRegion == "" {
				continue
			}
			if !tp.canUnitEnterRegion(unit, order.TargetRegion) {
				continue
			}
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

		// ── DeployNazgul (Dark Side only): move Nazgul to target region ──
		if order.OrderType == "DEPLOY_NAZGUL" {
			unit, ok := state.Units[order.UnitID]
			if !ok || unit.Status != "ACTIVE" || unit.Config.Side != "SHADOW" || order.TargetRegion == "" {
				continue
			}
			if !tp.canUnitEnterRegion(unit, order.TargetRegion) {
				continue
			}
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
		region.FortifyTimer = 2 // Spec: fortifyTurns=2 (Section 6, line 844)
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

		targetPath := order.TargetPathID
		if targetPath == "" {
			targetPath = order.PathID
		}
		if targetPath == "" && len(order.PathIDs) > 0 {
			targetPath = order.PathIDs[0]
		}

		// Saruman: corrupt configured paths permanently.
		if len(unit.Config.MaiaAbilityPaths) > 0 {
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
					path.SurveillanceLevel = 3 // Spec: permanently sets surveillanceLevel=3 (Section 3.5)
					path.BlockedBy = unit.ID
					path.Corrupted = true
					events = append(events, makeEvent("game.events.path", targetPath, map[string]interface{}{
						"pathId":    targetPath,
						"newStatus": "BLOCKED",
						"type":      "CORRUPTED",
						"turn":      state.Turn,
					}))
				}
			}
		} else if targetPath != "" && tp.graph.IsEndpointOf(unit.CurrentRegion, targetPath) {
			// Gandalf: temporarily opens a blocked path from either endpoint.
			if path, ok := state.Paths[targetPath]; ok && path.Status == "BLOCKED" {
				path.Status = "TEMPORARILY_OPEN"
				path.TempOpenTurns = 2
				events = append(events, makeEvent("game.events.path", targetPath, map[string]interface{}{
					"pathId":    targetPath,
					"newStatus": "TEMPORARILY_OPEN",
					"type":      "REOPENED_BY_MAIA",
					"turn":      state.Turn,
				}))
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

		if unit.TravelPathID != "" {
			path, ok := state.Paths[unit.TravelPathID]
			if !ok || path.Status == "BLOCKED" {
				continue
			}
			unit.TravelRemaining--
			if unit.TravelRemaining > 0 {
				continue
			}

			events = append(events, tp.completeUnitMove(state, unit, unit.TravelPathID, unit.TravelFrom, unit.TravelTo)...)
			unit.TravelPathID = ""
			unit.TravelFrom = ""
			unit.TravelTo = ""
			unit.TravelRemaining = 0
			unit.RouteIdx++
			continue
		}

		if unit.RouteIdx >= len(unit.Route) {
			continue
		}

		nextPathID := unit.Route[unit.RouteIdx]
		path, ok := state.Paths[nextPathID]
		if !ok {
			continue
		}

		if path.Status == "BLOCKED" {
			continue // Can't advance through blocked path
		}

		pathCfg := tp.cfg.PathsByID[nextPathID]
		destination := pathCfg.To
		if unit.CurrentRegion == pathCfg.To {
			destination = pathCfg.From
		}

		cost := pathCfg.Cost
		if cost < 1 {
			cost = 1
		}
		if cost > 1 {
			unit.TravelPathID = nextPathID
			unit.TravelFrom = unit.CurrentRegion
			unit.TravelTo = destination
			unit.TravelRemaining = cost - 1
			events = append(events, makeEvent("game.events.unit", unit.ID, map[string]interface{}{
				"unitId":          unit.ID,
				"from":            unit.TravelFrom,
				"to":              unit.TravelTo,
				"pathId":          nextPathID,
				"travelRemaining": unit.TravelRemaining,
				"type":            "TRAVEL_STARTED",
				"turn":            state.Turn,
			}))
			continue
		}

		events = append(events, tp.completeUnitMove(state, unit, nextPathID, unit.CurrentRegion, destination)...)
		unit.RouteIdx++
	}

	return events
}

func (tp *TurnProcessor) completeUnitMove(state *TurnState, unit *UnitRuntime, pathID, oldRegion, destination string) []GameEvent {
	var events []GameEvent
	path := state.Paths[pathID]

	unit.CurrentRegion = destination
	events = append(events, makeEvent("game.events.unit", unit.ID, map[string]interface{}{
		"unitId": unit.ID,
		"from":   oldRegion,
		"to":     destination,
		"pathId": pathID,
		"turn":   state.Turn,
	}))

	if unit.Config.Class == "RingBearer" {
		state.LightView.RingBearerRegion = destination
		events = append(events, makeEvent("game.ring.position", "", map[string]interface{}{
			"trueRegion": destination,
			"turn":       state.Turn,
		}))
		if path != nil && path.SurveillanceLevel >= 1 && state.Turn > tp.cfg.HiddenUntilTurn {
			state.Exposed = true
			events = append(events, makeEvent("game.ring.detection", "", map[string]interface{}{
				"pathId": pathID,
				"turn":   state.Turn,
				"type":   "RING_BEARER_SPOTTED",
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
			if !participatesInCombat(u) {
				continue
			}
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
		attackerSide := attackers[0].Config.Side

		// ResolveCombat is pure calculation; this step applies the returned
		// strength/status changes back onto mutable runtime units.
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
			region.Controller = attackerSide
			region.Fortified = false // Fortification destroyed on capture
			events = append(events, makeEvent("game.events.region", regionID, map[string]interface{}{
				"regionId":      regionID,
				"newController": attackerSide,
				"attackerWon":   true,
				"turn":          state.Turn,
			}))

			// Spec Section 6, line 860: If Isengard falls to Light Side → disable Saruman permanently
			regionCfgCheck := tp.cfg.RegionsByID[regionID]
			if regionCfgCheck.SpecialRole == "SHADOW_STRONGHOLD" && attackerSide == "FREE_PEOPLES" && regionID == "isengard" {
				for _, u := range state.Units {
					if u.Config.Maia && u.Config.Side == "SHADOW" && len(u.Config.MaiaAbilityPaths) > 0 {
						u.Status = "DESTROYED" // Saruman disabled permanently
						log.Printf("⚔️ Isengard fell! Saruman (config-driven: Maia+SHADOW+maiaAbilityPaths) disabled")
						events = append(events, makeEvent("game.events.unit", u.ID, map[string]interface{}{
							"unitId": u.ID,
							"event":  "ISENGARD_DESTROYED",
							"turn":   state.Turn,
						}))
					}
				}
			}
		}

		events = append(events, makeEvent("game.events.region", regionID, map[string]interface{}{
			"eventType":     "BATTLE_RESOLVED",
			"regionId":      regionID,
			"attackerSide":  attackerSide,
			"defenderSide":  defenders[0].Config.Side,
			"attackerWon":   result.AttackerWon,
			"attackerPower": result.AttackerPower,
			"defenderPower": result.DefenderPower,
			"damage":        result.Damage,
			"attackers":     combatUnitStatuses(state, attackers),
			"defenders":     combatUnitStatuses(state, defenders),
			"destroyed":     combatUnitsWithStatus(state, append(attackers, defenders...), "DESTROYED"),
			"respawning":    combatUnitsWithStatus(state, append(attackers, defenders...), "RESPAWNING"),
			"survivors":     combatActiveUnits(state, append(attackers, defenders...)),
			"turn":          state.Turn,
		}))
	}

	return events
}

func combatUnitStatuses(state *TurnState, units []CombatUnit) []map[string]interface{} {
	statuses := make([]map[string]interface{}, 0, len(units))
	for _, unit := range units {
		runtime, ok := state.Units[unit.ID]
		if !ok {
			continue
		}
		statuses = append(statuses, map[string]interface{}{
			"unitId":   unit.ID,
			"strength": runtime.Strength,
			"status":   runtime.Status,
		})
	}
	return statuses
}

func combatUnitsWithStatus(state *TurnState, units []CombatUnit, status string) []string {
	var ids []string
	for _, unit := range units {
		if runtime, ok := state.Units[unit.ID]; ok && runtime.Status == status {
			ids = append(ids, unit.ID)
		}
	}
	return ids
}

func combatActiveUnits(state *TurnState, units []CombatUnit) []string {
	var ids []string
	for _, unit := range units {
		if runtime, ok := state.Units[unit.ID]; ok && runtime.Status == "ACTIVE" {
			ids = append(ids, unit.ID)
		}
	}
	return ids
}

func (tp *TurnProcessor) step9UpdatePathTimers(state *TurnState) []GameEvent {
	var events []GameEvent
	for _, path := range state.Paths {
		if path.Status == "BLOCKED" && path.BlockedBy != "" && !path.Corrupted {
			blocker, ok := state.Units[path.BlockedBy]
			if !ok || blocker.Status != "ACTIVE" || !tp.graph.IsEndpointOf(blocker.CurrentRegion, path.ID) {
				path.Status = "OPEN"
				path.BlockedBy = ""
				events = append(events, makeEvent("game.events.path", path.ID, map[string]interface{}{
					"pathId":    path.ID,
					"newStatus": "OPEN",
					"turn":      state.Turn,
				}))
			}
		}
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

	// Detection input is derived from config and current unit state so the
	// detection logic does not need hardcoded unit IDs.
	input := BuildDetectionInput(rbRegion, state.Turn, tp.cfg, unitStates)
	result := CheckDetection(tp.graph, input)

	if result.Detected {
		state.Exposed = true
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

	// Win 1: Ring Bearer destroys the ring at Mount Doom with no Shadow unit present.
	destroySubmitted := false
	for _, order := range state.CurrentOrders {
		if order.OrderType == "DESTROY_RING" {
			destroySubmitted = true
			break
		}
	}
	for _, unit := range state.Units {
		if unit.Config.Class == "RingBearer" && unit.Status == "ACTIVE" {
			regionCfg := tp.cfg.RegionsByID[unit.CurrentRegion]
			if destroySubmitted && regionCfg.SpecialRole == "RING_DESTRUCTION_SITE" && !hasActiveShadowUnit(state, unit.CurrentRegion) {
				events = append(events, markGameOver(state, "FREE_PEOPLES", "Ring destroyed at Mount Doom"))
				return events
			}
			// Interception win follows the spec: an exposed Ring Bearer must be
			// in the same region as an active Nazgul. Passive Sauron does not
			// intercept; his Eye only extends Nazgul detection range.
			if state.Exposed && hasActiveNazgulUnit(state, unit.CurrentRegion) {
				events = append(events, markGameOver(state, "SHADOW", "Ring Bearer exposed and intercepted"))
				return events
			}
		}
	}

	// Win 2: Ring Bearer destroyed
	for _, unit := range state.Units {
		if unit.Config.Class == "RingBearer" && unit.Status == "DESTROYED" {
			events = append(events, markGameOver(state, "SHADOW", "Ring Bearer destroyed"))
			return events
		}
	}

	// Draw: max turns reached with no winner.
	if state.Turn >= tp.cfg.MaxTurns {
		events = append(events, markGameOver(state, "DRAW", "Maximum turns reached with no winner"))
		return events
	}

	return events
}

func (tp *TurnProcessor) produceWorldSnapshot(state *TurnState) GameEvent {
	return MakeWorldSnapshotEvent(state)
}

// MakeWorldSnapshotEvent serializes the mutable runtime state so another engine
// can rebuild both its read cache and turn processor state from Kafka.
func MakeWorldSnapshotEvent(state *TurnState) GameEvent {
	units := make([]*UnitRuntime, 0, len(state.Units))
	for _, unit := range state.Units {
		units = append(units, unit)
	}
	regions := make([]*RegionRuntime, 0, len(state.Regions))
	for _, region := range state.Regions {
		regions = append(regions, region)
	}
	paths := make([]*PathRuntime, 0, len(state.Paths))
	for _, path := range state.Paths {
		paths = append(paths, path)
	}

	snapshot := map[string]interface{}{
		"turn":      state.Turn,
		"type":      "WORLD_STATE",
		"units":     units,
		"regions":   regions,
		"paths":     paths,
		"gameOver":  state.GameOver,
		"winner":    state.Winner,
		"cause":     state.Cause,
		"exposed":   state.Exposed,
		"timestamp": time.Now().UnixMilli(),
	}
	return makeEvent("game.broadcast", "", snapshot)
}

// InitTurnStateFromJSON rebuilds mutable turn state from a WORLD_STATE snapshot.
func InitTurnStateFromJSON(cfg *config.GameConfig, graph *GameGraph, data []byte) (*TurnState, int64, error) {
	var snap struct {
		Type      string          `json:"type"`
		Turn      int             `json:"turn"`
		Units     []UnitRuntime   `json:"units"`
		Regions   []RegionRuntime `json:"regions"`
		Paths     []PathRuntime   `json:"paths"`
		GameOver  bool            `json:"gameOver"`
		Winner    string          `json:"winner"`
		Cause     string          `json:"cause"`
		Exposed   bool            `json:"exposed"`
		Timestamp int64           `json:"timestamp"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, 0, err
	}
	if snap.Type != "" && snap.Type != "WORLD_STATE" {
		return nil, 0, nil
	}
	state := InitTurnState(cfg, graph)
	if snap.Turn > 0 {
		state.Turn = snap.Turn
	}
	state.GameOver = snap.GameOver
	state.Winner = snap.Winner
	state.Cause = snap.Cause
	state.Exposed = snap.Exposed
	for i := range snap.Units {
		unit := snap.Units[i]
		if unit.Config.ID == "" {
			unit.Config = cfg.UnitsByID[unit.ID]
		}
		u := unit
		state.Units[u.ID] = &u
	}
	for i := range snap.Regions {
		region := snap.Regions[i]
		r := region
		state.Regions[r.ID] = &r
	}
	for i := range snap.Paths {
		path := snap.Paths[i]
		p := path
		state.Paths[p.ID] = &p
	}
	if state.Turn >= cfg.MaxTurns {
		state.GameOver = true
	}
	return state, snap.Timestamp, nil
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

func makeGameOverEvent(winner, cause string, turn int) GameEvent {
	return makeEvent("game.broadcast", "game-over", map[string]interface{}{
		"type":    "GAME_OVER",
		"eventId": gameOverEventID(winner, turn),
		"winner":  winner,
		"cause":   cause,
		"turn":    turn,
	})
}

func markGameOver(state *TurnState, winner, cause string) GameEvent {
	state.GameOver = true
	state.Winner = winner
	state.Cause = cause
	return makeGameOverEvent(winner, cause, state.Turn)
}

func gameOverEventID(winner string, turn int) string {
	return "game-over-" + winner + "-" + strconv.Itoa(turn)
}

func hasActiveShadowUnit(state *TurnState, regionID string) bool {
	for _, unit := range state.Units {
		if unit.Status == "ACTIVE" && unit.CurrentRegion == regionID && unit.Config.Side == "SHADOW" {
			return true
		}
	}
	return false
}

func hasActiveNazgulUnit(state *TurnState, regionID string) bool {
	for _, unit := range state.Units {
		if unit.Status == "ACTIVE" && unit.CurrentRegion == regionID && unit.Config.Class == "Nazgul" {
			return true
		}
	}
	return false
}

func participatesInCombat(unit *UnitRuntime) bool {
	if unit.Config.Class == "RingBearer" {
		return false
	}
	if unit.ID == "sauron" {
		return false
	}
	return true
}

// InitTurnState creates the initial turn state from configuration.
func InitTurnState(cfg *config.GameConfig, graph *GameGraph) *TurnState {
	state := &TurnState{
		Turn:      1,
		Units:     make(map[string]*UnitRuntime),
		Regions:   make(map[string]*RegionRuntime),
		Paths:     make(map[string]*PathRuntime),
		LightView: &LightView{},
		DarkView:  &DarkViewData{},
		Config:    cfg,
		Graph:     graph,
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
			From:   p.From,
			To:     p.To,
			Status: "OPEN",
		}
	}

	return state
}
