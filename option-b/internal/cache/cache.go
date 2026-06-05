// Package cache implements WorldStateCache — a goroutine-safe cache
// that holds the current game state. The CacheManager goroutine is the
// sole owner; it sends value copies to workers, never pointers.
package cache

import (
	"encoding/json"
	"sync"

	"rotr/internal/config"
)

// ═══════════════════════════════════════════════════════
// WORLD STATE CACHE
// ═══════════════════════════════════════════════════════

// WorldStateCache holds the current game state.
type WorldStateCache struct {
	mu          sync.RWMutex
	Turn        int
	Units       map[string]UnitSnapshot
	Regions     map[string]RegionSnapshot
	Paths       map[string]PathSnapshot
	UnitConfigs map[string]config.UnitConfig // read-only after startup
	LightView   LightSideView
	DarkView    DarkSideView
	GameOver    bool
	Winner      string
	Cause       string
}

// UnitSnapshot is the current state of a unit.
type UnitSnapshot struct {
	ID              string   `json:"id"`
	CurrentRegion   string   `json:"currentRegion"`
	Strength        int      `json:"strength"`
	Status          string   `json:"status"` // ACTIVE | DESTROYED | RESPAWNING
	RespawnTurns    int      `json:"respawnTurns"`
	Cooldown        int      `json:"cooldown"`
	Route           []string `json:"route,omitempty"`
	RouteIdx        int      `json:"routeIdx"`
	TravelPathID    string   `json:"travelPathId,omitempty"`
	TravelFrom      string   `json:"travelFrom,omitempty"`
	TravelTo        string   `json:"travelTo,omitempty"`
	TravelRemaining int      `json:"travelRemaining,omitempty"`
}

// RegionSnapshot is the current state of a region.
type RegionSnapshot struct {
	ID           string `json:"id"`
	Controller   string `json:"controller"`
	ThreatLevel  int    `json:"threatLevel"`
	Fortified    bool   `json:"fortified"`
	FortifyTurns int    `json:"fortifyTurns"`
}

// PathSnapshot is the current state of a path.
type PathSnapshot struct {
	ID                string `json:"id"`
	From              string `json:"from,omitempty"`
	To                string `json:"to,omitempty"`
	Status            string `json:"status"` // OPEN | BLOCKED | THREATENED | TEMPORARILY_OPEN
	SurveillanceLevel int    `json:"surveillanceLevel"`
	TempOpenTurns     int    `json:"tempOpenTurns"`
	BlockedBy         string `json:"blockedBy,omitempty"`
	Corrupted         bool   `json:"corrupted,omitempty"`
}

// LightSideView holds Light Side-specific data.
type LightSideView struct {
	RingBearerRegion string   `json:"ringBearerRegion"`
	AssignedRoute    []string `json:"assignedRoute,omitempty"`
	RouteIdx         int      `json:"routeIdx"`
}

// DarkSideView holds Dark Side-specific data.
// CRITICAL: RingBearerRegion is ALWAYS "" — no code path ever sets this.
type DarkSideView struct {
	RingBearerRegion   string `json:"ringBearerRegion"` // ALWAYS ""
	LastDetectedRegion string `json:"lastDetectedRegion"`
	LastDetectedTurn   int    `json:"lastDetectedTurn"`
}

// ═══════════════════════════════════════════════════════
// CONSTRUCTOR
// ═══════════════════════════════════════════════════════

// NewWorldStateCache creates a new cache initialized from config.
func NewWorldStateCache(cfg *config.GameConfig) *WorldStateCache {
	c := &WorldStateCache{
		Turn:        1,
		Units:       make(map[string]UnitSnapshot, len(cfg.Units)),
		Regions:     make(map[string]RegionSnapshot, len(cfg.Regions)),
		Paths:       make(map[string]PathSnapshot, len(cfg.Paths)),
		UnitConfigs: cfg.UnitsByID,
	}

	// Initialize units from config
	for _, u := range cfg.Units {
		c.Units[u.ID] = UnitSnapshot{
			ID:            u.ID,
			CurrentRegion: u.StartRegion,
			Strength:      u.Strength,
			Status:        "ACTIVE",
		}
	}

	// Initialize regions from config
	for _, r := range cfg.Regions {
		c.Regions[r.ID] = RegionSnapshot{
			ID:          r.ID,
			Controller:  r.StartControl,
			ThreatLevel: r.StartThreat,
		}
	}

	// Initialize paths
	for _, p := range cfg.Paths {
		c.Paths[p.ID] = PathSnapshot{
			ID:     p.ID,
			From:   p.From,
			To:     p.To,
			Status: "OPEN",
		}
	}

	// Light view: Ring Bearer starts at the-shire
	for _, u := range cfg.Units {
		if u.Class == "RingBearer" {
			c.LightView.RingBearerRegion = u.StartRegion
			break
		}
	}

	// Dark view: ALWAYS empty
	c.DarkView.RingBearerRegion = "" // ENFORCED

	return c
}

