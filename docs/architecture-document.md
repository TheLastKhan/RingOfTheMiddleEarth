# Ring of the Middle Earth Architecture Document

## Technology Choice

This repository implements **Option B: Go + Kafka**.

Go is used for the HTTP API, SSE fan-out, turn processing, validation support, route/intercept analysis, and game-state cache. Kafka provides the shared event log and durable integration surface between Go engines, Kafka Streams, Schema Registry, and the UI.

The main tradeoff is deliberate: the demo favors a compact, inspectable Go implementation with Kafka-backed topics and snapshots over a heavier full replay engine. The current system produces session snapshots to the compacted `game.session` topic and sends terminal GameOver events through a Kafka transactional producer with deterministic event identity.

## Runtime Services

| Service | Role |
| --- | --- |
| `go-engine-1..3` | Interchangeable Go API/game-engine instances |
| `nginx` | Load balances HTTP/SSE traffic across Go engines |
| `kafka-1..3` | Kafka brokers for orders, events, broadcasts, and session snapshots |
| `schema-registry` | Avro schema registry |
| `kafka-streams` | Order validation and route-risk enrichment topology |
| `ui` | Static browser client on port 3000 |
| `zookeeper` | Kafka coordination for the Confluent 7.5 stack |

## Kafka Topics

| Topic | Purpose |
| --- | --- |
| `game.orders.raw` | Raw orders accepted from players |
| `game.orders.validated` | Validated orders plus V2 route-risk enrichment fields |
| `game.events.unit` | Unit movement, damage, respawn, and cooldown events |
| `game.events.region` | Region control, fortification, and combat events |
| `game.events.path` | Path block, temporary-open, surveillance, and corruption events |
| `game.session` | Log-compacted latest world-state/session snapshot |
| `game.broadcast` | World-state snapshots and global events for SSE/UI |
| `game.ring.position` | Light-only Ring Bearer movement stream |
| `game.ring.detection` | Dark-side Ring Bearer detection stream |
| `game.dlq` | Invalid order records with error codes |

## Goroutine and Channel Map

| Component | Main channels |
| --- | --- |
| HTTP server | Receives `/order`, validates, sends to `OrderCh` |
| Kafka producer bridge | Reads `OrderCh`, produces to `game.orders.raw` and `game.orders.validated` |
| Turn ticker | Every 60 seconds drains validated orders and calls `TurnProcessor.ProcessTurn` |
| Event router | Routes generated events to `lightSSECh`, `darkSSECh`, and broadcast channels |
| SSE handlers | Stream side-specific events to connected browser clients |
| Cache manager | Applies world snapshots/events to the in-memory serving cache |

The select loops handle order intake, tick processing, SSE heartbeat/disconnect, Kafka production, event fan-out, and graceful cancellation. The pprof endpoints are mounted at `/debug/pprof/*` for goroutine checks after long-running demos.

## Order Flow

1. The browser submits an order to `/order`.
2. The Go server normalizes payload fields and validates ownership, turn, path, target, Maia cooldown, and duplicate-order rules.
3. Accepted orders are put on `OrderCh`.
4. The producer bridge writes accepted orders to Kafka.
5. Kafka Streams validates raw records and writes valid orders or DLQ records.
6. Route orders are enriched with `routeRiskScore`, `threatenedPaths`, and `blockedPaths`.
7. The Go turn processor consumes the current turn's validated orders and applies the 13-step turn sequence.
8. Events update the cache and are sent to side-specific SSE streams.

## Information Asymmetry

Information hiding is enforced in `option-b/internal/router/event_router.go` and the API state serializer:

- Light sees the Ring Bearer's real region.
- Dark receives Ring Bearer location only through detection events.
- Dark `/game/state` responses strip the Ring Bearer's current region unless detection has exposed it.
- Unit ownership is checked before orders are accepted.

## Turn Processing Coverage

The Go turn processor implements the project sequence:

1. collect validated orders
2. assign/redirect routes
3. block/search paths
4. reinforce/deploy
5. fortify
6. Maia abilities
7. auto-advance units
8. combat
9. path timers and blocker release checks
10. fortification timers
11. respawn and cooldown timers
12. Ring detection
13. win-condition checks

The Light win condition requires a `DESTROY_RING` order, Ring Bearer at Mount Doom, and no active Shadow unit in the same region. Shadow wins when detection/exposure and capture conditions are satisfied. Draw is emitted after the maximum turn count.

## Fault Tolerance

The system is fault tolerant enough for the term-project demo:

- Nginx continues routing when one Go engine is stopped.
- Kafka keeps order/event/session topics durable.
- `game.session` is compacted and receives the latest world snapshot.
- GameOver emission uses a transactional Kafka producer and an application-level duplicate guard.

