// Package game implements the game graph, combat engine, detection, and turn processing.
// The graph supports BFS shortest-path queries for detection range and route risk.
package game

import (
	"rotr/internal/config"
)

// ═══════════════════════════════════════════════════════
// GAME GRAPH — 22 regions, 37 bidirectional paths
// ═══════════════════════════════════════════════════════

// GameGraph represents the Middle Earth map as an adjacency list.
type GameGraph struct {
	// adjacency[regionA] = [{neighborRegion, pathID, cost}, ...]
	adjacency map[string][]Edge
	cfg       *config.GameConfig
}

// Edge represents a single path connection between two regions.
type Edge struct {
	To     string // destination region ID
	PathID string // path ID
	Cost   int    // traversal cost in turns
}

// NewGameGraph builds the graph from configuration.
func NewGameGraph(cfg *config.GameConfig) *GameGraph {
	g := &GameGraph{
		adjacency: make(map[string][]Edge),
		cfg:       cfg,
	}

	// All paths are bidirectional
	for _, p := range cfg.Paths {
		g.adjacency[p.From] = append(g.adjacency[p.From], Edge{To: p.To, PathID: p.ID, Cost: p.Cost})
		g.adjacency[p.To] = append(g.adjacency[p.To], Edge{To: p.From, PathID: p.ID, Cost: p.Cost})
	}

	return g
}

// Neighbors returns all adjacent regions from a given region.
func (g *GameGraph) Neighbors(regionID string) []Edge {
	return g.adjacency[regionID]
}

// Distance computes the shortest hop distance between two regions using BFS.
// Returns -1 if no path exists (should never happen on a connected graph).
func (g *GameGraph) Distance(from, to string) int {
	if from == to {
		return 0
	}

	visited := make(map[string]bool)
	type queueItem struct {
		region string
		dist   int
	}

	queue := []queueItem{{from, 0}}
	visited[from] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range g.adjacency[current.region] {
			if edge.To == to {
				return current.dist + 1
			}
			if !visited[edge.To] {
				visited[edge.To] = true
				queue = append(queue, queueItem{edge.To, current.dist + 1})
			}
		}
	}

	return -1 // unreachable
}

// ShortestPath returns the shortest path (in hops) between two regions.
// Returns nil if no path exists.
func (g *GameGraph) ShortestPath(from, to string) []string {
	if from == to {
		return []string{from}
	}

	visited := make(map[string]bool)
	parent := make(map[string]string)

	type queueItem struct {
		region string
	}

	queue := []queueItem{{from}}
	visited[from] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range g.adjacency[current.region] {
			if !visited[edge.To] {
				visited[edge.To] = true
				parent[edge.To] = current.region
				if edge.To == to {
					// Reconstruct path
					path := []string{to}
					cur := to
					for cur != from {
						cur = parent[cur]
						path = append([]string{cur}, path...)
					}
					return path
				}
				queue = append(queue, queueItem{edge.To})
			}
		}
	}

	return nil
}

// PathEndpoints returns the two endpoint regions of a path.
func (g *GameGraph) PathEndpoints(pathID string) (string, string) {
	p, ok := g.cfg.PathsByID[pathID]
	if !ok {
		return "", ""
	}
	return p.From, p.To
}

// IsEndpointOf checks if a region is an endpoint of a given path.
func (g *GameGraph) IsEndpointOf(regionID, pathID string) bool {
	from, to := g.PathEndpoints(pathID)
	return regionID == from || regionID == to
}

// RegionsWithinHops returns all regions within N hops of a source region.
func (g *GameGraph) RegionsWithinHops(source string, maxHops int) []string {
	if maxHops <= 0 {
		return []string{source}
	}

	visited := map[string]bool{source: true}
	result := []string{source}

	type queueItem struct {
		region string
		dist   int
	}
	queue := []queueItem{{source, 0}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.dist >= maxHops {
			continue
		}

		for _, edge := range g.adjacency[current.region] {
			if !visited[edge.To] {
				visited[edge.To] = true
				result = append(result, edge.To)
				queue = append(queue, queueItem{edge.To, current.dist + 1})
			}
		}
	}

	return result
}
