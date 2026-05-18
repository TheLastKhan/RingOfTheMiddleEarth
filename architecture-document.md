# Ring of the Middle Earth Architecture Document

## Technology Choice

This repository implements **Option B: Go + Kafka**.

Go is used for the HTTP API, SSE fan-out, turn processing, validation support, route/intercept analysis, and game-state cache. Kafka provides the shared event log and durable integration surface between Go engines, Kafka Streams, Schema Registry, and the UI.

The main tradeoff is deliberate: the demo favors a compact, inspectable Go implementation with Kafka-backed topics and snapshots over a heavier fully transactional game-state replay engine. The current system produces session snapshots to the compacted `game.session` topic and uses app-level duplicate guards for terminal events.

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
- GameOver emission has an application-level duplicate guard.

The current implementation is not a full production transactional replay engine. A crash exactly between all possible side effects is not proven with a transactional Kafka producer in Go, and Go engine restart recovery does not yet rebuild every in-memory structure exclusively from Kafka replay. Those are the main production-hardening boundaries.

## Schema Evolution

`game.orders.validated-value` has a compatible V2 schema with nullable/defaulted fields:

- `routeRiskScore`
- `threatenedPaths`
- `blockedPaths`

Existing V1 consumers can continue reading because the V2 fields have defaults. `game.session-value` is registered with a flexible world-state snapshot schema.

## LLM Usage Log

AI assistance was used for:

- translating the project rubric into implementation checklists
- debugging Docker/Kafka startup and schema-registration issues
- improving UI readability and map rendering details
- identifying gaps between the implementation and the specification
- adding targeted tests around GameOver, path timers, Maia abilities, and validation behavior

The project-specific rules, map data, game flow, and implementation decisions were reviewed against `TermProject_RingOfTheMiddleEarth.md` and local test results.
