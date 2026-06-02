# Ring of the Middle Earth - Detailed In-Person Presentation Guide

This document is the English version of the in-person presentation guide.
Its purpose is not only to demonstrate that the application runs, but also to explain the distributed-systems concepts measured by the project: Kafka, event-driven state, validation, fault tolerance, information hiding, analysis pipelines, SSE, and multi-instance deployment.

Technology choice:

```text
Option B - Go + Kafka
```

In short, this project is a two-player, turn-based strategy game. The Light Side tries to move the Ring Bearer from The Shire to Mount Doom and destroy the Ring. The Shadow Side tries to locate, intercept, or destroy the Ring Bearer. Technically, the UI, Go game engines, Kafka topics, Kafka Streams, Schema Registry, nginx, and Docker Compose work together as a distributed application.

For visual support during the presentation, open:

```text
TEKNIK_AKIS_DIYAGRAMLARI.md
```

That file contains Mermaid diagrams for the architecture, player-order flow, End Turn pipeline, Kafka topics, SSE information hiding, fault tolerance, and exactly-once GameOver flow.

---

## 1. Before the Presentation

Start the full system:

```powershell
cd C:\Users\hakan\termproject
docker compose up -d --build
docker compose ps
```

Expected containers:

- `rotr-ui`
- `rotr-nginx`
- `rotr-go-1`, `rotr-go-2`, `rotr-go-3`
- `rotr-kafka-1`, `rotr-kafka-2`, `rotr-kafka-3`
- `rotr-schema-registry`
- `rotr-kafka-streams`
- `rotr-zookeeper`

Check the health endpoint:

```powershell
Invoke-RestMethod http://localhost/health
```

Check Schema Registry:

```powershell
Invoke-RestMethod http://localhost:8081/subjects
```

Run the Go tests:

```powershell
cd C:\Users\hakan\termproject\option-b
go test ./...
cd ..
```

Run the Kafka Streams validation tests:

```powershell
.\scripts\demo-validation-k4.ps1
```

Run the transactional GameOver exactly-once smoke tests:

```powershell
.\scripts\check-gameover-idempotency.ps1
.\scripts\check-gameover-idempotency-dark.ps1
```

Run the E2E and fault-tolerance smoke tests:

```powershell
.\scripts\full-e2e-smoke.ps1
.\scripts\chaos-soak-smoke.ps1
```

Run the browser/UI smoke test:

```powershell
.\scripts\browser-smoke.ps1
```

You do not have to run every script during the live presentation. A short and reliable pre-demo check is:

```powershell
docker compose ps
Invoke-RestMethod http://localhost/health
Invoke-RestMethod http://localhost:8081/subjects
.\scripts\demo-validation-k4.ps1
```

---

## 2. Suggested 15-20 Minute Presentation Flow

Use this order:

1. Explain the project goal and game rules.
2. Explain the architecture: UI, nginx, Go engines, Kafka, Kafka Streams, and Schema Registry.
3. Open the Light and Shadow browser views.
4. Demonstrate information hiding.
5. Explain how an order travels from the browser to the backend.
6. Explain the End Turn pipeline.
7. Explain Kafka topics and event-driven state.
8. Demonstrate Kafka Streams validation and schema evolution.
9. Demonstrate route-risk and interception analysis.
10. Demonstrate fault tolerance by stopping one engine.
11. Show tests and evidence.
12. Finish with a short code walkthrough and Q&A.

Suggested opening:

```text
For this project I selected Option B, the Go and Kafka architecture. I implemented a two-player turn-based strategy game to demonstrate Kafka topics, event-driven state, order validation, information hiding, fault tolerance, and a multi-service deployment.
```

---

## 3. Explain the Architecture

Open `docker compose ps`, `docker-compose.yml`, or the first diagram in `TEKNIK_AKIS_DIYAGRAMLARI.md`.

High-level architecture:

```text
Browser UI
   |
   | HTTP / SSE
   v
nginx load balancer
   |
   v
3 Go game-engine instances
   |
   v
Kafka topics
   |
   v
Kafka Streams validation and enrichment
   |
   v
Schema Registry
```

Explain:

