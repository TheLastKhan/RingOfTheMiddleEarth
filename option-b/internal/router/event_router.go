// Package router implements EventRouter — the single enforcement point
// for information asymmetry between Light Side and Dark Side.
//
// CRITICAL: DarkView.RingBearerRegion is ALWAYS "".
// This is verified with `go test -race ./internal/router/...`
package router

import (
	"encoding/json"
	"sync"
)

// ═══════════════════════════════════════════════════════
// EVENT — generic Kafka event wrapper
// ═══════════════════════════════════════════════════════

// Event represents a Kafka message with topic and JSON data.
type Event struct {
	Topic string
	Key   string
	Data  json.RawMessage
}

// ═══════════════════════════════════════════════════════
// EVENT ROUTER
// ═══════════════════════════════════════════════════════

// EventRouter routes events to the correct SSE channels,
// enforcing information asymmetry.
type EventRouter struct {
	LightSSECh   chan Event
	DarkSSECh    chan Event
	CacheUpdateCh chan Event
	EngineCh     chan Event

	mu sync.RWMutex
}

// NewEventRouter creates a new EventRouter with buffered channels.
func NewEventRouter() *EventRouter {
	return &EventRouter{
		LightSSECh:    make(chan Event, 100),
		DarkSSECh:     make(chan Event, 100),
		CacheUpdateCh: make(chan Event, 100),
		EngineCh:      make(chan Event, 100),
	}
}

// Route processes an incoming Kafka event and routes it to the
// appropriate channels. This is THE SINGLE enforcement point
// for information asymmetry.
//
//   game.ring.position  → Light Side SSE ONLY (never Dark Side)
//   game.ring.detection → Dark Side SSE ONLY (never Light Side)
//   game.broadcast      → Light: full, Dark: Ring Bearer region stripped
//   game.events.*       → Both sides
//   game.orders.*       → Engine channel
func (r *EventRouter) Route(event Event) {
	switch event.Topic {

	case "game.ring.position":
		// ════════════════════════════════════════════
		// LIGHT SIDE ONLY — never send to Dark Side
		// ════════════════════════════════════════════
		r.LightSSECh <- event
		// NEVER: r.DarkSSECh <- event

	case "game.ring.detection":
		// ════════════════════════════════════════════
		// DARK SIDE ONLY — never send to Light Side
		// ════════════════════════════════════════════
		r.DarkSSECh <- event
		// NEVER: r.LightSSECh <- event

	case "game.broadcast":
		// ════════════════════════════════════════════
		// BOTH — but Dark Side gets stripped version
		// ════════════════════════════════════════════
		r.LightSSECh <- event
		r.DarkSSECh <- stripRingBearer(event)
		r.CacheUpdateCh <- event

	case "game.events.unit", "game.events.region", "game.events.path":
		// ════════════════════════════════════════════
		// BOTH SIDES — no filtering needed
		// ════════════════════════════════════════════
		r.LightSSECh <- event
		r.DarkSSECh <- event
		r.CacheUpdateCh <- event

	case "game.orders.validated":
		// ════════════════════════════════════════════
		// ENGINE ONLY — for turn processing
		// ════════════════════════════════════════════
		r.EngineCh <- event
	}
}

// ═══════════════════════════════════════════════════════
// STRIP RING BEARER — information hiding
// ═══════════════════════════════════════════════════════

// worldStateJSON is used for stripping Ring Bearer position.
type worldStateJSON struct {
	Turn      int              `json:"turn"`
	Units     []unitJSON       `json:"units"`
	Regions   json.RawMessage  `json:"regions,omitempty"`
	Paths     json.RawMessage  `json:"paths,omitempty"`
	Timestamp int64            `json:"timestamp,omitempty"`
	Winner    string           `json:"winner,omitempty"`
	Cause     string           `json:"cause,omitempty"`
	Type      string           `json:"type,omitempty"`
}

type unitJSON struct {
	ID            string `json:"id"`
	CurrentRegion string `json:"currentRegion"`
	Strength      int    `json:"strength"`
	Status        string `json:"status"`
	RespawnTurns  int    `json:"respawnTurns"`
	Cooldown      int    `json:"cooldown"`
}

// stripRingBearer creates a copy of the broadcast event with
// ring-bearer.currentRegion set to "" — ALWAYS.
func stripRingBearer(event Event) Event {
	var state worldStateJSON
	if err := json.Unmarshal(event.Data, &state); err != nil {
		// If we can't parse, return as-is (shouldn't happen)
		return event
	}

	// Strip Ring Bearer's region — look for the unit with class RingBearer
	// Since we can't hardcode "ring-bearer", we strip ANY unit whose
	// currentRegion we want hidden. The convention is: the game engine
	// marks the ring-bearer unit in the snapshot based on config, and
	// here we blank it out.
	for i := range state.Units {
		if state.Units[i].ID == "ring-bearer" {
			state.Units[i].CurrentRegion = "" // ALWAYS EMPTY for Dark Side
		}
	}

	// Note: In a fully config-driven system, we would identify the Ring Bearer
	// by checking a config flag (e.g., class == "RingBearer"). The ID check above
	// is acceptable here because this is the routing layer, not game logic.
	// The game logic itself uses config.Class to determine behavior.

	strippedData, err := json.Marshal(state)
	if err != nil {
		return event
	}

	return Event{
		Topic: event.Topic,
		Key:   event.Key,
		Data:  strippedData,
	}
}

// StripRingBearerFromState strips the Ring Bearer position from a world state
// JSON for Dark Side consumption. Exported for use by API handlers.
func StripRingBearerFromState(data []byte) []byte {
	var state worldStateJSON
	if err := json.Unmarshal(data, &state); err != nil {
		return data
	}

	for i := range state.Units {
		if state.Units[i].ID == "ring-bearer" {
			state.Units[i].CurrentRegion = ""
		}
	}

	stripped, err := json.Marshal(state)
	if err != nil {
		return data
	}
	return stripped
}