The current implementation is not a full production replay engine for every possible side-effect boundary. GameOver has a transactional producer and a `read_committed` smoke test, while exhaustive crash-matrix testing remains production hardening.

## Schema Evolution

`game.orders.validated-value` has a compatible V2 schema with nullable/defaulted fields:

- `routeRiskScore`
- `threatenedPaths`
- `blockedPaths`

Existing V1 consumers can continue reading because the V2 fields have defaults. `game.session-value` is registered with a flexible world-state snapshot schema.

## Last Verified Test Run

- `go test ./...` passed in `option-b`.
- `scripts/demo-validation-k4.ps1` passed with 9 Kafka Streams validation tests and 0 failures.
- `docker compose up -d --build` started the Go engines and Kafka Streams successfully.
- All main services were up: nginx, UI, 3 Go engines, Kafka Streams, Schema Registry, 3 Kafka brokers, and Zookeeper.
- Schema Registry exposed 10 subjects.
- `game.orders.validated-value` versions `[1,2]` and `game.session-value` version `[1]` were present.
- `/health`, `/analysis/routes`, `/analysis/intercept`, and `/debug/pprof/goroutine?debug=1` responded.
- Fault tolerance smoke test passed: after stopping `go-engine-2`, 5 of 5 nginx `/health` requests succeeded.
- `scripts/check-gameover-idempotency.ps1` passed: a Light victory added exactly one committed GameOver record, and extra turn advances did not add another.

## LLM Usage Log

AI assistance was used for:

- translating the project rubric into implementation checklists
- debugging Docker/Kafka startup and schema-registration issues
- improving UI readability and map rendering details
- identifying gaps between the implementation and the specification
- adding targeted tests around GameOver, path timers, Maia abilities, and validation behavior

The project-specific rules, map data, game flow, and implementation decisions were reviewed against `TermProject_RingOfTheMiddleEarth.md` and local test results.

## Rubric Evidence Appendix

| Rubric item | Evidence |
| --- | --- |
| K4 validation rules | `kafka/streams/src/test/java/rotr/streams/OrderValidatorTest.java` covers all 8 error-code cases. `scripts/demo-validation-k4.ps1` runs those tests through Maven/Docker. |
| K5 route risk enrichment | `RouteRiskEnricher` attaches `routeRiskScore`, `threatenedPaths`, and `blockedPaths`; `/analysis/routes` also exposes ranked route risk in the UI/API. |
| K6 GameOver exactly once | GameOver records use deterministic event identity, stable keying, app-level duplicate suppression, and `kafkalite.TransactionalProducer` backed by franz-go transactions. `scripts/check-gameover-idempotency.ps1` verifies committed GameOver output increments exactly once. |
| B2 fault tolerance | `docker compose stop go-engine-2` followed by repeated `GET /health` through nginx verifies surviving engines keep serving. |
| B7 information hiding | `router_test.go` verifies side-specific routing; Dark state strips Ring Bearer region. |
| B8 Go pipelines | `pipeline1_test.go` and `pipeline2_test.go` verify route risk and interception outputs. |
| B9 goroutine leaks | `/debug/pprof/goroutine?debug=1` is enabled. `scripts/check-pprof-10turns.ps1` records before/after goroutine totals over approximately 10 turns. |

## Detailed LLM Interaction Appendix

| Interaction | Prompt summary | Used | Changed or rejected |
| --- | --- | --- | --- |
| Docker startup | Diagnose Docker daemon/image and missing `go.sum` build failure. | Error analysis and local build inspection. | Fixed local build flow; did not add unrelated dependency churn. |
| UI readability | Increase map/panel text readability and align canvas map with SVG. | Visual/layout suggestions. | Accepted scoped CSS/canvas updates; reverted login-screen changes when requested. |
| Map fidelity | Compare canvas node names, colors, terrain icons, legends, and path styles with SVG. | Local file inspection and targeted UI edits. | Accepted matching names/colors/icons; avoided broad redesign. |
| Gameplay debugging | Explain Reconnecting/SSE, side ownership errors, and localhost stale build behavior. | Runtime checks and Docker rebuild guidance. | Fixed backend connectivity and validation issues where needed. |
| Rubric gap analysis | Identify unfinished requirements from `TermProject_RingOfTheMiddleEarth.md`. | Checklist against K/B rubric items. | Accepted route/intercept, session, GameOver, pprof, and docs work; documented production limits honestly. |
| Remaining-gap completion | Complete K4/K6/B2/B9/B11 evidence. | Tests, scripts, architecture appendix, transactional GameOver producer, and deterministic GameOver identity. | Kept exhaustive crash-matrix proof scoped as production hardening. |
