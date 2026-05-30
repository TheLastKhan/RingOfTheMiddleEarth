# Ring of the Middle Earth

Distributed Application Development Term Project

Technology choice: **Option B - Go + Kafka**

## Current Status

This repository contains a playable/demo-ready distributed strategy game based on the Ring of the Middle Earth specification.

Latest verified checks:

- `go test ./...` passes.
- `scripts/demo-validation-k4.ps1` passes Kafka Streams validation tests: 9 tests, 0 failures.
- `scripts/check-gameover-idempotency.ps1` verifies committed transactional GameOver output increments exactly once for Light victory.
- `scripts/check-gameover-idempotency-dark.ps1` verifies the same exactly-once behavior for Dark victory.
- `scripts/chaos-soak-smoke.ps1` runs a configurable engine stop/start soak loop and checks replay convergence.
- `scripts/browser-smoke.ps1` checks the UI contract and, when Playwright CLI is available, captures Light/Dark screenshots.
- `docker compose up -d --build` starts the full stack.
- 3 Go engines, Kafka Streams, 3 Kafka brokers, Schema Registry, nginx, UI, and Zookeeper run together.
- `/health`, `/analysis/routes`, `/analysis/intercept`, Schema Registry, and pprof endpoints respond.
- Full E2E smoke test covers session replay, duplicate-order concurrency, SSE, and engine failover.
- Fault tolerance smoke test passes: stopping an engine still allows the game to advance through nginx.

Important honesty note: GameOver is produced through a Kafka transactional producer and verified with a `read_committed` smoke test. The project still treats exhaustive crash-matrix testing for every possible side-effect boundary as production hardening.

## Quick Start

Open Docker Desktop first, then run:

```powershell
cd C:\Users\hakan\termproject
docker compose up -d --build
docker ps
Invoke-RestMethod http://localhost/health
```

Open the game:

- Light side: `http://localhost:3000?side=light`
- Dark side: `http://localhost:3000?side=dark`

## How To Play

Light side tries to move the Ring Bearer from The Shire to Mount Doom and then submit `DESTROY_RING`.

Light wins when:

- Ring Bearer is at Mount Doom.
- `DESTROY_RING` is submitted that turn.
- No active Shadow unit is in Mount Doom.

Shadow tries to find, block, intercept, or destroy the Ring Bearer.

Shadow wins when:

- Ring Bearer is destroyed/captured by the game logic.

The game ends in a draw if the max turn limit is reached before either side wins.

## Main Services

| Service | Port | Purpose |
| --- | --- | --- |
| UI | 3000 | Browser game |
| nginx | 80 | Load balancer for Go engines |
| go-engine-1 | 8080 | Go game engine |
| go-engine-2 | 8082 | Go game engine |
| go-engine-3 | 8083 | Go game engine |
| schema-registry | 8081 | Avro schema registry |
| kafka-1 | 9092 | Kafka broker |
| kafka-2 | 9093 | Kafka broker |
| kafka-3 | 9094 | Kafka broker |
| zookeeper | 2181 | Kafka coordination |

## Useful Commands

Run Go tests:

```powershell
cd C:\Users\hakan\termproject\option-b
go test ./...
```

Run Kafka Streams validation tests:

```powershell
cd C:\Users\hakan\termproject
.\scripts\demo-validation-k4.ps1
```

Check schemas:

```powershell
Invoke-RestMethod http://localhost:8081/subjects
Invoke-RestMethod http://localhost:8081/subjects/game.orders.validated-value/versions
Invoke-RestMethod http://localhost:8081/subjects/game.session-value/versions
```

Check route/intercept analysis:

```powershell
Invoke-RestMethod http://localhost/analysis/routes
Invoke-RestMethod http://localhost/analysis/intercept
```

Check pprof:

```powershell
Invoke-RestMethod "http://localhost/debug/pprof/goroutine?debug=1"
```

Fault tolerance smoke test:

```powershell
.\scripts\full-e2e-smoke.ps1
```

Transactional GameOver exactly-once smoke test:

```powershell
.\scripts\check-gameover-idempotency.ps1
.\scripts\check-gameover-idempotency-dark.ps1
```

Chaos/soak hardening smoke:

```powershell
.\scripts\chaos-soak-smoke.ps1
.\scripts\chaos-soak-smoke.ps1 -Rounds 20 -StopSeconds 3
```

Browser/UI smoke:

```powershell
.\scripts\browser-smoke.ps1
.\scripts\browser-smoke.ps1 -RequireScreenshot
```

Stop everything:

```powershell
docker compose down
```

Reset volumes too:

```powershell
docker compose down -v
```

## Kafka Topics

| Topic | Purpose |
| --- | --- |
| `game.orders.raw` | Raw player orders |
| `game.orders.validated` | Validated and route-risk-enriched orders |
| `game.events.unit` | Unit events |
| `game.events.region` | Region events |
| `game.events.path` | Path events |
| `game.session` | Compacted latest world-state/session snapshot |
| `game.broadcast` | World snapshots and global events |
| `game.ring.position` | Light-only Ring Bearer position events |
| `game.ring.detection` | Shadow detection events |
| `game.dlq` | Invalid orders |

## API Endpoints

| Endpoint | Purpose |
| --- | --- |
| `GET /health` | Service health |
| `GET /game/state?playerId=...&side=...` | Side-specific world state |
| `POST /order` | Submit an order |
| `GET /orders/available` | List possible orders |
| `GET /events` | SSE stream |
| `GET /analysis/routes` | Light route risk analysis |
| `GET /analysis/intercept` | Shadow intercept analysis |
| `GET /debug/pprof/goroutine?debug=1` | Goroutine diagnostics |

## Tests And Evidence

| Rubric area | Evidence |
| --- | --- |
| Go combat/router/pipeline/turn logic | `go test ./...` |
| Kafka Streams 8 validation rules | `scripts/demo-validation-k4.ps1` |
| E2E gameplay/failover smoke | `scripts/full-e2e-smoke.ps1` |
| Transactional GameOver exactly-once smoke | `scripts/check-gameover-idempotency.ps1`, `scripts/check-gameover-idempotency-dark.ps1` |
| Chaos/soak hardening | `scripts/chaos-soak-smoke.ps1` |
| Browser/UI smoke | `scripts/browser-smoke.ps1` |
| Schema evolution | Schema Registry shows `game.orders.validated-value` versions `1,2` |
| Session schema | Schema Registry shows `game.session-value` version `1` |
| Fault tolerance | Stop one engine, advance turn through nginx, restarted engine replays `game.session` |
| pprof | `/debug/pprof/goroutine?debug=1` |

## Documentation

- `CALISTIRMA_REHBERI.md`: how to run and test the project.
- `SUNUM_REHBERI.md`: demo script and Q&A answers.
- `TEKNOLOJI_EGITIM.md`: technology explanations.
- `architecture-document.md`: architecture, tradeoffs, and rubric evidence.
- `YENI_DOSYALAR_ANALIZI.md`: documentation/file map.
- `sonnet.md`: archived learning notes summary.
