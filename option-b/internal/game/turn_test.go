package game

import (
	"strings"
	"testing"

	"rotr/internal/config"
)

func TestProcessTurnEmitsGameOverOnce(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MaxTurns = 1
	graph := NewGameGraph(cfg)
	processor := NewTurnProcessor(cfg, graph)
	state := InitTurnState(cfg, graph)

	first := processor.ProcessTurn(state, nil)
	if countGameOver(first) != 1 {
		t.Fatalf("first ProcessTurn emitted %d GAME_OVER events, want 1", countGameOver(first))
	}
	second := processor.ProcessTurn(state, nil)
	if countGameOver(second) != 0 {
		t.Fatalf("second ProcessTurn emitted %d GAME_OVER events, want 0", countGameOver(second))
	}
	if len(second) != 0 {
		t.Fatalf("second ProcessTurn emitted %d events after game over, want 0", len(second))
	}
}

func TestGameOverEventHasDeterministicIdentity(t *testing.T) {
	cfg := config.DefaultConfig()
	graph := NewGameGraph(cfg)
	processor := NewTurnProcessor(cfg, graph)
	state := InitTurnState(cfg, graph)
	state.Units["ring-bearer"].Status = "DESTROYED"

	events := processor.ProcessTurn(state, nil)
	if countGameOver(events) != 1 {
		t.Fatalf("ProcessTurn emitted %d GAME_OVER events, want 1", countGameOver(events))
	}
	if events[0].Key != "game-over" {
		t.Fatalf("GAME_OVER key = %q, want game-over", events[0].Key)
	}
	if !strings.Contains(string(events[0].Data), `"eventId":"game-over-SHADOW-1"`) {
		t.Fatalf("GAME_OVER event missing deterministic eventId: %s", string(events[0].Data))
	}
}

func TestMaxTurnsEmitsDraw(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MaxTurns = 1
	graph := NewGameGraph(cfg)
	processor := NewTurnProcessor(cfg, graph)
	state := InitTurnState(cfg, graph)

	events := processor.ProcessTurn(state, nil)
	if countGameOverByWinner(events, "DRAW") != 1 {
		t.Fatalf("max turn emitted events %v, want DRAW game over", gameOverPayloads(events))
	}
}

func TestDestroyRingRequiresOrderAndNoShadowAtMountDoom(t *testing.T) {
	cfg := config.DefaultConfig()
	graph := NewGameGraph(cfg)
	processor := NewTurnProcessor(cfg, graph)
	state := InitTurnState(cfg, graph)
	state.Units["ring-bearer"].CurrentRegion = "mount-doom"

	withoutOrder := processor.ProcessTurn(state, nil)
	if countGameOver(withoutOrder) != 0 {
		t.Fatalf("Ring Bearer at Mount Doom without DESTROY_RING emitted GAME_OVER")
	}

	state.Turn = 1
	state.Units["witch-king"].CurrentRegion = "mount-doom"
	withShadow := processor.ProcessTurn(state, []Order{{OrderType: "DESTROY_RING", UnitID: "ring-bearer"}})
	if countGameOverByWinner(withShadow, "FREE_PEOPLES") != 0 {
		t.Fatalf("DESTROY_RING with Shadow unit at Mount Doom emitted FREE_PEOPLES GAME_OVER")
	}

	successState := InitTurnState(cfg, graph)
	successState.Units["ring-bearer"].CurrentRegion = "mount-doom"
	success := processor.ProcessTurn(successState, []Order{{OrderType: "DESTROY_RING", UnitID: "ring-bearer"}})
	if countGameOver(success) != 1 {
		t.Fatalf("valid DESTROY_RING emitted %d GAME_OVER events, want 1", countGameOver(success))
	}
}

