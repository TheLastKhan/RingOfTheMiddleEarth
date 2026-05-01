# Ring of the Middle Earth

**Distributed Application Development — Term Project**  
**Technology Choice: Option B — Go**

---

## Technology Choice

This project implements the game engine using **Go 1.22+** with **confluent-kafka-go v2**.

**Why Go over Akka:**

The Go + Kafka stateless architecture maps naturally to this problem. All authoritative game state lives in Kafka KTables — Go instances are fully interchangeable. When a node crashes, Kafka consumer group rebalance reassigns its partitions to surviving instances within seconds, with zero application-layer coordination. State recovery is Kafka partition replay, not actor journal recovery.

The single hardest constraint — information asymmetry — is enforced in one place: `EventRouter.route()`. `game.ring.position` goes exclusively to `lightSSECh`, `game.ring.detection` exclusively to `darkSSECh`. This is verified with `go test -race`.

---

## Repository Structure

```
ring-of-the-middle-earth/
├── docker-compose.yml
├── Makefile
├── README.md
├── config/
│   ├── units.conf          14 units, all config-driven
│   └── map.conf            22 regions, 37 paths, 4 canonical routes
├── kafka/
│   ├── schemas/            11 Avro .avsc files + register script
│   ├── streams/            Kafka Streams Topology 1 & 2 (Java)
│   └── init/               create-topics.sh
├── nginx/
│   └── nginx.conf
├── option-b/               Go implementation
│   ├── go.mod
│   ├── cmd/server/
│   │   └── main.go
│   └── internal/
│       ├── api/            HTTP handlers + SSE
│       ├── cache/          WorldStateCache + CacheManager
│       ├── config/         Config loader
│       ├── game/           TurnProcessor, CombatEngine, GameGraph
│       ├── kafka/          Consumer + Producer
│       ├── pipeline/       Route Risk (P1) + Intercept (P2)
│       ├── router/         EventRouter — information asymmetry
│       └── validation/     Order validation rules
└── ui/
    └── index.html          Vanilla JS + SSE, no framework
```

---

## Prerequisites

| Tool | Version |
|------|---------|
| Docker | 24+ |
| Docker Compose | v2 |
| Go | 1.22+ (for `make test`) |
| Java | 17+ (for Kafka Streams build) |
| Make | any |

---

## Quick Start

```bash
# Clone
git clone https://github.com/yourusername/ring-of-the-middle-earth
cd ring-of-the-middle-earth

# Start everything
make up

# Wait ~90 seconds for Kafka to be ready
# Then open two browser tabs:
# Tab 1 (Light Side): http://localhost:3000
# Tab 2 (Dark Side):  http://localhost:3000
```

---

## Make Targets

```bash
make up              # Build + start all services (detached)
make down            # Stop all services + remove volumes
make test            # Run unit tests — no Docker required
make logs            # Follow Go instance logs
make logs-kafka      # Follow Kafka broker logs
make ps              # Show service status
make check-topics    # Describe all 10 Kafka topics
make register-schemas # Register Avro schemas in Schema Registry
make fault-test      # Demo Scenario 3: stop go-2, observe rebalance
make check-game-over # Count GameOver events in game.broadcast
make clean           # Remove all containers + images
```

---

## Running Unit Tests (No Docker Required)

```bash
cd option-b
go test -race ./...

# Expected output:
# ok  rotr/internal/game      0.12s
# ok  rotr/internal/router    0.11s
# ok  rotr/internal/pipeline  0.09s
```

Test files:

| File | Tests | Rubric |
|------|-------|--------|
| `internal/game/combat_test.go` | 6 combat formula cases | B3 |
| `internal/router/router_test.go` | 3 information hiding cases | B7 |
| `internal/pipeline/pipeline1_test.go` | 2 route risk cases | B8 |
| `internal/pipeline/pipeline2_test.go` | 2 intercept cases | B8 |

---

## Services

| Service | Port | Description |
|---------|------|-------------|
| nginx | 80 | Load balancer → 3 Go instances |
| go-1 | 8080 | Go game engine instance 1 |
| go-2 | 8081 | Go game engine instance 2 |
| go-3 | 8082 | Go game engine instance 3 |
| kafka-1 | 29092 | Kafka broker 1 |
| kafka-2 | 29093 | Kafka broker 2 |
| kafka-3 | 29094 | Kafka broker 3 |
| schema-registry | 8081 | Confluent Schema Registry |
| kafka-streams | 8090 | Topology 1 + 2 |
| ui | 3000 | Game UI |
| zookeeper | 2181 | Zookeeper |

---

## Service Startup Order

```
zookeeper → kafka-1/2/3 → schema-registry → kafka-init → kafka-streams + go-1/2/3 → nginx
```

`kafka-init` runs once: creates 10 topics and registers Avro schemas, then exits.

---

## Kafka Topics

| Topic | Partitions | Cleanup | Key |
|-------|-----------|---------|-----|
| game.orders.raw | 3 | delete 1h | playerId |
| game.orders.validated | 6 | delete 1h | unitId |
| game.events.unit | 6 | delete 7d | unitId |
| game.events.region | 6 | delete 7d | regionId |
| game.events.path | 6 | delete 7d | pathId |
| game.session | 1 | **compact** | — |
| game.broadcast | 1 | delete 1h | — |
| game.ring.position | 1 | delete 1h | — |
| game.ring.detection | 2 | delete 1h | playerId |
| game.dlq | 3 | delete 7d | errorCode |

