# Ring of the Middle Earth - Teknik Akis Diyagramlari

Bu belge projede "hangi veri nereden nereye gidiyor?" sorusunu teknik terimlerle gorsellestirir.
GitHub, asagidaki Mermaid diyagramlarini otomatik olarak render eder.

---

## 1. Sistem Mimarisi

```mermaid
flowchart LR
    LIGHT["Light Browser<br/>localhost:3000?side=light"]
    DARK["Shadow Browser<br/>localhost:3000?side=dark"]
    UI["Static UI<br/>nginx container<br/>host :3000"]
    LB["nginx Load Balancer<br/>host :80"]

    E1["go-engine-1<br/>host :8080"]
    E2["go-engine-2<br/>host :8082"]
    E3["go-engine-3<br/>host :8083"]

    K1["Kafka Broker 1<br/>host :9092"]
    K2["Kafka Broker 2<br/>host :9093"]
    K3["Kafka Broker 3<br/>host :9094"]
    KS["Kafka Streams<br/>validation + enrichment"]
    SR["Schema Registry<br/>host :8081"]
    ZK["Zookeeper<br/>host :2181"]

    LIGHT -->|"GET index.html"| UI
    DARK -->|"GET index.html"| UI
    LIGHT -->|"HTTP API + SSE"| LB
    DARK -->|"HTTP API + SSE"| LB

    LB -->|"reverse proxy"| E1
    LB -->|"reverse proxy"| E2
    LB -->|"reverse proxy"| E3

    E1 <-->|"produce / consume"| K1
    E2 <-->|"produce / consume"| K1
    E3 <-->|"produce / consume"| K1

    K1 <-->|"replication"| K2
    K2 <-->|"replication"| K3
    K3 <-->|"replication"| K1

    KS <-->|"Kafka topics"| K1
    SR <-->|"Avro schemas"| K1
    K1 <-->|"coordination"| ZK
```

Sunumda soyle:

```text
Browser UI statik olarak 3000 portundan aciliyor. API ve SSE trafigi nginx load balancer'a gidiyor. Nginx istekleri uc Go engine arasinda dagitiyor. Go engine'ler Kafka uzerinden order, event ve session snapshot akisini kullaniyor. Kafka Streams raw orderlari validate ve enrich ediyor. Schema Registry Avro mesaj semalarini takip ediyor.
```

Onemli port notu:

| Port | Servis |
| --- | --- |
| `3000` | Browser UI |
| `80` | nginx load balancer |
| `8080` | `go-engine-1` |
| `8081` | Schema Registry; Go engine degil |
| `8082` | `go-engine-2` |
| `8083` | `go-engine-3` |

---

## 2. Player Order Akisi

Ornek: Light oyuncu Frodo icin `ASSIGN_ROUTE` emri veriyor.

```mermaid
sequenceDiagram
    autonumber
    actor Player as Light Browser
    participant Nginx as nginx Load Balancer
    participant API as Go HTTP API
    participant Validator as Go Validator
    participant OrderCh as OrderCh
    participant Bridge as Kafka Producer Bridge
    participant Raw as game.orders.raw
    participant Streams as Kafka Streams Topology
    participant Valid as game.orders.validated
    participant DLQ as game.dlq
    participant Turn as TurnProcessor

    Player->>Nginx: POST /order (JSON)
    Nginx->>API: reverse proxy
    API->>Validator: Normalize + Validate(order)
    Validator-->>API: valid / invalid

    alt Go validation accepted
        API->>OrderCh: enqueue accepted order
        OrderCh->>Bridge: channel receive
        Bridge->>Raw: produce raw order record
        Raw->>Streams: consume raw order
        alt Kafka Streams validation accepted
            Streams->>Valid: produce validated + enriched order
            Valid->>Turn: current turn order input
            Turn-->>Player: result arrives later through SSE
        else Kafka Streams validation rejected
            Streams->>DLQ: produce invalid order + error code
        end
    else Go validation rejected
        API-->>Player: HTTP validation error
    end
```

Teknik olarak anlat:

```text
Order iki katmanda korunuyor. Go API hizli HTTP validation yapiyor. Kabul edilen order OrderCh kanalina yaziliyor. Producer bridge order'i Kafka'ya tasiyor. Kafka Streams topology de raw order akisini validation ve enrichment islemlerinden geciriyor. Gecerli kayitlar game.orders.validated topicine, gecersiz kayitlar game.dlq topicine ayriliyor.
```

