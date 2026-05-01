// Package game — combat.go implements the combat resolution formula.
// All modifiers (terrain, fortification, leadership, ignoresFortress, indestructible)
// are driven by configuration — no unit ID string literal appears here.
package game

import (
	"rotr/internal/config"
)

// ═══════════════════════════════════════════════════════
// TYPES
// ═══════════════════════════════════════════════════════

// CombatUnit represents a unit participating in combat.
type CombatUnit struct {
	ID       string
	Strength int
	Config   config.UnitConfig
}

// CombatResult contains the outcome of a battle.
type CombatResult struct {
	AttackerWon     bool
	AttackerPower   int
	DefenderPower   int
	Damage          int
	UpdatedAttackers []CombatUnit
	UpdatedDefenders []CombatUnit
}

// ═══════════════════════════════════════════════════════
// TERRAIN BONUS
// ═══════════════════════════════════════════════════════

// TerrainBonus returns the defensive terrain bonus for a region type.
//   FORTRESS  → +2
//   MOUNTAINS → +1
//   others    → 0
func TerrainBonus(terrain string) int {
	switch terrain {
	case "FORTRESS":
		return 2
	case "MOUNTAINS":
		return 1
	default:
		return 0
	}
}

// ═══════════════════════════════════════════════════════
// EFFECTIVE STRENGTH (with leadership bonus)
// ═══════════════════════════════════════════════════════

// effectiveStrength computes a unit's effective strength including
// leadership bonus from co-located leaders on the same side.
func effectiveStrength(unit CombatUnit, allUnits []CombatUnit) int {
	str := unit.Strength

	// Check if any co-located ally is a leader
	for _, ally := range allUnits {
		if ally.ID == unit.ID {
			continue // skip self
		}
		if ally.Config.Side != unit.Config.Side {
			continue // different side
		}
		if ally.Config.Leadership {
			str += ally.Config.LeadershipBonus
		}
	}

	return str
}

// ═══════════════════════════════════════════════════════
// COMBAT RESOLUTION
// ═══════════════════════════════════════════════════════

// ResolveCombat resolves a battle between attackers and defenders in a region.
//
// Formula:
//   attacker_power = sum of attackers' effective strengths
//   defender_power = sum of defenders' effective strengths
//                  + terrain_bonus  (skipped if any attacker has ignoresFortress)
//                  + fortification_bonus
//
//   if attacker_power > defender_power:
//     damage = attacker_power - defender_power
//     region control → attacker's side
//   else:
//     each attacker loses 1 strength
//     region control unchanged
func ResolveCombat(attackers, defenders []CombatUnit, terrain string, fortified bool) CombatResult {
	// ── Attacker power ──
	allCombatants := append(append([]CombatUnit{}, attackers...), defenders...)
	attackerPower := 0
	for _, u := range attackers {
		attackerPower += effectiveStrength(u, allCombatants)
	}

	// ── Defender power ──
	defenderPower := 0
	for _, u := range defenders {
		defenderPower += effectiveStrength(u, allCombatants)
	}

	// Terrain bonus — skipped if any attacker has ignoresFortress
	anyIgnoresFortress := false
	for _, u := range attackers {
		if u.Config.IgnoresFortress {
			anyIgnoresFortress = true
			break
		}
	}
	if !anyIgnoresFortress {
		defenderPower += TerrainBonus(terrain)
	}

	// Fortification bonus — always applies (even with ignoresFortress)
	if fortified {
		defenderPower += 2
	}

	// ── Resolution ──
	result := CombatResult{
		AttackerPower: attackerPower,
		DefenderPower: defenderPower,
	}

	if attackerPower > defenderPower {
		// Attacker wins
		result.AttackerWon = true
		result.Damage = attackerPower - defenderPower
		result.UpdatedAttackers = attackers // unchanged
		result.UpdatedDefenders = applyDamageToDefenders(defenders, result.Damage)
	} else {
		// Defender holds — each attacker loses 1 strength
		result.AttackerWon = false
		result.UpdatedAttackers = applyRepelDamage(attackers)
		result.UpdatedDefenders = defenders
	}

	return result
}

// applyDamageToDefenders distributes damage across defenders.
func applyDamageToDefenders(defenders []CombatUnit, totalDamage int) []CombatUnit {
	updated := make([]CombatUnit, len(defenders))
	copy(updated, defenders)

	remaining := totalDamage
	for i := range updated {
		if remaining <= 0 {
			break
		}
		dmg := remaining
		if dmg > updated[i].Strength {
			dmg = updated[i].Strength
		}
		updated[i] = applyDamage(updated[i], dmg)
		remaining -= dmg
	}

	return updated
}

// applyRepelDamage applies 1 damage to each attacker (repelled).
func applyRepelDamage(attackers []CombatUnit) []CombatUnit {
	updated := make([]CombatUnit, len(attackers))
	for i, u := range attackers {
		updated[i] = applyDamage(u, 1)
	}
	return updated
}

// applyDamage applies damage to a single unit, respecting indestructible.
//
// Config-driven:
//   - indestructible: strength floors at 1, never destroyed
//   - respawns: strength=0 → RESPAWNING
//   - otherwise: strength=0 → DESTROYED
func applyDamage(unit CombatUnit, damage int) CombatUnit {
	raw := unit.Strength - damage

	if unit.Config.Indestructible {
		if raw < 1 {
			raw = 1
		}
		unit.Strength = raw
		return unit
	}

	if raw <= 0 {
		unit.Strength = 0
		// Status is determined by caller using config.Respawns
		return unit
	}

	unit.Strength = raw
	return unit
}
