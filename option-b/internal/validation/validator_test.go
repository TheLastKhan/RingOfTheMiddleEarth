package validation

import (
	"testing"

	"rotr/internal/cache"
	"rotr/internal/config"
)

func TestSauronCannotReceiveOrders(t *testing.T) {
	cfg := config.DefaultConfig()
	worldCache := cache.NewWorldStateCache(cfg)
	validator := NewValidator(cfg, worldCache)

	result := validator.Validate(Order{
		OrderType:  "MAIA_ABILITY",
		PlayerID:   "dark-player",
		PlayerSide: "SHADOW",
		UnitID:     "sauron",
		Turn:       1,
	})

	if result.Valid {
		t.Fatalf("Sauron order validated, want rejection")
	}
	if result.ErrorCode != ErrUnitCannotAct {
		t.Fatalf("Sauron error = %s, want %s", result.ErrorCode, ErrUnitCannotAct)
	}
}

func TestIndestructibleShadowCannotRouteToMountDoom(t *testing.T) {
	cfg := config.DefaultConfig()
	worldCache := cache.NewWorldStateCache(cfg)
	validator := NewValidator(cfg, worldCache)

	result := validator.Validate(Order{
		OrderType:  "ASSIGN_ROUTE",
		PlayerID:   "dark-player",
		PlayerSide: "SHADOW",
		UnitID:     "witch-king",
		Turn:       1,
		PathIDs: []string{
			"minas-morgul-to-cirith-ungol",
			"cirith-ungol-to-mount-doom",
		},
	})

	if result.Valid {
		t.Fatalf("Witch-King route to Mount Doom validated, want rejection")
	}
	if result.ErrorCode != ErrInvalidTarget {
		t.Fatalf("Witch-King error = %s, want %s", result.ErrorCode, ErrInvalidTarget)
	}
}