func TestRingBearerSpottedBySurveillanceAfterHiddenTurns(t *testing.T) {
	cfg := config.DefaultConfig()
	graph := NewGameGraph(cfg)
	processor := NewTurnProcessor(cfg, graph)
	state := InitTurnState(cfg, graph)
	state.Turn = cfg.HiddenUntilTurn
	state.Paths["shire-to-bree"].SurveillanceLevel = 1

	hiddenEvents := processor.ProcessTurn(state, []Order{{
		OrderType: "ASSIGN_ROUTE",
		UnitID:    "ring-bearer",
		PathIDs:   []string{"shire-to-bree"},
	}})
	if countRingDetection(hiddenEvents) != 0 {
		t.Fatalf("hidden turn emitted ring detection event")
	}

	state = InitTurnState(cfg, graph)
	state.Turn = cfg.HiddenUntilTurn + 1
	state.Paths["shire-to-bree"].SurveillanceLevel = 1
	visibleEvents := processor.ProcessTurn(state, []Order{{
		OrderType: "ASSIGN_ROUTE",
		UnitID:    "ring-bearer",
		PathIDs:   []string{"shire-to-bree"},
	}})
	if countRingDetection(visibleEvents) != 1 {
		t.Fatalf("surveilled path emitted %d ring detection events, want 1", countRingDetection(visibleEvents))
	}
	if !state.Exposed {
		t.Fatalf("surveilled path did not mark Ring Bearer exposed")
	}
}

func TestExposedRingBearerWithShadowUnitWinsForShadow(t *testing.T) {
	cfg := config.DefaultConfig()
	graph := NewGameGraph(cfg)
	processor := NewTurnProcessor(cfg, graph)
	state := InitTurnState(cfg, graph)
	state.Units["ring-bearer"].CurrentRegion = "bree"
	state.Units["witch-king"].CurrentRegion = "bree"
	state.Exposed = true

	events := processor.step13CheckWinConditions(state)
	if countGameOverByWinner(events, "SHADOW") != 1 {
		t.Fatalf("exposed intercepted Ring Bearer emitted events %v, want SHADOW game over", gameOverPayloads(events))
	}
}

func TestBlockedPathReopensWhenBlockerLeavesEndpoint(t *testing.T) {
	cfg := config.DefaultConfig()
	graph := NewGameGraph(cfg)
	processor := NewTurnProcessor(cfg, graph)
	state := InitTurnState(cfg, graph)
	pathID := "minas-morgul-to-cirith-ungol"
	state.Paths[pathID].Status = "BLOCKED"
	state.Paths[pathID].BlockedBy = "witch-king"
	state.Units["witch-king"].CurrentRegion = "mordor"

	processor.ProcessTurn(state, nil)
	if state.Paths[pathID].Status != "OPEN" {
		t.Fatalf("path stayed %s after blocker left endpoint, want OPEN", state.Paths[pathID].Status)
	}
}

func TestGandalfTemporarilyOpensBlockedPath(t *testing.T) {
	cfg := config.DefaultConfig()
	graph := NewGameGraph(cfg)
	processor := NewTurnProcessor(cfg, graph)
	state := InitTurnState(cfg, graph)
	pathID := "rivendell-to-moria"
	state.Paths[pathID].Status = "BLOCKED"
	state.Paths[pathID].BlockedBy = "saruman"
	state.Paths[pathID].Corrupted = true
	state.Units["gandalf"].CurrentRegion = "rivendell"

	processor.ProcessTurn(state, []Order{{OrderType: "MAIA_ABILITY", UnitID: "gandalf", TargetPathID: pathID}})
	if state.Paths[pathID].Status != "TEMPORARILY_OPEN" {
		t.Fatalf("Gandalf ability left path %s, want TEMPORARILY_OPEN", state.Paths[pathID].Status)
	}
}

func countGameOver(events []GameEvent) int {
	count := 0
	for _, event := range events {
		if event.Topic == "game.broadcast" && strings.Contains(string(event.Data), `"GAME_OVER"`) {
			count++
		}
	}
	return count
}

func countGameOverByWinner(events []GameEvent, winner string) int {
	count := 0
	for _, event := range events {
		data := string(event.Data)
		if event.Topic == "game.broadcast" && strings.Contains(data, `"GAME_OVER"`) && strings.Contains(data, `"winner":"`+winner+`"`) {
			count++
		}
	}
	return count
}

func countRingDetection(events []GameEvent) int {
	count := 0
	for _, event := range events {
		if event.Topic == "game.ring.detection" {
			count++
		}
	}
	return count
}

func gameOverPayloads(events []GameEvent) []string {
	var payloads []string
	for _, event := range events {
		if event.Topic == "game.broadcast" && strings.Contains(string(event.Data), `"GAME_OVER"`) {
			payloads = append(payloads, string(event.Data))
		}
	}
	return payloads
}