```text
This is not a single monolithic application. Docker Compose starts multiple services. The browser UI is served separately. HTTP API requests and SSE connections go through nginx. Nginx routes requests to one of three Go-engine instances. Kafka provides the event and integration layer for orders, game events, broadcasts, and session snapshots.
```

Main services:

| Service | Port | Purpose |
| --- | --- | --- |
| UI | `3000` | Static browser game |
| nginx | `80` | Load balancer for HTTP and SSE |
| `go-engine-1` | `8080` | Go API and game engine |
| Schema Registry | `8081` | Avro schema registry |
| `go-engine-2` | `8082` | Go API and game engine |
| `go-engine-3` | `8083` | Go API and game engine |
| Kafka brokers | `9092`, `9093`, `9094` | Distributed Kafka cluster |
| Zookeeper | `2181` | Kafka coordination |

Important note:

```text
Port 8081 is not a Go engine. It belongs to Schema Registry. The direct Go-engine ports are 8080, 8082, and 8083.
```

---

## 4. Open Both Browser Views

Open two browser windows:

```text
Light Side:
http://localhost:3000?side=light

Shadow Side:
http://localhost:3000?side=dark
```

Log in as Free Peoples in the Light window and as Shadow in the Dark window.

Explain:

```text
Both players observe the same game session, but they do not receive the same information. The Light Side can see the Ring Bearer's real position. The Shadow Side cannot see that position directly. Shadow receives location information only when a detection event is allowed by the game rules.
```

Show:

- The same map exists in both windows.
- The Light browser can follow Frodo.
- The Shadow browser does not receive the real Ring Bearer position by default.
- The connection-status text represents the SSE connection.
- `End Turn` manually triggers turn processing.
- `End Game` resets the session for a clean demo.

---

## 5. Explain the Game Rules

Light Side goal:

1. Assign a route to the Ring Bearer.
2. Advance Frodo along that route at turn boundaries.
3. Reach Mount Doom.
4. Submit a `DESTROY_RING` order.
5. Win if no active Shadow unit is present at Mount Doom.

Shadow Side goal:

1. Move Nazgul and other Shadow units to strategic locations.
2. Search or block paths.
3. Detect and expose the Ring Bearer.
4. Intercept or destroy the Ring Bearer.

Draw:

```text
If neither side wins before the maximum turn limit, the game ends in a draw.
```

Detection:

```text
Detection is disabled for the first three turns. Starting from turn four, Nazgul detection range, Sauron's bonus, and path-surveillance mechanics can expose the Ring Bearer.
```

There is no AI-controlled opponent:

```text
The specification describes two human players. Shadow units do not move automatically. The Shadow player must issue orders.
```

---

## 6. Demonstrate Light-Side Gameplay

In the Light browser:

1. Select Frodo Baggins.
2. Select the route-assignment action.
3. Click Mount Doom on the map.
4. Show that the UI fills the path list automatically.
5. Click `Issue Order`.
6. Click `End Turn`.

Explain:

```text
The user clicks a destination region instead of typing path IDs manually. The UI calculates the path list from the map graph. For Mount Doom, the Light-side route prefers Cirith Ungol because entering Mordor directly is dangerous due to Sauron's presence.
```

Example Light route:

```text
shire-to-bree
bree-to-rivendell
rivendell-to-lothlorien
lothlorien-to-emyn-muil
emyn-muil-to-ithilien
ithilien-to-cirith-ungol
cirith-ungol-to-mount-doom
```

After each processed turn:

- Frodo advances by one path.
- The event log displays a unit-movement event.
- Light can see the Ring Bearer's true position.
- Shadow does not receive the true position directly.

---

## 7. Demonstrate Shadow-Side Gameplay

In the Shadow browser:

1. Select the Witch-King or another Nazgul.
2. Explain strategic regions such as Minas Morgul, Cirith Ungol, Mordor, and Mount Doom.
3. Demonstrate path search, path block, route, or redirect actions.
4. Process the turn with `End Turn`.

Explain:

```text
The Shadow Side tries to cover important corridors. Search actions increase surveillance. Blocking affects movement through a path. If the Ring Bearer becomes exposed and a Shadow unit intercepts him, Shadow wins.
```

---

## 8. Explain the Player-Order Flow

Open:

```text
index.html
option-b/internal/api/server.go
option-b/internal/validation/validator.go
TEKNIK_AKIS_DIYAGRAMLARI.md
```