---

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/game/start` | POST | `{"mode":"HVH","lightPlayerId":"...","darkPlayerId":"..."}` |
| `/game/state` | GET | World state. Ring Bearer region stripped for Dark Side. |
| `/order` | POST | Submit order → 202 Accepted |
| `/orders/available` | GET | `?unitId=X&playerId=Y` |
| `/events` | GET | SSE stream `?playerId=Y` |
| `/analysis/routes` | GET | Light Side only — ranked route risk |
| `/analysis/intercept` | GET | Dark Side only — Nazgul intercept plan |
| `/health` | GET | 200 OK or 503 |

---

## Demo Scenarios

### Scenario 1 — Information Hiding

```bash
# Terminal 1: Light Side SSE
curl -N "http://localhost:80/events?playerId=light-player"

# Terminal 2: Dark Side SSE
curl -N "http://localhost:80/events?playerId=dark-player"

# After turn end with Witch-King 1 hop from Ring Bearer:
# Terminal 1: RingBearerMoved visible, RingBearerDetected NOT present
# Terminal 2: RingBearerDetected visible, ring-bearer.currentRegion=""

# Verify:
curl "localhost:80/game/state?playerId=light-player" | jq '.units[]|select(.id=="ring-bearer")'
curl "localhost:80/game/state?playerId=dark-player"  | jq '.units[]|select(.id=="ring-bearer")'
```

### Scenario 2 — Maia Dispatch

```bash
# Same orderType, different effect based on config:

# Gandalf OpenPath (path turns TEMPORARILY_OPEN)
curl -X POST localhost:80/order -H "Content-Type: application/json" \
  -d '{"orderType":"MAIA_ABILITY","playerId":"light-player","unitId":"gandalf","turn":6,"payload":{"targetPathId":"rivendell-to-moria"}}'

# Saruman CorruptPath (surveillanceLevel=3, permanent)
curl -X POST localhost:80/order -H "Content-Type: application/json" \
  -d '{"orderType":"MAIA_ABILITY","playerId":"dark-player","unitId":"saruman","turn":6,"payload":{"targetPathId":"fords-of-isen-to-edoras"}}'
```

### Scenario 3 — Fault Tolerance + Exactly-Once

```bash
# Run fault tolerance test
make fault-test

# Exactly-once: advance Ring Bearer to Mount Doom, submit DestroyRing, kill engine
curl -X POST localhost:80/order \
  -d '{"orderType":"DESTROY_RING","playerId":"light-player","unitId":"ring-bearer","turn":15}'

docker stop go-1 go-2 go-3
docker start go-1 go-2 go-3

# Count GameOver in broadcast — must be exactly 1
make check-game-over
```

---

## Goroutine Leak Test

```bash
# After 10 turns, check goroutine count
curl -s localhost:6060/debug/pprof/goroutine?debug=1 | head -5

# Count should remain stable — no growth over time
```

---

## Schema Evolution Demo (V2)

```bash
# Check current compatibility
curl localhost:8081/config/game.orders.validated-value

# Test V2 compatibility before deploying
curl -X POST -H "Content-Type: application/vnd.schemaregistry.v1+json" \
  -d "{\"schema\": $(cat kafka/schemas/game.orders.validated.v2.avsc | jq -c . | jq -R .)}" \
  localhost:8081/compatibility/subjects/game.orders.validated-value/versions/latest
# Expected: {"is_compatible":true}

# Deploy V2 while V1 consumers run
curl -X POST -H "Content-Type: application/vnd.schemaregistry.v1+json" \
  -d "{\"schema\": $(cat kafka/schemas/game.orders.validated.v2.avsc | jq -c . | jq -R .)}" \
  localhost:8081/subjects/game.orders.validated-value/versions
# Expected: {"id":2}

# V1 consumers continue without error
docker logs go-1 --tail=10  # No schema errors
```

---

## Key Design Decisions

**No unit ID string literals in game logic.**  
All unit behaviour is config-driven. `cfg.DetectionRange > 0` identifies Nazgul. `cfg.Maia && cfg.CanOpenPath()` identifies Gandalf. Running `grep -r "witch-king" option-b/internal/game/` returns zero results.

**Single information asymmetry enforcement point.**  
`EventRouter.route()` in `internal/router/event_router.go` is the only place routing decisions are made. Verified with `go test -race ./internal/router/...`.

**Stateless application tier.**  
All authoritative state lives in Kafka KTables. Any Go instance can handle any request. Fault tolerance is delegated to Kafka consumer group protocol.

**Exactly-once GameOver.**  
`enable.idempotence=true` on the producer. Verified with `make check-game-over` after engine crash + restart.

---

## Academic Integrity

AI tools were used to understand concepts (Kafka KTable semantics, Go pipeline patterns, Docker Compose healthcheck syntax). All game logic — combat formula, detection formula, 13-step turn processing, information asymmetry enforcement, state machine transitions — was written directly from the project specification.

See `architecture-document.pdf` for the full LLM usage log.
