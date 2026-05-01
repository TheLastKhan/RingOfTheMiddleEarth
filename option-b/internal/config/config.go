// Package config loads unit and map configuration from JSON files.
// All game behaviour is config-driven — no unit ID string literals in game logic.
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// ═══════════════════════════════════════════════════════
// UNIT CONFIG
// ═══════════════════════════════════════════════════════

// UnitConfig holds all configuration for a single game unit.
// Behaviour is determined entirely by these fields — never by ID string matching.
type UnitConfig struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Class            string   `json:"class"`
	Side             string   `json:"side"`
	StartRegion      string   `json:"start"`
	Strength         int      `json:"strength"`
	Leadership       bool     `json:"leadership"`
	LeadershipBonus  int      `json:"leadershipBonus"`
	Indestructible   bool     `json:"indestructible"`
	DetectionRange   int      `json:"detectionRange"`
	Respawns         bool     `json:"respawns"`
	RespawnTurns     int      `json:"respawnTurns"`
	Maia             bool     `json:"maia"`
	MaiaAbilityPaths []string `json:"maiaAbilityPaths"`
	IgnoresFortress  bool     `json:"ignoresFortress"`
	CanFortify       bool     `json:"canFortify"`
	Cooldown         int      `json:"cooldown"`
}

// ═══════════════════════════════════════════════════════
// REGION CONFIG
// ═══════════════════════════════════════════════════════

// RegionConfig holds the fixed configuration for a map region.
type RegionConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Terrain      string `json:"terrain"`
	SpecialRole  string `json:"specialRole"`
	StartControl string `json:"startControl"`
	StartThreat  int    `json:"startThreat"`
}

// ═══════════════════════════════════════════════════════
// PATH CONFIG
// ═══════════════════════════════════════════════════════

// PathConfig holds the fixed configuration for a map path.
type PathConfig struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Cost int    `json:"cost"`
}

// ═══════════════════════════════════════════════════════
// CANONICAL ROUTE
// ═══════════════════════════════════════════════════════

// CanonicalRoute represents a predefined Ring Bearer route.
type CanonicalRoute struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Turns int      `json:"turns"`
	Path  []string `json:"path"`
}

// ═══════════════════════════════════════════════════════
// GAME CONFIG (TOP LEVEL)
// ═══════════════════════════════════════════════════════

// GameConfig is the top-level configuration structure.
type GameConfig struct {
	HiddenUntilTurn      int              `json:"hiddenUntilTurn"`
	MaxTurns             int              `json:"maxTurns"`
	TurnDurationSeconds  int              `json:"turnDurationSeconds"`
	Units                []UnitConfig     `json:"units"`
	Regions              []RegionConfig   `json:"regions"`
	Paths                []PathConfig     `json:"paths"`
	CanonicalRoutes      []CanonicalRoute `json:"canonicalRoutes"`

	// Indexed lookups — populated after loading
	UnitsByID   map[string]UnitConfig   `json:"-"`
	RegionsByID map[string]RegionConfig `json:"-"`
	PathsByID   map[string]PathConfig   `json:"-"`
}

// ═══════════════════════════════════════════════════════
// CONFIG FILE STRUCTURES (for JSON parsing)
// ═══════════════════════════════════════════════════════

type unitsFile struct {
	HiddenUntilTurn     int          `json:"hiddenUntilTurn"`
	MaxTurns            int          `json:"maxTurns"`
	TurnDurationSeconds int          `json:"turnDurationSeconds"`
	Units               []UnitConfig `json:"units"`
}

type mapFile struct {
	Regions         []RegionConfig   `json:"regions"`
	Paths           []PathConfig     `json:"paths"`
	CanonicalRoutes []CanonicalRoute `json:"canonicalRoutes"`
}

// ═══════════════════════════════════════════════════════
// LOAD FUNCTIONS
// ═══════════════════════════════════════════════════════