func (c *WorldStateCache) ResetFromConfig(cfg *config.GameConfig) {
	// Reset swaps the cache back to a clean config-derived state. This is used
	// by /game/start and End Game so the browser can restart from Turn 1.
	fresh := NewWorldStateCache(cfg)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.Turn = fresh.Turn
	c.Units = fresh.Units
	c.Regions = fresh.Regions
	c.Paths = fresh.Paths
	c.UnitConfigs = fresh.UnitConfigs
	c.LightView = fresh.LightView
	c.DarkView = fresh.DarkView
	c.GameOver = false
	c.Winner = ""
	c.Cause = ""
}

// ═══════════════════════════════════════════════════════
// THREAD-SAFE ACCESS
// ═══════════════════════════════════════════════════════

// GetSnapshot returns a value copy of the entire game state.
func (c *WorldStateCache) GetSnapshot() WorldStateSnapshot {
	// Return value copies instead of internal maps. Callers can build analysis
	// inputs or HTTP responses without racing against cache updates.
	c.mu.RLock()
	defer c.mu.RUnlock()

	units := make([]UnitSnapshot, 0, len(c.Units))
	for _, u := range c.Units {
		units = append(units, u)
	}

	regions := make([]RegionSnapshot, 0, len(c.Regions))
	for _, r := range c.Regions {
		regions = append(regions, r)
	}

	paths := make([]PathSnapshot, 0, len(c.Paths))
	for _, p := range c.Paths {
		paths = append(paths, p)
	}

	return WorldStateSnapshot{
		Turn:     c.Turn,
		Units:    units,
		Regions:  regions,
		Paths:    paths,
		GameOver: c.GameOver,
		Winner:   c.Winner,
		Cause:    c.Cause,
	}
}

// WorldStateSnapshot is a serializable snapshot of the game state.
type WorldStateSnapshot struct {
	Turn     int              `json:"turn"`
	Units    []UnitSnapshot   `json:"units"`
	Regions  []RegionSnapshot `json:"regions"`
	Paths    []PathSnapshot   `json:"paths"`
	GameOver bool             `json:"gameOver,omitempty"`
	Winner   string           `json:"winner,omitempty"`
	Cause    string           `json:"cause,omitempty"`
}

// UpdateFromJSON updates the cache from a WorldStateSnapshot JSON.
func (c *WorldStateCache) UpdateFromJSON(data []byte) error {
	// Turn processing publishes WORLD_STATE snapshots as JSON. Applying those
	// snapshots here keeps the read-side HTTP cache in sync with the engine.
	c.mu.Lock()
	defer c.mu.Unlock()

	var snap WorldStateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}

	c.Turn = snap.Turn
	c.GameOver = snap.GameOver
	c.Winner = snap.Winner
	c.Cause = snap.Cause

	for _, u := range snap.Units {
		c.Units[u.ID] = u
	}
	for _, r := range snap.Regions {
		c.Regions[r.ID] = r
	}
	for _, p := range snap.Paths {
		c.Paths[p.ID] = p
	}

	// Update Light view — find Ring Bearer by config class
	for id, u := range c.Units {
		cfg, ok := c.UnitConfigs[id]
		if ok && cfg.Class == "RingBearer" {
			c.LightView.RingBearerRegion = u.CurrentRegion
		}
	}

	// ENFORCE: Dark view never gets Ring Bearer region
	c.DarkView.RingBearerRegion = ""

	return nil
}

// GetLightState returns a JSON state for the Light Side player.
func (c *WorldStateCache) GetLightState() []byte {
	// Light state is the full snapshot because Light is allowed to know the Ring
	// Bearer's true position.
	snap := c.GetSnapshot()
	data, _ := json.Marshal(snap)
	return data
}

// GetDarkState returns a JSON state for the Dark Side player
// with Ring Bearer position stripped.
func (c *WorldStateCache) GetDarkState() []byte {
	// Dark state starts from the same snapshot, then strips the Ring Bearer
	// position before serialization. This prevents API-level information leaks.
	snap := c.GetSnapshot()

	// Strip Ring Bearer position
	for i := range snap.Units {
		cfg, ok := c.UnitConfigs[snap.Units[i].ID]
		if ok && cfg.Class == "RingBearer" {
			snap.Units[i].CurrentRegion = "" // ALWAYS EMPTY
			snap.Units[i].TravelPathID = ""
			snap.Units[i].TravelFrom = ""
			snap.Units[i].TravelTo = ""
			snap.Units[i].TravelRemaining = 0
		}
	}

	data, _ := json.Marshal(snap)
	return data
}
