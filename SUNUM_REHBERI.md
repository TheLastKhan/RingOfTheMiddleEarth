# Ring of the Middle Earth - Sunum ve Demo Rehberi

Bu belge demo gunu icin kisa ve gercekci akistir. Proje teknoloji secimi:
**Option B - Go + Kafka**.

## Demo Oncesi Kontrol

```powershell
cd C:\Users\hakan\termproject
docker compose up -d --build
docker ps
Invoke-RestMethod http://localhost/health
Invoke-RestMethod http://localhost:8081/subjects

cd option-b
go test ./...
cd ..

.\scripts\demo-validation-k4.ps1
```

Production hardening smoke gostermek istersen:

```powershell
.\scripts\chaos-soak-smoke.ps1
.\scripts\browser-smoke.ps1
```

Beklenen sonuc:

- Tum ana container'lar `Up` gorunur.
- `/health` `ok` doner.
- Schema Registry 10 subject listeler.
- Go testleri gecer.
- Kafka Streams validation testi: 9 test, 0 failure.

## 15 Dakikalik Sunum Akisi

1. Mimariyi anlat: UI -> nginx -> 3 Go engine -> Kafka -> Kafka Streams -> Schema Registry.
2. Demo 1: Light/Dark bilgi gizleme.
3. Demo 2: validation, route risk, intercept ve Maia/path mekanikleri.
4. Demo 3: fault tolerance.
5. Kisa Q&A.

## Demo 1 - Information Hiding

Iki tarayici ac:

- Light: `http://localhost:3000?side=light`
- Dark: `http://localhost:3000?side=dark`

Gosterilecek fikir:

- Light taraf Ring Bearer konumunu gorur.
- Dark taraf Ring Bearer konumunu dogrudan gormez.
- Dark sadece detection bilgisi gelirse bilgi alir.

API ile gostermek istersen:

```powershell
Invoke-RestMethod "http://localhost/game/state?playerId=light-player&side=FREE_PEOPLES"
Invoke-RestMethod "http://localhost/game/state?playerId=dark-player&side=SHADOW"
```

Kod referansi:

- `option-b/internal/router/event_router.go`
- `option-b/internal/api/server.go`

## Demo 2 - Validation ve Analysis

```powershell
.\scripts\demo-validation-k4.ps1
Invoke-RestMethod http://localhost/analysis/routes
Invoke-RestMethod http://localhost/analysis/intercept
```

Anlatilacak noktalar:

- Kafka Streams tarafinda 8 validation kuralinin testleri var.
- Gecersiz emirler DLQ mantigiyla ayriliyor.
- Route analysis Light icin rota riskini hesapliyor.
- Intercept analysis Dark icin yakalama planini hesapliyor.
- `game.orders.validated-value` schema V1 ve V2 olarak kayitli.

## Demo 3 - Fault Tolerance

```powershell
docker compose stop go-engine-2
for ($i=1; $i -le 5; $i++) { Invoke-RestMethod http://localhost/health }
docker compose up -d go-engine-2
```

Anlatilacak noktalar:

- nginx kalan Go engine instance'larina route etmeye devam eder.
- Kafka topic'leri order/event/session kayitlarini tutar.
- `game.session` compacted topic olarak son snapshot'i saklar.

## Q&A Kisa Cevaplar

**Hardcoding var mi?**
Oyun davranislari config ve mevcut state uzerinden okunuyor. Validation testlerinde fixture verisi var; bu test verisi, oyun mantigini hardcode etmek anlamina gelmez.

**Information asymmetry nerede uygulanir?**
Event router ve API serializer tarafinda. Light gercek Ring Bearer konumunu alabilir; Dark tarafindan bu alan gizlenir.

**Kafka Streams tam olarak ne yapiyor?**
Topology 1 raw order validation yapar. Topology 2 route emirlerine route-risk enrichment ekler. Streams ayarlari `EXACTLY_ONCE_V2` kullanir.

**Exactly-once iddiasi ne kadar guclu?**
Kafka Streams tarafinda exactly-once v2 kullaniliyor. Go tarafinda GameOver, franz-go tabanli transactional producer ile `game.broadcast` topic'ine yaziliyor. `scripts/check-gameover-idempotency.ps1` Light victory icin, `scripts/check-gameover-idempotency-dark.ps1` Dark victory icin `read_committed` consumer ile GameOver sayisinin sadece 1 arttigini ve ekstra turn advance sonrasi tekrar uretim olmadigini dogrular.

**State tamamen Kafka'dan mi rebuild oluyor?**
Demo icin `game.session` snapshot'i var ve state Kafka'da gorunur. Go engine runtime cache'i hala bellekte calisir; restart sonrasi tam world-state replay production seviyesinde degil.

**pprof nereden gosterilir?**

```powershell
Invoke-RestMethod "http://localhost/debug/pprof/goroutine?debug=1"
```

**Demo sirasinda en guvenli siralama nedir?**

1. `/health`
2. UI Light/Dark
3. `/analysis/routes`
4. `/analysis/intercept`
5. `demo-validation-k4.ps1`
6. `go-engine-2` stop/start fault tolerance
