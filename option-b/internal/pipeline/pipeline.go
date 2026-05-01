// Package pipeline implements Go Pipeline 1 (Route Risk) and Pipeline 2 (Interception).
// Each pipeline uses the fan-out/fan-in pattern with buffered channels and context cancellation.
package pipeline

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"rotr/internal/config"
	"rotr/internal/game"
)

// ═══════════════════════════════════════════════════════
// PIPELINE 1 — ROUTE RISK (Light Side)
// 4 workers, buffer cap 20, 2s timeout
// ═══════════════════════════════════════════════════════

// RouteRiskInput contains the data needed to score a route.
type RouteRiskInput struct {
	RouteID    string
	PathIDs    []string
	RegionIDs  []string
}

// RouteRiskResult is the score for a single route.
type RouteRiskResult struct {
	RouteID         string   `json:"routeId"`
	RiskScore       int      `json:"riskScore"`
	ThreatenedPaths []string `json:"threatenedPaths"`
	BlockedPaths    []string `json:"blockedPaths"`
}

// RankedRouteList is the final output of Pipeline 1.
type RankedRouteList struct {
	Routes      []RouteRiskResult `json:"routes"`
	Recommended string            `json:"recommended"`
	Warnings    []string          `json:"warnings"`
}

// RouteRiskState holds the world state snapshot needed for risk calculation.
type RouteRiskState struct {
	Regions    map[string]RegionSnapshot
	Paths      map[string]PathSnapshot
	NazgulUnits []UnitSnapshot
	Graph      *game.GameGraph
}

// RegionSnapshot is a snapshot of a region's state.
type RegionSnapshot struct {
	ID          string
	Controller  string
	ThreatLevel int
	Fortified   bool
}

// PathSnapshot is a snapshot of a path's state.
type PathSnapshot struct {
	ID                string
	Status            string // OPEN, BLOCKED, THREATENED, TEMPORARILY_OPEN
	SurveillanceLevel int
}

// UnitSnapshot is a snapshot of a unit's state.
type UnitSnapshot struct {
	ID            string
	CurrentRegion string
	Strength      int
	Status        string
	Side          string
	Config        config.UnitConfig
}

