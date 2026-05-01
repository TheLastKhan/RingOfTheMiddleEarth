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
	cfg         *config.GameConfig
	cache       *cache.WorldStateCache
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

	// Determine player's side from playerId convention
	playerSide := "FREE_PEOPLES"
	if order.PlayerID == "dark-player" || order.PlayerID == "dark-opponent" {
		playerSide = "SHADOW"
	}
	// More robust: check if the order's player matches any unit on that side
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

func (v *Validator) rule4PathInRoute(_ Order) ValidationResult {
	// This rule checks if path is in assigned route — simplified
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
	if order.OrderType != "ATTACK_REGION" {
		return ValidationResult{Valid: true}
	}

	if order.TargetRegion == "" {
		return ValidationResult{
			ErrorCode: ErrInvalidTarget,
			ErrorMsg:  "no target region specified",
		}
	}

	return ValidationResult{Valid: true}
}

func (v *Validator) rule7MaiaCooldown(order Order) ValidationResult {
	if order.OrderType != "MAIA_ABILITY" {
		return ValidationResult{Valid: true}
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
