package pipeline

import (
	"context"
	"rotr/internal/config"
	"rotr/internal/game"
	"testing"
)

// ═══════════════════════════════════════════════════════
// PIPELINE 1 TEST — 2 required cases (Rubric B8)
// ═══════════════════════════════════════════════════════

// Case 1: Route with known threat and surveillance values → correct riskScore
func TestPipeline1_KnownRiskScore(t *testing.T) {
	state := RouteRiskState{
		Regions: map[string]RegionSnapshot{
			"weathertop": {ID: "weathertop", ThreatLevel: 2},
			"moria":      {ID: "moria", ThreatLevel: 3},
			"lothlorien": {ID: "lothlorien", ThreatLevel: 0},
		},
		Paths: map[string]PathSnapshot{
			"weathertop-to-rivendell": {ID: "weathertop-to-rivendell", Status: "OPEN", SurveillanceLevel: 1},
			"rivendell-to-moria":      {ID: "rivendell-to-moria", Status: "THREATENED", SurveillanceLevel: 0},
			"moria-to-lothlorien":     {ID: "moria-to-lothlorien", Status: "OPEN", SurveillanceLevel: 0},
		},
		NazgulUnits: nil, // No Nazgul for this test
		Graph:       nil,
	}

	routes := []RouteRiskInput{
		{
			RouteID:   "test-route",
			PathIDs:   []string{"weathertop-to-rivendell", "rivendell-to-moria", "moria-to-lothlorien"},
			RegionIDs: []string{"weathertop", "moria", "lothlorien"},
		},
	}

	result := ComputeRouteRisk(context.Background(), routes, state)

	if len(result.Routes) != 1 {
		t.Fatalf("expected 1 route result, got %d", len(result.Routes))
	}

	r := result.Routes[0]

	// Expected:
	//   region threat: 2 + 3 + 0 = 5
	//   surveillance: 1 * 3 = 3
	//   THREATENED: 1 * 2 = 2
	//   BLOCKED: 0
	//   proximity: 0
	//   Total: 5 + 3 + 2 = 10
	expectedScore := 10
	if r.RiskScore != expectedScore {
		t.Errorf("expected riskScore=%d, got %d", expectedScore, r.RiskScore)
	}
}

// Case 2: Nazgul within 2 hops → proximity count adds correctly to score
func TestPipeline1_NazgulProximity(t *testing.T) {
	cfg := config.DefaultConfig()
	graph := game.NewGameGraph(cfg)

	state := RouteRiskState{
		Regions: map[string]RegionSnapshot{
			"lothlorien": {ID: "lothlorien", ThreatLevel: 0},
			"emyn-muil":  {ID: "emyn-muil", ThreatLevel: 2},
		},
		Paths: map[string]PathSnapshot{
			"lothlorien-to-emyn-muil": {ID: "lothlorien-to-emyn-muil", Status: "OPEN", SurveillanceLevel: 0},
		},
		NazgulUnits: []UnitSnapshot{
			{
				ID:            "nazgul-test",
				CurrentRegion: "dead-marshes", // 1 hop from emyn-muil
				Status:        "ACTIVE",
				Side:          "SHADOW",
				Config:        config.UnitConfig{DetectionRange: 1},
			},
		},
		Graph: graph,
	}

	routes := []RouteRiskInput{
		{
			RouteID:   "proximity-route",
			PathIDs:   []string{"lothlorien-to-emyn-muil"},
			RegionIDs: []string{"lothlorien", "emyn-muil"},
		},
	}

	result := ComputeRouteRisk(context.Background(), routes, state)

	if len(result.Routes) != 1 {
		t.Fatalf("expected 1 route result, got %d", len(result.Routes))
	}

	r := result.Routes[0]

	// Expected:
	//   region threat: 0 + 2 = 2
	//   surveillance: 0
	//   BLOCKED/THREATENED: 0
	//   proximity: 1 Nazgul within 2 hops of emyn-muil → 1 * 2 = 2
	//   Total: 2 + 2 = 4
	expectedScore := 4
	if r.RiskScore != expectedScore {
		t.Errorf("expected riskScore=%d (with proximity), got %d", expectedScore, r.RiskScore)
	}
}