// ComputeRouteRisk runs Pipeline 1 with fan-out to 4 workers.
func ComputeRouteRisk(ctx context.Context, routes []RouteRiskInput, state RouteRiskState) RankedRouteList {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	numWorkers := 4
	inputCh := make(chan RouteRiskInput, 20)     // buffered cap 20
	resultCh := make(chan RouteRiskResult)        // unbuffered

	// ── Fan-out: dispatch routes to workers ──
	go func() {
		defer close(inputCh)
		for _, route := range routes {
			select {
			case inputCh <- route:
			case <-ctx.Done():
				return
			}
		}
	}()

	// ── Workers ──
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for route := range inputCh {
				select {
				case <-ctx.Done():
					return
				default:
					result := computeSingleRouteRisk(route, state)
					select {
					case resultCh <- result:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// ── Close result channel when all workers done ──
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// ── Fan-in: aggregate results ──
	var results []RouteRiskResult
	for result := range resultCh {
		results = append(results, result)
	}

	// Sort by risk score (ascending = safest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].RiskScore < results[j].RiskScore
	})

	ranked := RankedRouteList{
		Routes: results,
	}
	if len(results) > 0 {
		ranked.Recommended = results[0].RouteID
	}

	// Add warnings for high-risk routes
	for _, r := range results {
		if len(r.BlockedPaths) > 0 {
			ranked.Warnings = append(ranked.Warnings, r.RouteID+": has blocked paths")
		}
	}

	return ranked
}

// computeSingleRouteRisk computes the risk score for one route.
//
// riskScore =
//     sum(region.threatLevel for each destination region)
//   + sum(path.surveillanceLevel for each path) * 3
//   + count(BLOCKED paths) * 5
//   + count(THREATENED paths) * 2
//   + nazgulProximityCount * 2
func computeSingleRouteRisk(route RouteRiskInput, state RouteRiskState) RouteRiskResult {
	result := RouteRiskResult{RouteID: route.RouteID}

	// Sum region threat levels
	for _, regionID := range route.RegionIDs {
		if r, ok := state.Regions[regionID]; ok {
			result.RiskScore += r.ThreatLevel
		}
	}

	// Sum path surveillance levels * 3
	for _, pathID := range route.PathIDs {
		if p, ok := state.Paths[pathID]; ok {
			result.RiskScore += p.SurveillanceLevel * 3

			if p.Status == "BLOCKED" {
				result.RiskScore += 5
				result.BlockedPaths = append(result.BlockedPaths, pathID)
			}
			if p.Status == "THREATENED" {
				result.RiskScore += 2
				result.ThreatenedPaths = append(result.ThreatenedPaths, pathID)
			}
		}
	}

	// Nazgul proximity: count Nazgul within 2 hops of any region in route
	if state.Graph != nil {
		proximityCount := 0
		for _, nazgul := range state.NazgulUnits {
			if nazgul.Status != "ACTIVE" || nazgul.CurrentRegion == "" {
				continue
			}
			for _, regionID := range route.RegionIDs {
				dist := state.Graph.Distance(nazgul.CurrentRegion, regionID)
				if dist >= 0 && dist <= 2 {
					proximityCount++
					break // count each Nazgul only once
				}
			}
		}
		result.RiskScore += proximityCount * 2
	}

	return result
}

// ═══════════════════════════════════════════════════════
// PIPELINE 2 — INTERCEPTION (Dark Side)
// 4 workers, buffer cap 30
// ═══════════════════════════════════════════════════════

// InterceptInput represents one (Nazgul, route) pair for scoring.
type InterceptInput struct {
	NazgulID     string
	NazgulRegion string
	RouteRegions []string
	RouteCosts   []int // cost to reach each region from start
}

// InterceptResult is the score for one Nazgul-route pair.
type InterceptResult struct {
	UnitID       string  `json:"unitId"`
	TargetRegion string  `json:"targetRegion"`
	Score        float64 `json:"score"`
}

// InterceptPlan is the final output of Pipeline 2.
type InterceptPlan struct {
	ByUnit []InterceptResult `json:"byUnit"`
}

// ComputeInterception runs Pipeline 2 with fan-out to 4 workers.
func ComputeInterception(ctx context.Context, inputs []InterceptInput, graph *game.GameGraph) InterceptPlan {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	numWorkers := 4
	inputCh := make(chan InterceptInput, 30)    // buffered cap 30
	resultCh := make(chan InterceptResult)       // unbuffered

	// ── Fan-out ──
	go func() {
		defer close(inputCh)
		for _, inp := range inputs {
			select {
			case inputCh <- inp:
			case <-ctx.Done():
				return
			}
		}
	}()

	// ── Workers ──
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for inp := range inputCh {
				select {
				case <-ctx.Done():
					return
				default:
					result := computeSingleIntercept(inp, graph)
					select {
					case resultCh <- result:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// ── Close results ──
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// ── Fan-in ──
	plan := InterceptPlan{}
	for result := range resultCh {
		if result.Score > 0 {
			plan.ByUnit = append(plan.ByUnit, result)
		}
	}

	// Sort by score descending (best intercept first)
	sort.Slice(plan.ByUnit, func(i, j int) bool {
		return plan.ByUnit[i].Score > plan.ByUnit[j].Score
	})

	return plan
}

// computeSingleIntercept scores one (Nazgul, route) pair.
//
//   turnsToIntercept = graph.shortestPath(nazgul.region, routeRegion) [in hops]
//   rbTurnsToReach   = sum of traversal costs to that region
//   interceptWindow  = rbTurnsToReach - turnsToIntercept
//   score = interceptWindow >= 0 ?
//           1.0 - (turnsToIntercept / routeLength) : 0.0
func computeSingleIntercept(input InterceptInput, graph *game.GameGraph) InterceptResult {
	bestScore := 0.0
	bestRegion := ""
	routeLength := len(input.RouteRegions)
	if routeLength == 0 {
		return InterceptResult{UnitID: input.NazgulID, Score: 0}
	}

	// Accumulate traversal cost
	rbTurnsToReach := 0
	for i, regionID := range input.RouteRegions {
		if i > 0 && i-1 < len(input.RouteCosts) {
			rbTurnsToReach += input.RouteCosts[i-1]
		}

		turnsToIntercept := graph.Distance(input.NazgulRegion, regionID)
		if turnsToIntercept < 0 {
			continue
		}

		interceptWindow := rbTurnsToReach - turnsToIntercept
		if interceptWindow >= 0 {
			score := 1.0 - (float64(turnsToIntercept) / float64(routeLength))
			if score > bestScore {
				bestScore = score
				bestRegion = regionID
			}
		}
	}

	return InterceptResult{
		UnitID:       input.NazgulID,
		TargetRegion: bestRegion,
		Score:        math.Round(bestScore*100) / 100,
	}
}