// LoadConfig loads the game configuration from unit and map JSON files.
func LoadConfig(unitsPath, mapPath string) (*GameConfig, error) {
	cfg := &GameConfig{}

	// Load units
	if err := loadUnitsJSON(unitsPath, cfg); err != nil {
		return nil, fmt.Errorf("loading units: %w", err)
	}

	// Load map
	if err := loadMapJSON(mapPath, cfg); err != nil {
		return nil, fmt.Errorf("loading map: %w", err)
	}

	// Build index maps
	cfg.UnitsByID = make(map[string]UnitConfig, len(cfg.Units))
	for _, u := range cfg.Units {
		cfg.UnitsByID[u.ID] = u
	}

	cfg.RegionsByID = make(map[string]RegionConfig, len(cfg.Regions))
	for _, r := range cfg.Regions {
		cfg.RegionsByID[r.ID] = r
	}

	cfg.PathsByID = make(map[string]PathConfig, len(cfg.Paths))
	for _, p := range cfg.Paths {
		cfg.PathsByID[p.ID] = p
	}

	return cfg, nil
}

func loadUnitsJSON(path string, cfg *GameConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var uf unitsFile
	if err := json.Unmarshal(data, &uf); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	cfg.HiddenUntilTurn = uf.HiddenUntilTurn
	cfg.MaxTurns = uf.MaxTurns
	cfg.TurnDurationSeconds = uf.TurnDurationSeconds
	cfg.Units = uf.Units
	return nil
}

func loadMapJSON(path string, cfg *GameConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var mf mapFile
	if err := json.Unmarshal(data, &mf); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	cfg.Regions = mf.Regions
	cfg.Paths = mf.Paths
	cfg.CanonicalRoutes = mf.CanonicalRoutes
	return nil
}

// ═══════════════════════════════════════════════════════
// DEFAULT CONFIG (embedded, no file required for tests)
// ═══════════════════════════════════════════════════════