Explain:

```text
When a player clicks Issue Order, the browser sends a JSON request to POST /order. nginx forwards the request to a Go engine. The Go API normalizes the payload and performs fast validation checks. Accepted orders are sent through OrderCh to the Kafka producer bridge. Kafka Streams consumes the raw order stream, validates or enriches records, and separates invalid records into the DLQ flow.
```

Example payload:

```json
{
  "playerId": "light-player",
  "playerSide": "FREE_PEOPLES",
  "turn": 1,
  "unitId": "ring-bearer",
  "orderType": "ASSIGN_ROUTE",
  "pathIds": ["shire-to-bree", "bree-to-rivendell"]
}
```

Important validations:

- A player can command only a unit belonging to that side.
- The order turn must match the current turn.
- A unit must not receive a duplicate order during the same turn.
- `DESTROY_RING` is valid only for the Ring Bearer at Mount Doom with no active Shadow unit there.
- `SEARCH_PATH` and `BLOCK_PATH` require a unit at a path endpoint.

---

## 9. Explain End Turn and the Deterministic Pipeline

Open:

```text
option-b/internal/game/turn.go
```

Main function:

```text
ProcessTurn
```

Explain:

```text
This is a turn-based game. Submitting an order does not immediately move a unit. When End Turn is triggered, the game engine executes a deterministic pipeline. The sequence matters because routing, movement, combat, detection, and win-condition checks must occur in a predictable order.
```

The 13 steps:

1. Collect validated orders.
2. Assign or redirect routes.
3. Apply path blocking and searching.
4. Process reinforcement and deployment.
5. Apply fortification.
6. Apply Maia abilities.
7. Auto-advance units.
8. Resolve combat.
9. Update path timers.
10. Update fortification timers.
11. Update respawn and cooldown timers.
12. Calculate Ring Bearer detection.
13. Check win conditions.

Key sentence:

```text
The game state does not change randomly. Every turn follows the same deterministic sequence, which makes the system easier to test and debug.
```

---

## 10. Explain Kafka Topics and Event-Driven State

| Topic | Purpose |
| --- | --- |
| `game.orders.raw` | Raw player orders |
| `game.orders.validated` | Validated and enriched orders |
| `game.events.unit` | Unit movement and unit-status events |
| `game.events.region` | Region control and combat-result events |
| `game.events.path` | Path blocking, searching, and surveillance events |
| `game.broadcast` | World-state snapshots and global events such as GameOver |
| `game.ring.position` | Light-only Ring Bearer position events |
| `game.ring.detection` | Shadow-side detection events |
| `game.session` | Log-compacted latest session snapshot |
| `game.dlq` | Invalid-order records |

Explain:

```text
Kafka is not used merely as a message queue. It provides a shared event log and a durable integration surface. Separate topics make orders, validation results, game events, hidden information, and session snapshots easier to observe and reason about.
```

Check schemas:

```powershell
Invoke-RestMethod http://localhost:8081/subjects
Invoke-RestMethod http://localhost:8081/subjects/game.orders.validated-value/versions
Invoke-RestMethod http://localhost:8081/subjects/game.session-value/versions
```

Schema evolution explanation:

```text
Schema Registry tracks the expected message shape. The validated-order schema has multiple versions. New nullable or defaulted fields allow compatible schema evolution.
```

---

## 11. Explain Kafka Streams Validation

Open:

```text
kafka/streams
kafka/streams/src/test
```

Run:

```powershell
.\scripts\demo-validation-k4.ps1
```

Explain:

```text
Kafka Streams processes the raw-order flow. It applies validation and route-risk enrichment. Valid orders go to the validated topic, while invalid orders are separated into the DLQ flow with error information.
```

Exactly-once:

```text
Kafka Streams uses EXACTLY_ONCE_V2. For terminal GameOver events, the Go engine also uses a transactional Kafka producer. Dedicated Light and Dark victory smoke tests verify that the committed GameOver count increases exactly once.
```

---

## 12. Explain Information Hiding

Open:

```text
option-b/internal/router/event_router.go
option-b/internal/api/server.go
option-b/internal/cache/cache.go
```

Show API responses:

