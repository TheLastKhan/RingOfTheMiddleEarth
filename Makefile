# ═══════════════════════════════════════════════════════
# Ring of the Middle Earth — Makefile
# ═══════════════════════════════════════════════════════

.PHONY: all up down build test logs clean fault-test demo-1 demo-2 demo-3

# ── Start all services ──
up:
	docker compose up -d --build
	@echo "✅ All services started"
	@echo "🌐 UI:              http://localhost:3000"
	@echo "🎮 Game Engine:     http://localhost:80"
	@echo "📊 Schema Registry: http://localhost:8081"

# ── Stop all services ──
down:
	docker compose down -v
	@echo "🛑 All services stopped"

# ── Build Go engine ──
build:
	cd option-b && go build -o ../bin/rotr-engine ./cmd/server
	@echo "✅ Build complete: bin/rotr-engine"

# ── Run unit tests with -race flag ──
test:
	cd option-b && go test -race -v ./...
	@echo "✅ All tests passed with -race"

# ── View logs ──
logs:
	docker compose logs -f

# ── View specific service logs ──
logs-%:
	docker compose logs -f $*

# ── Fault tolerance test: stop one Go instance ──
fault-test:
	@echo "🔧 Fault tolerance test: stopping go-engine-2..."
	docker compose stop go-engine-2
	@echo "⏳ Waiting 10s for consumer group rebalance..."
	@sleep 10
	@echo "📊 Checking health of remaining engines..."
	curl -s http://localhost:8080/health | jq .
	@echo "✅ Fault test complete. Restart with: make fault-recover"

# ── Recover from fault test ──
fault-recover:
	docker compose start go-engine-2
	@echo "✅ go-engine-2 restarted"

# ── Clean everything ──
clean:
	docker compose down -v --rmi all
	rm -rf bin/
	@echo "🗑️  Everything cleaned"

# ── Demo 1: Information Hiding ──
demo-1:
	@echo "═══════════════════════════════════════"
	@echo "DEMO 1: Information Hiding"
	@echo "═══════════════════════════════════════"
	@echo "1. Open two browser tabs:"
	@echo "   Light Side: http://localhost:3000?side=light"
	@echo "   Dark Side:  http://localhost:3000?side=dark"
	@echo "2. Move Ring Bearer to a new region"
	@echo "3. Verify: Light Side sees the region"
	@echo "4. Verify: Dark Side sees '???' for Ring Bearer"

# ── Demo 2: Maia Abilities + Path Mechanics ──
demo-2:
	@echo "═══════════════════════════════════════"
	@echo "DEMO 2: Maia Abilities + Path Mechanics"
	@echo "═══════════════════════════════════════"
	@echo "1. Use Saruman's MAIA_ABILITY on a path"
	@echo "2. Verify path becomes BLOCKED"
	@echo "3. Use Saruman again → cooldown error"
	@echo "4. Wait cooldown turns, try again → success"

# ── Demo 3: Fault Tolerance ──
demo-3:
	@echo "═══════════════════════════════════════"
	@echo "DEMO 3: Fault Tolerance"
	@echo "═══════════════════════════════════════"
	@echo "1. Run: make fault-test"
	@echo "2. Observe consumer group rebalance in logs"
	@echo "3. Verify game continues on remaining instances"
	@echo "4. Run: make fault-recover"