Ilgili kod:

| Sorumluluk | Dosya |
| --- | --- |
| Frontend order payload | `index.html` |
| HTTP endpoint | `option-b/internal/api/server.go` |
| Go validation | `option-b/internal/validation/validator.go` |
| Kafka producer bridge | `option-b/cmd/server/main.go` |
| Kafka Streams topology | `kafka/streams` |

---

## 3. End Turn ve Event Akisi

Oyuncu `End Turn` butonuna bastiginda:

```mermaid
sequenceDiagram
    autonumber
    actor Player as Browser
    participant Nginx as nginx
    participant API as Go API
    participant Loop as Engine Loop
    participant TP as TurnProcessor.ProcessTurn
    participant Cache as In-Memory Cache
    participant Session as game.session
    participant Broadcast as game.broadcast
    participant Router as EventRouter
    participant SSE as GET /events SSE

    Player->>Nginx: POST /game/advance-turn
    Nginx->>API: reverse proxy
    API->>Loop: manual advance signal
    Loop->>TP: ProcessTurn(state, orders)

    Note over TP: 13-step deterministic pipeline
    TP-->>Loop: generated GameEvent list

    Loop->>Cache: apply snapshot / events
    Loop->>Session: produce latest WorldStateSnapshot
    Loop->>Broadcast: produce global events
    Loop->>Router: route side-specific events
    Router->>SSE: fan-out permitted events
    SSE-->>Player: event: message / data: JSON
```

`ProcessTurn` icindeki 13 adim:

```mermaid
flowchart TD
    A["1. Collect validated orders"]
    B["2. Assign / redirect routes"]
    C["3. Block / search paths"]
    D["4. Reinforce / deploy"]
    E["5. Fortify"]
    F["6. Maia abilities"]
    G["7. Auto-advance units"]
    H["8. Resolve combat"]
    I["9. Update path timers"]
    J["10. Update fortification timers"]
    K["11. Respawn + cooldown"]
    L["12. Ring detection"]
    M["13. Check win conditions"]
    N["Emit WorldStateSnapshot"]

    A --> B --> C --> D --> E --> F --> G --> H --> I --> J --> K --> L --> M --> N
```

Sunumda soyle:

```text
Turn processing deterministic bir pipeline. Emir verildigi anda unit aniden hareket etmiyor. End Turn sonrasi ayni 13 adim her seferinde ayni sirayla calisiyor. Bu siralama movement, combat, detection ve win-condition davranisini test edilebilir hale getiriyor.
```

Ilgili kod:

```text
option-b/internal/game/turn.go
```

---

## 4. SSE ve Information Hiding

Ring Bearer konumu iki oyuncuya ayni sekilde dagitilmaz.

```mermaid
flowchart LR
    TP["TurnProcessor<br/>GameEvent list"]
    POS["game.ring.position<br/>RingBearerMoved"]
    DET["game.ring.detection<br/>Detected / Spotted"]
    BC["game.broadcast<br/>WorldStateSnapshot"]
    ROUTER["EventRouter"]
    STRIP["StripRingBearerFromState<br/>currentRegion = empty"]
    LIGHT["Light SSE Channel"]
    DARK["Dark SSE Channel"]
    LB["Light Browser"]
    DB["Shadow Browser"]

    TP --> POS
    TP --> DET
    TP --> BC

    POS --> ROUTER
    DET --> ROUTER
    BC --> ROUTER

    ROUTER -->|"Light-only position"| LIGHT
    ROUTER -->|"Shadow detection only"| DARK
    ROUTER -->|"full snapshot"| LIGHT
    ROUTER --> STRIP
    STRIP -->|"sanitized snapshot"| DARK

    LIGHT --> LB
    DARK --> DB
```

Anlat:

```text
Information hiding frontend CSS ile yapilmiyor. Backend EventRouter Ring Bearer position eventini sadece Light SSE kanalina yollar. Broadcast snapshot Shadow tarafina giderken Ring Bearer currentRegion alani temizlenir. Shadow ancak detection event'i olusursa izin verilen bilgiyi alir.
```

Ilgili kod:

| Sorumluluk | Dosya |
| --- | --- |
| Event fan-out | `option-b/internal/router/event_router.go` |
| State serialization | `option-b/internal/api/server.go` |
| Side-specific view cache | `option-b/internal/cache/cache.go` |
| Browser SSE client | `index.html` |

---