// DefaultConfig returns a hardcoded GameConfig for unit testing
// without requiring external config files.
func DefaultConfig() *GameConfig {
	cfg := &GameConfig{
		HiddenUntilTurn:     3,
		MaxTurns:            40,
		TurnDurationSeconds: 60,
	}

	// Units — all 14
	cfg.Units = []UnitConfig{
		{ID: "ring-bearer", Name: "Frodo Baggins", Class: "RingBearer", Side: "FREE_PEOPLES", StartRegion: "the-shire", Strength: 1},
		{ID: "aragorn", Name: "Aragorn", Class: "FellowshipGuard", Side: "FREE_PEOPLES", StartRegion: "bree", Strength: 5, Leadership: true, LeadershipBonus: 1},
		{ID: "legolas", Name: "Legolas", Class: "FellowshipGuard", Side: "FREE_PEOPLES", StartRegion: "rivendell", Strength: 3},
		{ID: "gimli", Name: "Gimli", Class: "FellowshipGuard", Side: "FREE_PEOPLES", StartRegion: "rivendell", Strength: 3},
		{ID: "rohan-cavalry", Name: "Riders of Rohan", Class: "FellowshipGuard", Side: "FREE_PEOPLES", StartRegion: "edoras", Strength: 4},
		{ID: "gondor-army", Name: "Army of Gondor", Class: "GondorArmy", Side: "FREE_PEOPLES", StartRegion: "minas-tirith", Strength: 5, CanFortify: true},
		{ID: "gandalf", Name: "Gandalf", Class: "Maia", Side: "FREE_PEOPLES", StartRegion: "rivendell", Strength: 4, Maia: true, Cooldown: 3},
		{ID: "witch-king", Name: "Witch-King", Class: "Nazgul", Side: "SHADOW", StartRegion: "minas-morgul", Strength: 5, Leadership: true, LeadershipBonus: 1, Indestructible: true, DetectionRange: 2},
		{ID: "nazgul-2", Name: "Dark Marshal", Class: "Nazgul", Side: "SHADOW", StartRegion: "minas-morgul", Strength: 3, DetectionRange: 1, Respawns: true, RespawnTurns: 3},
		{ID: "nazgul-3", Name: "The Betrayer", Class: "Nazgul", Side: "SHADOW", StartRegion: "minas-morgul", Strength: 3, DetectionRange: 1, Respawns: true, RespawnTurns: 3},
		{ID: "uruk-hai-legion", Name: "Uruk-hai Legion", Class: "UrukHaiLegion", Side: "SHADOW", StartRegion: "isengard", Strength: 5, IgnoresFortress: true},
		{ID: "saruman", Name: "Saruman", Class: "Maia", Side: "SHADOW", StartRegion: "isengard", Strength: 4, Maia: true, MaiaAbilityPaths: []string{"fangorn-to-isengard", "helms-deep-to-isengard", "fords-of-isen-to-isengard", "tharbad-to-fords-of-isen", "fords-of-isen-to-edoras"}, Cooldown: 2},
		{ID: "sauron", Name: "Sauron", Class: "Maia", Side: "SHADOW", StartRegion: "mordor", Strength: 5, Indestructible: true, Maia: true},
	}

	// Regions — all 22
	cfg.Regions = []RegionConfig{
		{ID: "the-shire", Name: "The Shire", Terrain: "PLAINS", SpecialRole: "RING_BEARER_START", StartControl: "FREE_PEOPLES", StartThreat: 0},
		{ID: "bree", Name: "Bree", Terrain: "PLAINS", StartControl: "NEUTRAL", StartThreat: 1},
		{ID: "tharbad", Name: "Tharbad", Terrain: "SWAMP", StartControl: "NEUTRAL", StartThreat: 2},
		{ID: "weathertop", Name: "Weathertop", Terrain: "MOUNTAINS", StartControl: "NEUTRAL", StartThreat: 2},
		{ID: "rivendell", Name: "Rivendell", Terrain: "MOUNTAINS", StartControl: "FREE_PEOPLES"},
		{ID: "fangorn", Name: "Fangorn", Terrain: "FOREST", StartControl: "FREE_PEOPLES"},
		{ID: "fords-of-isen", Name: "Fords of Isen", Terrain: "PLAINS", StartControl: "NEUTRAL", StartThreat: 2},
		{ID: "rohan-plains", Name: "Rohan Plains", Terrain: "PLAINS", StartControl: "FREE_PEOPLES", StartThreat: 1},
		{ID: "moria", Name: "Moria", Terrain: "MOUNTAINS", StartControl: "NEUTRAL", StartThreat: 3},
		{ID: "helms-deep", Name: "Helm's Deep", Terrain: "FORTRESS", StartControl: "FREE_PEOPLES", StartThreat: 1},
		{ID: "isengard", Name: "Isengard", Terrain: "FORTRESS", SpecialRole: "SHADOW_STRONGHOLD", StartControl: "SHADOW", StartThreat: 3},
		{ID: "edoras", Name: "Edoras", Terrain: "PLAINS", StartControl: "FREE_PEOPLES", StartThreat: 1},
		{ID: "lothlorien", Name: "Lothlórien", Terrain: "FOREST", StartControl: "FREE_PEOPLES"},
		{ID: "dead-marshes", Name: "Dead Marshes", Terrain: "SWAMP", StartControl: "NEUTRAL", StartThreat: 2},
		{ID: "emyn-muil", Name: "Emyn Muil", Terrain: "MOUNTAINS", StartControl: "NEUTRAL", StartThreat: 2},
		{ID: "minas-tirith", Name: "Minas Tirith", Terrain: "FORTRESS", StartControl: "FREE_PEOPLES", StartThreat: 1},
		{ID: "ithilien", Name: "Ithilien", Terrain: "FOREST", StartControl: "NEUTRAL", StartThreat: 2},
		{ID: "osgiliath", Name: "Osgiliath", Terrain: "PLAINS", StartControl: "NEUTRAL", StartThreat: 3},
		{ID: "minas-morgul", Name: "Minas Morgul", Terrain: "FORTRESS", SpecialRole: "SHADOW_STRONGHOLD", StartControl: "SHADOW", StartThreat: 4},
		{ID: "cirith-ungol", Name: "Cirith Ungol", Terrain: "MOUNTAINS", StartControl: "SHADOW", StartThreat: 4},
		{ID: "mordor", Name: "Mordor", Terrain: "VOLCANIC", SpecialRole: "SHADOW_STRONGHOLD", StartControl: "SHADOW", StartThreat: 5},
		{ID: "mount-doom", Name: "Mount Doom", Terrain: "VOLCANIC", SpecialRole: "RING_DESTRUCTION_SITE", StartControl: "SHADOW", StartThreat: 5},
	}

	// Paths — all 37
	cfg.Paths = []PathConfig{
		{ID: "shire-to-bree", From: "the-shire", To: "bree", Cost: 1},
		{ID: "bree-to-weathertop", From: "bree", To: "weathertop", Cost: 1},
		{ID: "bree-to-rivendell", From: "bree", To: "rivendell", Cost: 2},
		{ID: "bree-to-tharbad", From: "bree", To: "tharbad", Cost: 1},
		{ID: "shire-to-tharbad", From: "the-shire", To: "tharbad", Cost: 2},
		{ID: "weathertop-to-rivendell", From: "weathertop", To: "rivendell", Cost: 1},
		{ID: "rivendell-to-moria", From: "rivendell", To: "moria", Cost: 2},
		{ID: "rivendell-to-lothlorien", From: "rivendell", To: "lothlorien", Cost: 2},
		{ID: "moria-to-lothlorien", From: "moria", To: "lothlorien", Cost: 1},
		{ID: "lothlorien-to-emyn-muil", From: "lothlorien", To: "emyn-muil", Cost: 1},
		{ID: "lothlorien-to-rohan-plains", From: "lothlorien", To: "rohan-plains", Cost: 1},
		{ID: "rohan-plains-to-fangorn", From: "rohan-plains", To: "fangorn", Cost: 1},
		{ID: "rohan-plains-to-edoras", From: "rohan-plains", To: "edoras", Cost: 1},
		{ID: "rohan-plains-to-minas-tirith", From: "rohan-plains", To: "minas-tirith", Cost: 2},
		{ID: "fangorn-to-isengard", From: "fangorn", To: "isengard", Cost: 1},
		{ID: "isengard-to-rohan-plains", From: "isengard", To: "rohan-plains", Cost: 1},
		{ID: "tharbad-to-fords-of-isen", From: "tharbad", To: "fords-of-isen", Cost: 2},
		{ID: "fords-of-isen-to-isengard", From: "fords-of-isen", To: "isengard", Cost: 1},
		{ID: "fords-of-isen-to-helms-deep", From: "fords-of-isen", To: "helms-deep", Cost: 1},
		{ID: "fords-of-isen-to-edoras", From: "fords-of-isen", To: "edoras", Cost: 1},
		{ID: "edoras-to-helms-deep", From: "edoras", To: "helms-deep", Cost: 1},
		{ID: "helms-deep-to-isengard", From: "helms-deep", To: "isengard", Cost: 1},
		{ID: "edoras-to-minas-tirith", From: "edoras", To: "minas-tirith", Cost: 2},
		{ID: "emyn-muil-to-dead-marshes", From: "emyn-muil", To: "dead-marshes", Cost: 1},
		{ID: "emyn-muil-to-ithilien", From: "emyn-muil", To: "ithilien", Cost: 2},
		{ID: "dead-marshes-to-ithilien", From: "dead-marshes", To: "ithilien", Cost: 1},
		{ID: "dead-marshes-to-mordor", From: "dead-marshes", To: "mordor", Cost: 2},
		{ID: "ithilien-to-minas-tirith", From: "ithilien", To: "minas-tirith", Cost: 1},
		{ID: "ithilien-to-osgiliath", From: "ithilien", To: "osgiliath", Cost: 1},
		{ID: "ithilien-to-cirith-ungol", From: "ithilien", To: "cirith-ungol", Cost: 2},
		{ID: "minas-tirith-to-osgiliath", From: "minas-tirith", To: "osgiliath", Cost: 1},
		{ID: "osgiliath-to-minas-morgul", From: "osgiliath", To: "minas-morgul", Cost: 1},
		{ID: "minas-morgul-to-cirith-ungol", From: "minas-morgul", To: "cirith-ungol", Cost: 1},
		{ID: "minas-morgul-to-mordor", From: "minas-morgul", To: "mordor", Cost: 1},
		{ID: "cirith-ungol-to-mordor", From: "cirith-ungol", To: "mordor", Cost: 1},
		{ID: "cirith-ungol-to-mount-doom", From: "cirith-ungol", To: "mount-doom", Cost: 2},
		{ID: "mordor-to-mount-doom", From: "mordor", To: "mount-doom", Cost: 1},
	}

	// Build indexes
	cfg.UnitsByID = make(map[string]UnitConfig, len(cfg.Units))
	for _, u := range cfg.Units {
		cfg.UnitsByID[u.ID] = u
	}
	cfg.RegionsByID = make(map[string]RegionConfig, len(cfg.Regions))
	for _, r := range cfg.Regions {
		cfg.RegionsByID[r.ID] = r
	}
	cfg.PathsByID = make(map[string]PathConfig, len(cfg.Paths))
	for _, p := range cfg.Paths {
		cfg.PathsByID[p.ID] = p
	}

	return cfg
}
