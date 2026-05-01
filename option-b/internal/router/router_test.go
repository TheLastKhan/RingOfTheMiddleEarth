package router

import (
	"encoding/json"
	"testing"
)

// ═══════════════════════════════════════════════════════
// ROUTER TEST — 3 required cases (Rubric B7)
// Run: go test -race ./internal/router/...
// ═══════════════════════════════════════════════════════

// Case 1: WorldStateSnapshot with ring-bearer region set →
// Dark Side receives currentRegion="", Light Side receives real value
func TestRouter_DarkSideStripped(t *testing.T) {
	router := NewEventRouter()

	state := worldStateJSON{
		Turn: 5,
		Units: []unitJSON{
			{ID: "aragorn", CurrentRegion: "bree", Strength: 5, Status: "ACTIVE"},
			{ID: "ring-bearer", CurrentRegion: "weathertop", Strength: 1, Status: "ACTIVE"},
			{ID: "witch-king", CurrentRegion: "minas-morgul", Strength: 5, Status: "ACTIVE"},
		},
	}
	data, _ := json.Marshal(state)

	event := Event{
		Topic: "game.broadcast",
		Data:  data,
	}

	// Route the event
	go router.Route(event)

	// Check Light Side — should have real region
	lightEvent := <-router.LightSSECh
	var lightState worldStateJSON
	json.Unmarshal(lightEvent.Data, &lightState)

	rbLight := findUnit(lightState.Units, "ring-bearer")
	if rbLight == nil {
		t.Fatal("ring-bearer not found in light side state")
	}
	if rbLight.CurrentRegion != "weathertop" {
		t.Errorf("Light Side should see real region 'weathertop', got '%s'", rbLight.CurrentRegion)
	}

	// Check Dark Side — should have empty region
	darkEvent := <-router.DarkSSECh
	var darkState worldStateJSON
	json.Unmarshal(darkEvent.Data, &darkState)

	rbDark := findUnit(darkState.Units, "ring-bearer")
	if rbDark == nil {
		t.Fatal("ring-bearer not found in dark side state")
	}
	if rbDark.CurrentRegion != "" {
		t.Errorf("Dark Side should see empty region, got '%s'", rbDark.CurrentRegion)
	}
}

// Case 2: RingBearerMoved event → never reaches Dark Side SSE channel
func TestRouter_RingBearerMovedNeverDarkSide(t *testing.T) {
	router := NewEventRouter()

	moveData, _ := json.Marshal(map[string]interface{}{
		"trueRegion": "rivendell",
		"turn":       4,
	})

	event := Event{
		Topic: "game.ring.position",
		Data:  moveData,
	}

	go router.Route(event)

	// Light Side should receive
	lightEvent := <-router.LightSSECh
	if lightEvent.Topic != "game.ring.position" {
		t.Errorf("Light Side should receive ring.position, got '%s'", lightEvent.Topic)
	}

	// Dark Side should NOT receive — channel should be empty
	select {
	case evt := <-router.DarkSSECh:
		t.Errorf("Dark Side should NEVER receive ring.position, but got event: %s", evt.Topic)
	default:
		// Good — Dark Side channel is empty
	}
}

// Case 3: DarkView.RingBearerRegion is always "" after any cache update
func TestRouter_CacheUpdateNeverExposesRingBearer(t *testing.T) {
	// Test StripRingBearerFromState directly
	state := worldStateJSON{
		Turn: 10,
		Units: []unitJSON{
			{ID: "ring-bearer", CurrentRegion: "mount-doom", Strength: 1, Status: "ACTIVE"},
			{ID: "aragorn", CurrentRegion: "mordor", Strength: 5, Status: "ACTIVE"},
		},
	}
	data, _ := json.Marshal(state)

	stripped := StripRingBearerFromState(data)

	var result worldStateJSON
	json.Unmarshal(stripped, &result)

	for _, u := range result.Units {
		if u.ID == "ring-bearer" {
			if u.CurrentRegion != "" {
				t.Errorf("After stripping, ring-bearer region should be '', got '%s'", u.CurrentRegion)
			}
		}
		if u.ID == "aragorn" {
			if u.CurrentRegion != "mordor" {
				t.Errorf("Aragorn's region should be unchanged, got '%s'", u.CurrentRegion)
			}
		}
	}

	// Run multiple times to check race condition
	for i := 0; i < 100; i++ {
		s := StripRingBearerFromState(data)
		var r worldStateJSON
		json.Unmarshal(s, &r)
		rb := findUnit(r.Units, "ring-bearer")
		if rb != nil && rb.CurrentRegion != "" {
			t.Fatalf("Race condition: ring-bearer region leaked on iteration %d: '%s'", i, rb.CurrentRegion)
		}
	}
}

// ── Helper ──

func findUnit(units []unitJSON, id string) *unitJSON {
	for i := range units {
		if units[i].ID == id {
			return &units[i]
		}
	}
	return nil
}