## 5. Kafka Topic Haritasi

```mermaid
flowchart LR
    API["Go API"]
    RAW["game.orders.raw"]
    STREAMS["Kafka Streams<br/>validation + enrichment"]
    VALID["game.orders.validated"]
    DLQ["game.dlq"]
    ENGINE["Go TurnProcessor"]
    UNIT["game.events.unit"]
    REGION["game.events.region"]
    PATH["game.events.path"]
    POSITION["game.ring.position"]
    DETECTION["game.ring.detection"]
    SESSION["game.session<br/>log-compacted snapshot"]
    BROADCAST["game.broadcast<br/>snapshot + GameOver"]
    ROUTER["EventRouter + SSE"]

    API --> RAW
    RAW --> STREAMS
    STREAMS -->|"valid"| VALID
    STREAMS -->|"invalid"| DLQ
    VALID --> ENGINE

    ENGINE --> UNIT
    ENGINE --> REGION
    ENGINE --> PATH
    ENGINE --> POSITION
    ENGINE --> DETECTION
    ENGINE --> SESSION
    ENGINE --> BROADCAST

    UNIT --> ROUTER
    REGION --> ROUTER
    PATH --> ROUTER
    POSITION --> ROUTER
    DETECTION --> ROUTER
    BROADCAST --> ROUTER
```

Kisa aciklama:

```text
Topicler sorumluluklara gore ayrildi. Bu sayede raw order, validated order, invalid order, unit event, region event, path event, gizli Ring Bearer eventleri ve session snapshotlari birbirinden bagimsiz takip edilebiliyor.
```

---

## 6. Fault Tolerance ve Session Replay

```mermaid
flowchart LR
    B["Browser"]
    LB["nginx Load Balancer"]
    E1["go-engine-1"]
    E2["go-engine-2<br/>STOPPED"]
    E3["go-engine-3"]
    K["Kafka Cluster"]
    S["game.session<br/>log-compacted latest snapshot"]
    ER["go-engine-2<br/>RESTARTED"]

    B -->|"HTTP / SSE"| LB
    LB -->|"healthy upstream"| E1
    LB -.->|"unavailable"| E2
    LB -->|"healthy upstream"| E3

    E1 <-->|"produce / consume"| K
    E3 <-->|"produce / consume"| K
    K --> S
    S -->|"latest snapshot replay"| ER
```

Anlat:

```text
Bir Go engine durdugunda nginx kalan saglikli upstream instance'lara route etmeye devam eder. Kafka event ve session topiclerini saklar. game.session log-compacted topic oldugu icin latest snapshot tutulur. Restart olan engine bu snapshot ile state'e yeniden yaklasir.
```

Dikkatli ifade:

```text
Bu term-project demo seviyesinde fault tolerance kanitidir. Exhaustive production crash-matrix ve her side-effect boundary icin tam replay testi ayrica production hardening kapsamindadir.
```

---

## 7. GameOver Exactly-Once Akisi

```mermaid
sequenceDiagram
    autonumber
    participant TP as TurnProcessor
    participant Guard as GameOver Duplicate Guard
    participant Producer as Transactional Kafka Producer
    participant Topic as game.broadcast
    participant Consumer as read_committed Consumer

    TP->>Guard: markGameOver(winner, cause)
    Guard->>Guard: deterministic eventId
    Guard->>Producer: produce GAME_OVER
    Producer->>Topic: begin / commit transaction
    Topic-->>Consumer: expose committed GAME_OVER once

    TP->>Guard: extra advance after game end
    Guard-->>TP: no new GAME_OVER
```

Smoke testler:

```powershell
.\scripts\check-gameover-idempotency.ps1
.\scripts\check-gameover-idempotency-dark.ps1
```

Anlat:

```text
Light ve Dark victory senaryolari ayri smoke testlerle oynatiliyor. read_committed consumer GAME_OVER kaydinin sadece bir arttigini kontrol ediyor. Oyun bittikten sonra ekstra advance-turn cagrilari yeni GameOver uretmiyor.
```

---

## 8. Sunumda Kullanilacak En Kisa Diyagram Sirasi

Zaman azsa su uc diyagrami goster:

1. `Sistem Mimarisi`
2. `Player Order Akisi`
3. `SSE ve Information Hiding`

Biraz daha vaktin varsa:

4. `End Turn ve Event Akisi`
5. `Fault Tolerance ve Session Replay`
6. `GameOver Exactly-Once Akisi`

