// Package game — detection.go implements the Nazgul detection logic.
// Detection range is entirely config-driven — no hardcoded unit IDs.
package game

import (
	"rotr/internal/config"
)

// ═══════════════════════════════════════════════════════
// DETECTION — Ring Bearer detection by Nazgul
// ═══════════════════════════════════════════════════════

// DetectionInput holds the state needed for detection calculation.
type DetectionInput struct {
	RingBearerRegion string
	RingBearerTurn   int
	HiddenUntilTurn  int
	NazgulPositions  []NazgulPosition
	SauronActive     bool   // Sauron is not destroyed
	SauronRegion     string // Where is Sauron
	SauronBaseRegion string // Sauron's stronghold (from config.StartRegion)
}

// NazgulPosition holds one Nazgul's location and config.
type NazgulPosition struct {
	UnitID         string
	Region         string
	DetectionRange int // from config, not hardcoded
	Status         string
}

// DetectionResult holds whether Ring Bearer was detected and by whom.
type DetectionResult struct {
	Detected bool
	ByUnit   string // which Nazgul detected
	Region   string // detected in which region
}

// CheckDetection checks if any Nazgul detects the Ring Bearer this turn.
//
// Rules:
//   1. If turn <= hiddenUntilTurn → suppressed (no detection)
//   2. Each Nazgul's effective range = config.DetectionRange
//      + 1 if Sauron is ACTIVE and at his base region (Eye of Sauron bonus)
//   3. If graph.Distance(nazgul.region, ringBearer.region) <= effectiveRange → detected
//
// ALL detection logic uses config values. No unit ID string matching.
func CheckDetection(graph *GameGraph, input DetectionInput) DetectionResult {
	// Rule 1: suppression period
	if input.RingBearerTurn <= input.HiddenUntilTurn {
		return DetectionResult{Detected: false}
	}

	// Calculate Sauron's Eye bonus
	sauronBonus := 0
	if input.SauronActive && input.SauronRegion == input.SauronBaseRegion {
		sauronBonus = 1
	}

	// Check each Nazgul
	for _, nazgul := range input.NazgulPositions {
		if nazgul.Status != "ACTIVE" {
			continue
		}
		if nazgul.DetectionRange <= 0 {
			continue
		}

		effectiveRange := nazgul.DetectionRange + sauronBonus
		distance := graph.Distance(nazgul.Region, input.RingBearerRegion)

		if distance >= 0 && distance <= effectiveRange {
			return DetectionResult{
				Detected: true,
				ByUnit:   nazgul.UnitID,
				Region:   input.RingBearerRegion,
			}
		}
	}

	return DetectionResult{Detected: false}
}

// BuildDetectionInput builds the detection input from the current game state.
// This demonstrates config-driven approach — we look at config fields,
// never at unit ID strings.
func BuildDetectionInput(
	ringBearerRegion string,
	turn int,
	cfg *config.GameConfig,
	unitStates map[string]UnitState,
) DetectionInput {
	input := DetectionInput{
		RingBearerRegion: ringBearerRegion,
		RingBearerTurn:   turn,
		HiddenUntilTurn:  cfg.HiddenUntilTurn,
	}

	for _, unitCfg := range cfg.Units {
		state, ok := unitStates[unitCfg.ID]
		if !ok {
			continue
		}

		// Nazgul detection — identified by config.DetectionRange > 0
		if unitCfg.DetectionRange > 0 {
			input.NazgulPositions = append(input.NazgulPositions, NazgulPosition{
				UnitID:         unitCfg.ID,
				Region:         state.CurrentRegion,
				DetectionRange: unitCfg.DetectionRange,
				Status:         state.Status,
			})
		}

		// Sauron — identified by config.Maia && config.Indestructible && side SHADOW
		// (The Eye of Sauron is a passive ability of the indestructible Maia)
		if unitCfg.Maia && unitCfg.Indestructible && unitCfg.Side == "SHADOW" {
			input.SauronActive = state.Status == "ACTIVE"
			input.SauronRegion = state.CurrentRegion
			input.SauronBaseRegion = unitCfg.StartRegion
		}
	}

	return input
}

// UnitState represents the runtime state of a unit (used by detection).
type UnitState struct {
	CurrentRegion string
	Status        string
}
