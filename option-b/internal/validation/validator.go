// Package validation implements the 8 order validation rules.
// Invalid orders are sent to game.dlq with appropriate error codes.
package validation

import (
	"fmt"

	"rotr/internal/cache"
	"rotr/internal/config"
)

// ═══════════════════════════════════════════════════════
// ORDER STRUCTURE
// ═══════════════════════════════════════════════════════

// Order represents a player-submitted order.
type Order struct {
	OrderType    string                 `json:"orderType"`
	PlayerID     string                 `json:"playerId"`
	PlayerSide   string                 `json:"playerSide,omitempty"`
	UnitID       string                 `json:"unitId"`
	Turn         int                    `json:"turn"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
	PathID       string                 `json:"pathId,omitempty"`
	PathIDs      []string               `json:"pathIds,omitempty"`
	NewPathIDs   []string               `json:"newPathIds,omitempty"`
	TargetRegion string                 `json:"targetRegion,omitempty"`
	TargetPathID string                 `json:"targetPathId,omitempty"`
}

// ValidationResult contains the outcome of order validation.
type ValidationResult struct {
	Valid     bool
	ErrorCode string
	ErrorMsg  string
}

// Error codes matching the specification
const (
	ErrWrongTurn          = "WRONG_TURN"
	ErrNotYourUnit        = "NOT_YOUR_UNIT"
	ErrPathBlocked        = "PATH_BLOCKED"
	ErrInvalidPath        = "INVALID_PATH"
	ErrUnitNotAdjacent    = "UNIT_NOT_ADJACENT"
	ErrInvalidTarget      = "INVALID_TARGET"
	ErrAbilityOnCooldown  = "ABILITY_ON_COOLDOWN"
	ErrDuplicateUnitOrder = "DUPLICATE_UNIT_ORDER"
	ErrMaiaDisabled       = "MAIA_DISABLED"
	ErrDestroyCondition   = "DESTROY_CONDITION_NOT_MET"
)

// ═══════════════════════════════════════════════════════
// VALIDATOR
// ═══════════════════════════════════════════════════════

// Validator validates orders against the 8 rules.
type Validator struct {
	cfg               *config.GameConfig
	cache             *cache.WorldStateCache
	processedThisTurn map[string]bool // unitID → already has order this turn
}

// NewValidator creates a new order validator.
func NewValidator(cfg *config.GameConfig, c *cache.WorldStateCache) *Validator {
	return &Validator{
		cfg:               cfg,
		cache:             c,
		processedThisTurn: make(map[string]bool),
	}
}

// ResetTurn clears the duplicate tracking for a new turn.
func (v *Validator) ResetTurn() {
	v.processedThisTurn = make(map[string]bool)
}

// Validate applies all 8 rules to an order.
// Returns the first failing rule's error, or Valid=true if all pass.
func (v *Validator) Validate(order Order) ValidationResult {
	// Rule 1: Turn number matches current turn
	if result := v.rule1TurnNumber(order); !result.Valid {
		return result
	}

	// Rule 2: Unit belongs to submitting player's side
	if result := v.rule2UnitOwnership(order); !result.Valid {
		return result
	}

	// Rule 3: Ring Bearer route — next path is not BLOCKED
	if result := v.rule3PathBlocked(order); !result.Valid {
		return result
	}

	// Rule 4: Ring Bearer route — path in assigned route
	if result := v.rule4PathInRoute(order); !result.Valid {
		return result
	}

	// Rule 5: BlockPath/SearchPath — unit at endpoint
	if result := v.rule5UnitAtEndpoint(order); !result.Valid {
		return result
	}

	// Rule 6: AttackRegion — target valid
	if result := v.rule6AttackTarget(order); !result.Valid {
		return result
	}

	// Rule 7: MaiaAbility — cooldown expired
	if result := v.rule7MaiaCooldown(order); !result.Valid {
		return result
	}

	// Rule 8: Duplicate unit order this turn
	if result := v.rule8Duplicate(order); !result.Valid {
		return result
	}

	// Mark as processed
	v.processedThisTurn[order.UnitID] = true

	return ValidationResult{Valid: true}
}

// ═══════════════════════════════════════════════════════
// 8 VALIDATION RULES
// ═══════════════════════════════════════════════════════

func (v *Validator) rule1TurnNumber(order Order) ValidationResult {
	snap := v.cache.GetSnapshot()
	if order.Turn != snap.Turn {
		return ValidationResult{
			ErrorCode: ErrWrongTurn,
			ErrorMsg:  fmt.Sprintf("order turn %d does not match current turn %d", order.Turn, snap.Turn),
		}
	}
	return ValidationResult{Valid: true}
}

func (v *Validator) rule2UnitOwnership(order Order) ValidationResult {
	unitCfg, ok := v.cfg.UnitsByID[order.UnitID]
	if !ok {
		return ValidationResult{
			ErrorCode: ErrNotYourUnit,
			ErrorMsg:  fmt.Sprintf("unit %s not found", order.UnitID),
		}
	}

	playerSide := order.PlayerSide
	if playerSide != "FREE_PEOPLES" && playerSide != "SHADOW" {
		// Backward compatibility for older clients that only send playerId.
		playerSide = "FREE_PEOPLES"
		if order.PlayerID == "dark-player" || order.PlayerID == "dark-opponent" {
			playerSide = "SHADOW"
		}
	}

	if unitCfg.Side != playerSide {
		return ValidationResult{
			ErrorCode: ErrNotYourUnit,
			ErrorMsg:  fmt.Sprintf("unit %s belongs to %s, not your side", order.UnitID, unitCfg.Side),
		}
	}

	return ValidationResult{Valid: true}
}

func (v *Validator) rule3PathBlocked(order Order) ValidationResult {
	if order.OrderType != "ASSIGN_ROUTE" && order.OrderType != "REDIRECT_UNIT" {
		return ValidationResult{Valid: true}
	}

	unitCfg, ok := v.cfg.UnitsByID[order.UnitID]
	if !ok || unitCfg.Class != "RingBearer" {
		return ValidationResult{Valid: true}
	}

	// Check if the first path in the route is blocked
	pathIDs := order.PathIDs
	if order.OrderType == "REDIRECT_UNIT" {
		pathIDs = order.NewPathIDs
	}

	snap := v.cache.GetSnapshot()
	if len(pathIDs) > 0 {
		for _, p := range snap.Paths {
			if p.ID == pathIDs[0] && p.Status == "BLOCKED" {
				return ValidationResult{
					ErrorCode: ErrPathBlocked,
					ErrorMsg:  fmt.Sprintf("next path %s is BLOCKED", pathIDs[0]),
				}
			}
		}
	}

	return ValidationResult{Valid: true}
}

func (v *Validator) rule4PathInRoute(order Order) ValidationResult {
	if order.OrderType != "ASSIGN_ROUTE" && order.OrderType != "REDIRECT_UNIT" {
		return ValidationResult{Valid: true}
	}
	pathIDs := order.PathIDs
	if order.OrderType == "REDIRECT_UNIT" {
		pathIDs = order.NewPathIDs
	}
	if len(pathIDs) == 0 {
		return ValidationResult{Valid: true}
	}

	snap := v.cache.GetSnapshot()
	currentRegion := ""
	for _, u := range snap.Units {
		if u.ID == order.UnitID {
			currentRegion = u.CurrentRegion
			break
		}
	}
	if currentRegion == "" {
		return ValidationResult{ErrorCode: ErrInvalidPath, ErrorMsg: "unit has no current region"}
	}

	for _, pathID := range pathIDs {
		pathCfg, ok := v.cfg.PathsByID[pathID]
		if !ok {
			return ValidationResult{ErrorCode: ErrInvalidPath, ErrorMsg: fmt.Sprintf("path %s not found", pathID)}
		}
		if currentRegion == pathCfg.From {
			currentRegion = pathCfg.To
			continue
		}
		if currentRegion == pathCfg.To {
			currentRegion = pathCfg.From
			continue
		}
		return ValidationResult{
			ErrorCode: ErrInvalidPath,
			ErrorMsg:  fmt.Sprintf("path %s is not connected to current route position %s", pathID, currentRegion),
		}
	}
	return ValidationResult{Valid: true}
}

func (v *Validator) rule5UnitAtEndpoint(order Order) ValidationResult {
	if order.OrderType != "BLOCK_PATH" && order.OrderType != "SEARCH_PATH" {
		return ValidationResult{Valid: true}
	}

	pathID := order.PathID
	if pathID == "" {
		return ValidationResult{Valid: true}
	}

	pathCfg, ok := v.cfg.PathsByID[pathID]
	if !ok {
		return ValidationResult{
			ErrorCode: ErrUnitNotAdjacent,
			ErrorMsg:  fmt.Sprintf("path %s not found", pathID),
		}
	}

	// Check unit is at one of the path's endpoints
	snap := v.cache.GetSnapshot()
	var unitSnap *cache.UnitSnapshot
	for i := range snap.Units {
		if snap.Units[i].ID == order.UnitID {
			unitSnap = &snap.Units[i]
			break
		}
	}
	if unitSnap == nil {
		return ValidationResult{
			ErrorCode: ErrUnitNotAdjacent,
			ErrorMsg:  "unit not found in state",
		}
	}

	if unitSnap.CurrentRegion != pathCfg.From && unitSnap.CurrentRegion != pathCfg.To {
		return ValidationResult{
			ErrorCode: ErrUnitNotAdjacent,
			ErrorMsg:  fmt.Sprintf("unit at %s, not at endpoint of %s", unitSnap.CurrentRegion, pathID),
		}
	}

	return ValidationResult{Valid: true}
}

func (v *Validator) rule6AttackTarget(order Order) ValidationResult {
	if order.OrderType == "DESTROY_RING" {
		return v.validateDestroyRing(order)
	}
	if order.OrderType != "ATTACK_REGION" {
		return ValidationResult{Valid: true}
	}

	if order.TargetRegion == "" {
		return ValidationResult{
			ErrorCode: ErrInvalidTarget,
			ErrorMsg:  "no target region specified",
		}
	}

	snap := v.cache.GetSnapshot()
	unitRegion := ""
	for _, u := range snap.Units {
		if u.ID == order.UnitID {
			unitRegion = u.CurrentRegion
			break
		}
	}
	if unitRegion == "" {
		return ValidationResult{ErrorCode: ErrInvalidTarget, ErrorMsg: "attacking unit not found in state"}
	}
	if unitRegion == order.TargetRegion {
		return ValidationResult{Valid: true}
	}
	for _, p := range v.cfg.Paths {
		if (p.From == unitRegion && p.To == order.TargetRegion) || (p.To == unitRegion && p.From == order.TargetRegion) {
			return ValidationResult{Valid: true}
		}
	}
	return ValidationResult{
		ErrorCode: ErrInvalidTarget,
		ErrorMsg:  fmt.Sprintf("target region %s is not adjacent to %s", order.TargetRegion, unitRegion),
	}
}

func (v *Validator) rule7MaiaCooldown(order Order) ValidationResult {
	if order.OrderType != "MAIA_ABILITY" {
		return ValidationResult{Valid: true}
	}
	unitCfg, ok := v.cfg.UnitsByID[order.UnitID]
	if !ok || !unitCfg.Maia {
		return ValidationResult{ErrorCode: ErrMaiaDisabled, ErrorMsg: fmt.Sprintf("unit %s is not a Maia unit", order.UnitID)}
	}

	snap := v.cache.GetSnapshot()
	var unitSnap *cache.UnitSnapshot
	for i := range snap.Units {
		if snap.Units[i].ID == order.UnitID {
			unitSnap = &snap.Units[i]
			break
		}
	}
	if unitSnap == nil {
		return ValidationResult{Valid: true}
	}

	if unitSnap.Cooldown > 0 {
		return ValidationResult{
			ErrorCode: ErrAbilityOnCooldown,
			ErrorMsg:  fmt.Sprintf("unit %s ability on cooldown for %d more turns", order.UnitID, unitSnap.Cooldown),
		}
	}

	return ValidationResult{Valid: true}
}

func (v *Validator) rule8Duplicate(order Order) ValidationResult {
	if v.processedThisTurn[order.UnitID] {
		return ValidationResult{
			ErrorCode: ErrDuplicateUnitOrder,
			ErrorMsg:  fmt.Sprintf("unit %s already has an order this turn", order.UnitID),
		}
	}
	return ValidationResult{Valid: true}
}

func (v *Validator) validateDestroyRing(order Order) ValidationResult {
	unitCfg, ok := v.cfg.UnitsByID[order.UnitID]
	if !ok || unitCfg.Class != "RingBearer" {
		return ValidationResult{ErrorCode: ErrDestroyCondition, ErrorMsg: "only the Ring Bearer can destroy the ring"}
	}

	snap := v.cache.GetSnapshot()
	ringRegion := ""
	for _, u := range snap.Units {
		if u.ID == order.UnitID {
			ringRegion = u.CurrentRegion
			break
		}
	}
	regionCfg, ok := v.cfg.RegionsByID[ringRegion]
	if !ok || regionCfg.SpecialRole != "RING_DESTRUCTION_SITE" {
		return ValidationResult{ErrorCode: ErrDestroyCondition, ErrorMsg: "Ring Bearer is not at Mount Doom"}
	}
	for _, u := range snap.Units {
		cfg, ok := v.cfg.UnitsByID[u.ID]
		if ok && cfg.Side == "SHADOW" && u.Status == "ACTIVE" && u.CurrentRegion == ringRegion {
			return ValidationResult{ErrorCode: ErrDestroyCondition, ErrorMsg: "Shadow unit present at Mount Doom"}
		}
	}
	return ValidationResult{Valid: true}
}
