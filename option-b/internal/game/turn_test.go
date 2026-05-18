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

func countGameOver(events []GameEvent) int {
	count := 0
	for _, event := range events {
		if event.Topic == "game.broadcast" && strings.Contains(string(event.Data), `"GAME_OVER"`) {
			count++
		}
	}
	return count
}
