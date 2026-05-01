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
}

// UnitSnapshot is the current state of a unit.
type UnitSnapshot struct {
	ID            string `json:"id"`
	CurrentRegion string `json:"currentRegion"`
	Strength      int    `json:"strength"`
	Status        string `json:"status"` // ACTIVE | DESTROYED | RESPAWNING
	RespawnTurns  int    `json:"respawnTurns"`
	Cooldown      int    `json:"cooldown"`
	Route         []string `json:"route,omitempty"`
	RouteIdx      int    `json:"routeIdx"`
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
	Status            string `json:"status"` // OPEN | BLOCKED | THREATENED | TEMPORARILY_OPEN
	SurveillanceLevel int    `json:"surveillanceLevel"`
	TempOpenTurns     int    `json:"tempOpenTurns"`
	BlockedBy         string `json:"blockedBy,omitempty"`
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
	RingBearerRegion   string `json:"ringBearerRegion"`   // ALWAYS ""
	LastDetectedRegion string `json:"lastDetectedRegion"`
	LastDetectedTurn   int    `json:"lastDetectedTurn"`
}

// ═══════════════════════════════════════════════════════
// CONSTRUCTOR
// ═══════════════════════════════════════════════════════

// NewWorldStateCache creates a new cache initialized from config.
func NewWorldStateCache(cfg *config.GameConfig) *WorldStateCache {
	c := &WorldStateCache{
		Turn:        0,
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

// ═══════════════════════════════════════════════════════
// THREAD-SAFE ACCESS
// ═══════════════════════════════════════════════════════

// GetSnapshot returns a value copy of the entire game state.
func (c *WorldStateCache) GetSnapshot() WorldStateSnapshot {
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
		Turn:    c.Turn,
		Units:   units,
		Regions: regions,
		Paths:   paths,
	}
}

// WorldStateSnapshot is a serializable snapshot of the game state.
type WorldStateSnapshot struct {
	Turn    int              `json:"turn"`
	Units   []UnitSnapshot   `json:"units"`
	Regions []RegionSnapshot `json:"regions"`
	Paths   []PathSnapshot   `json:"paths"`
}

// UpdateFromJSON updates the cache from a WorldStateSnapshot JSON.
func (c *WorldStateCache) UpdateFromJSON(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var snap WorldStateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}

	c.Turn = snap.Turn

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
	snap := c.GetSnapshot()
	data, _ := json.Marshal(snap)
	return data
}

// GetDarkState returns a JSON state for the Dark Side player
// with Ring Bearer position stripped.
func (c *WorldStateCache) GetDarkState() []byte {
	snap := c.GetSnapshot()

	// Strip Ring Bearer position
	for i := range snap.Units {
		cfg, ok := c.UnitConfigs[snap.Units[i].ID]
		if ok && cfg.Class == "RingBearer" {
			snap.Units[i].CurrentRegion = "" // ALWAYS EMPTY
		}
	}

	data, _ := json.Marshal(snap)
	return data
}
