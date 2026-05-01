package game

import (
	"rotr/internal/config"
	"testing"
)

// ═══════════════════════════════════════════════════════
// COMBAT TEST — 6 required cases (Rubric B3)
// Run: go test -race ./internal/game/...
// ═══════════════════════════════════════════════════════

// Case 1: Attacker(5) vs Defender(5, PLAINS) → tie, attacker repelled
func TestCombat_PlainsTie(t *testing.T) {
	attacker := CombatUnit{
		ID: "a1", Strength: 5,
		Config: config.UnitConfig{ID: "a1", Side: "SHADOW", Strength: 5},
	}
	defender := CombatUnit{
		ID: "d1", Strength: 5,
		Config: config.UnitConfig{ID: "d1", Side: "FREE_PEOPLES", Strength: 5},
	}

	result := ResolveCombat([]CombatUnit{attacker}, []CombatUnit{defender}, "PLAINS", false)

	if result.AttackerWon {
		t.Errorf("expected defender to hold on PLAINS tie, but attacker won")
	}
	if result.AttackerPower != 5 || result.DefenderPower != 5 {
		t.Errorf("expected 5v5, got %dv%d", result.AttackerPower, result.DefenderPower)
	}
	// Each attacker loses 1 strength
	if result.UpdatedAttackers[0].Strength != 4 {
		t.Errorf("expected attacker strength=4 after repel, got %d", result.UpdatedAttackers[0].Strength)
	}
}

// Case 2: Attacker(5) vs Defender(5, FORTRESS) → defender wins (5 vs 7)
func TestCombat_FortressDefense(t *testing.T) {
	attacker := CombatUnit{
		ID: "a1", Strength: 5,
		Config: config.UnitConfig{ID: "a1", Side: "SHADOW", Strength: 5},
	}
	defender := CombatUnit{
		ID: "d1", Strength: 5,
		Config: config.UnitConfig{ID: "d1", Side: "FREE_PEOPLES", Strength: 5},
	}

	result := ResolveCombat([]CombatUnit{attacker}, []CombatUnit{defender}, "FORTRESS", false)

	if result.AttackerWon {
		t.Errorf("expected defender to hold in FORTRESS")
	}
	if result.DefenderPower != 7 {
		t.Errorf("expected defender power=7 (5+2 fortress), got %d", result.DefenderPower)
	}
}

// Case 3: UrukHai(5, ignoresFortress) vs Defender(5, FORTRESS) → tie (5 vs 5)
func TestCombat_IgnoresFortress(t *testing.T) {
	urukhai := CombatUnit{
		ID: "uh", Strength: 5,
		Config: config.UnitConfig{ID: "uh", Side: "SHADOW", Strength: 5, IgnoresFortress: true},
	}
	defender := CombatUnit{
		ID: "d1", Strength: 5,
		Config: config.UnitConfig{ID: "d1", Side: "FREE_PEOPLES", Strength: 5},
	}

	result := ResolveCombat([]CombatUnit{urukhai}, []CombatUnit{defender}, "FORTRESS", false)

	if result.AttackerWon {
		t.Errorf("expected tie (5v5), not attacker win")
	}
	// With ignoresFortress, terrain bonus is NOT added → 5 vs 5
	if result.DefenderPower != 5 {
		t.Errorf("expected defender power=5 (ignoresFortress), got %d", result.DefenderPower)
	}
}

// Case 4: UrukHai(5) vs Defender(5, FORTRESS, fortified) → defender wins (5 vs 7)
// ignoresFortress skips terrain bonus but fortification still applies
func TestCombat_IgnoresFortressButFortified(t *testing.T) {
	urukhai := CombatUnit{
		ID: "uh", Strength: 5,
		Config: config.UnitConfig{ID: "uh", Side: "SHADOW", Strength: 5, IgnoresFortress: true},
	}
	defender := CombatUnit{
		ID: "d1", Strength: 5,
		Config: config.UnitConfig{ID: "d1", Side: "FREE_PEOPLES", Strength: 5},
	}

	result := ResolveCombat([]CombatUnit{urukhai}, []CombatUnit{defender}, "FORTRESS", true)

	if result.AttackerWon {
		t.Errorf("expected defender to hold (5 vs 7: fortified)")
	}
	// ignoresFortress skips terrain (+2) but fortification (+2) still applies
	if result.DefenderPower != 7 {
		t.Errorf("expected defender power=7 (0 terrain + 2 fortification + 5), got %d", result.DefenderPower)
	}
}

// Case 5: Leadership bonus applied correctly to co-located allies
// Aragorn(5, leader+1) + Gimli(3) → Gimli effective=4, total=5+4=9
func TestCombat_LeadershipBonus(t *testing.T) {
	aragorn := CombatUnit{
		ID: "aragorn", Strength: 5,
		Config: config.UnitConfig{
			ID: "aragorn", Side: "FREE_PEOPLES", Strength: 5,
			Leadership: true, LeadershipBonus: 1,
		},
	}
	gimli := CombatUnit{
		ID: "gimli", Strength: 3,
		Config: config.UnitConfig{
			ID: "gimli", Side: "FREE_PEOPLES", Strength: 3,
		},
	}
	defender := CombatUnit{
		ID: "d1", Strength: 5,
		Config: config.UnitConfig{ID: "d1", Side: "SHADOW", Strength: 5},
	}

	result := ResolveCombat(
		[]CombatUnit{aragorn, gimli},
		[]CombatUnit{defender},
		"PLAINS", false,
	)

	// Aragorn: 5 (no bonus, he IS the leader)
	// Gimli: 3 + 1 (Aragorn's leadership) = 4
	// Total attacker: 5 + 4 = 9
	if result.AttackerPower != 9 {
		t.Errorf("expected attacker power=9 (5+4 with leadership), got %d", result.AttackerPower)
	}
	if !result.AttackerWon {
		t.Errorf("expected attacker to win (9 vs 5)")
	}
}

// Case 6: Indestructible unit — strength floors at 1
func TestCombat_Indestructible(t *testing.T) {
	attacker := CombatUnit{
		ID: "a1", Strength: 10,
		Config: config.UnitConfig{ID: "a1", Side: "FREE_PEOPLES", Strength: 10},
	}
	witchKing := CombatUnit{
		ID: "wk", Strength: 5,
		Config: config.UnitConfig{
			ID: "wk", Side: "SHADOW", Strength: 5,
			Indestructible: true,
		},
	}

	result := ResolveCombat([]CombatUnit{attacker}, []CombatUnit{witchKing}, "PLAINS", false)

	if !result.AttackerWon {
		t.Errorf("expected attacker to win (10 vs 5)")
	}

	// Witch-King is indestructible — strength floors at 1
	for _, u := range result.UpdatedDefenders {
		if u.ID == "wk" {
			if u.Strength < 1 {
				t.Errorf("indestructible unit should have strength >= 1, got %d", u.Strength)
			}
		}
	}
}