```powershell
Invoke-RestMethod "http://localhost/game/state?playerId=light-player&side=FREE_PEOPLES"
Invoke-RestMethod "http://localhost/game/state?playerId=dark-player&side=SHADOW"
```

Explain:

```text
Information hiding is enforced on the backend, not only in the UI. The Light Side can receive the Ring Bearer's real position. The Shadow Side receives a sanitized state response. Ring Bearer position events do not go to the Shadow SSE channel. Shadow receives only permitted detection events.
```

Technically:

- `game.ring.position` is a Light-only event stream.
- Ring Bearer position events never reach the Dark SSE channel.
- Broadcast snapshots are sanitized before Dark receives them.
- `DarkView.RingBearerRegion` remains empty.

Strong answer:

```text
This is not a frontend-only trick. Even if the Shadow player opens developer tools, the normal Dark state response does not contain the Ring Bearer's true currentRegion.
```

---

## 13. Explain SSE

Open:

```text
index.html
option-b/internal/api/server.go
```

Explain:

```text
The browser opens a Server-Sent Events connection to the backend. When the backend emits an event, the browser receives it without repeatedly polling for changes. SSE is a good fit because this UI mainly needs a one-way stream of state updates and game events from server to client.
```

Show:

- Connection status.
- Event log.
- Turn-number changes.
- Unit-moved events.
- Detection or GameOver events if available.

Why SSE instead of WebSocket?

```text
SSE is simpler for a server-to-browser event stream. This game does not require a full bidirectional socket protocol because player commands already use normal HTTP POST requests.
```

---

## 14. Explain Analysis Pipelines

Run:

```powershell
Invoke-RestMethod http://localhost/analysis/routes
Invoke-RestMethod http://localhost/analysis/intercept
```

Open:

```text
option-b/internal/pipeline
```

Explain:

```text
The analysis pipelines read the game state and produce decision-support information. Route analysis helps the Light Side compare route risk. Interception analysis helps the Shadow Side identify useful interception points for Nazgul units.
```

Why it matters:

```text
The specification asks for analysis pipelines in addition to gameplay. These endpoints transform the current state into meaningful analytical output.
```

---

## 15. Demonstrate Fault Tolerance

Run:

```powershell
docker compose stop go-engine-2
for ($i=1; $i -le 5; $i++) { Invoke-RestMethod http://localhost/health }
docker compose up -d go-engine-2
```

For stronger automated evidence:

```powershell
.\scripts\full-e2e-smoke.ps1
.\scripts\chaos-soak-smoke.ps1
```

Explain:

```text
The system has three Go-engine instances behind nginx. If one engine stops, nginx continues routing traffic to the remaining healthy engines. Kafka retains the order, event, and session topics. The game.session topic is log-compacted and stores the latest session snapshot.
```

Compare direct engine state:

```powershell
Invoke-RestMethod "http://localhost:8080/game/state?playerId=light-player&side=FREE_PEOPLES"
Invoke-RestMethod "http://localhost:8082/game/state?playerId=light-player&side=FREE_PEOPLES"
Invoke-RestMethod "http://localhost:8083/game/state?playerId=light-player&side=FREE_PEOPLES"
```

Be precise:

```text
This demonstrates fault tolerance at the term-project level: nginx failover, Kafka-backed session snapshots, and engine stop/start recovery. Exhaustive crash-matrix testing across every possible side-effect boundary would be additional production hardening.
```

---

## 16. Explain Transactional GameOver Exactly-Once Tests

Run:

```powershell
.\scripts\check-gameover-idempotency.ps1
.\scripts\check-gameover-idempotency-dark.ps1
```

Light scenario:

```text
Frodo reaches Mount Doom, submits DESTROY_RING, and the FREE_PEOPLES GameOver event is expected.
```

Dark scenario:

```text
Frodo deliberately follows a dangerous route into Mordor. Sauron defeats the Ring Bearer in combat, and the SHADOW GameOver event is expected.
```

Explain:

```text
The main purpose is not merely to show that Light or Shadow can win. The tests verify that a read_committed Kafka consumer observes exactly one new GameOver record and that extra advance-turn calls after the game ends do not create duplicate GameOver records.
```

---

## 17. Suggested Code Walkthrough

Open files in this order:

1. `docker-compose.yml`
   - Show services, ports, and three Go engines.

2. `TEKNIK_AKIS_DIYAGRAMLARI.md`
   - Show architecture and data-flow diagrams.

3. `index.html`
   - Show map canvas, order submission, and the SSE client.

4. `option-b/internal/api/server.go`
   - Show `/order`, `/game/state`, `/events`, and `/game/advance-turn`.

5. `option-b/internal/game/turn.go`
   - Show movement, combat, detection, and win conditions.

6. `option-b/internal/validation/validator.go`
   - Show ownership, turn, duplicate-order, target, and destroy-ring validation.

7. `option-b/internal/router/event_router.go`
   - Show Light/Dark SSE routing and information hiding.

8. `option-b/internal/pipeline`
   - Show route-risk and interception analysis.

9. `kafka/streams`
   - Show Kafka Streams validation and enrichment.

10. `scripts`
   - Show smoke tests and demo evidence.

---

## 18. Possible Questions and Answers

**Question: Why did you use Kafka?**

Kafka carries orders, events, and session snapshots. The event-driven model fits a turn-based game and provides observable topic separation, replay support, and a durable integration surface.

**Question: What does Kafka Streams do?**

Kafka Streams reads raw orders, applies validation and enrichment, sends valid records to the validated topic, and separates invalid records into the DLQ flow.

**Question: Is the UI showing only local state?**

No. The UI receives backend state through HTTP and SSE. Turn processing happens on the server.

**Question: How do you hide Frodo's position from Shadow?**

The backend sanitizes Dark state responses and prevents Light-only Ring Bearer position events from reaching the Dark SSE channel.

**Question: Is there an AI-controlled opponent?**

No. The specification requires two human players. Shadow units move only when the Shadow player issues orders.

**Question: What does End Turn do?**

It triggers the deterministic turn-processing pipeline: routes, movement, combat, timers, detection, and win-condition checks.

**Question: How strong is the exactly-once claim?**

Kafka Streams uses `EXACTLY_ONCE_V2`. GameOver uses a transactional producer and deterministic event identity. Light and Dark victory smoke tests use a `read_committed` consumer to verify that the committed GameOver count increases once and does not increase after extra turns.

**Question: Is this production-ready?**

It is a strong term-project implementation with multi-instance engines, Kafka, Schema Registry, validation, SSE, information hiding, fault-tolerance demos, and smoke tests. Longer chaos tests, broader browser automation, and exhaustive crash-matrix testing would be additional production hardening.

**Question: Why is port 8081 not included when comparing engine state?**

Port `8081` belongs to Schema Registry. The Go-engine ports are `8080`, `8082`, and `8083`.

---

## 19. Safest Short Demo Sequence

If time is limited:

1. Run `docker compose ps`.
2. Run `Invoke-RestMethod http://localhost/health`.
3. Open `TEKNIK_AKIS_DIYAGRAMLARI.md`.
4. Open the Light browser.
5. Open the Shadow browser.
6. Assign a Mount Doom route to Frodo.
7. Advance one or two turns.
8. Explain why Light sees Frodo while Shadow does not.
9. Show `/analysis/routes` and `/analysis/intercept`.
10. Run `demo-validation-k4.ps1`.
11. Stop and restart `go-engine-2`.
12. Open `turn.go`, `event_router.go`, and `server.go`.
13. Mention the two exactly-once smoke tests.

---

## 20. Suggested Closing

```text
This project is not only a browser game. The game is a practical way to demonstrate Kafka topics, Go-engine instances, Kafka Streams validation, Schema Registry, SSE, event-driven state, information hiding, and fault tolerance. Each major requirement has a visible implementation point in the codebase, and the smoke tests provide evidence for the important runtime behaviors.
```

---

## 21. Quick Recovery Commands

If the UI looks stale:

```powershell
docker compose up -d --build ui
```

Restart the full system:

```powershell
docker compose down
docker compose up -d --build
```

Reset volumes for a clean environment:

```powershell
docker compose down -v
docker compose up -d --build
```

If PowerShell blocks a script:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\demo-validation-k4.ps1
```

If Docker Desktop is not running:

```text
Open Docker Desktop, wait until the engine is ready, and run docker compose up -d --build again.
```

