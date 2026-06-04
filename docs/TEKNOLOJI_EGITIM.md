# Ring of the Middle Earth - Teknoloji Egitimi

Bu belge projedeki teknolojileri sade sekilde anlatir. Resmi gereksinim dosyasi
`TermProject_RingOfTheMiddleEarth.md` degistirilmeden tutulur; bu dosya ise
calisan uygulamayi anlamak icindir.

## Sistem Ozeti

Proje tarayici tabanli, tur bazli bir strateji oyunudur. Asil teknik amac,
oyunu dagitik bir sistem olarak calistirmaktir.

Akis:

```text
Browser UI
  -> nginx
  -> 3 Go engine instance
  -> Kafka topics
  -> Kafka Streams validation/enrichment
  -> Schema Registry
  -> SSE ile tekrar browser
```

## Kafka Temelleri

Kafka, servisler arasinda kalici event log gorevi gorur.

- Producer: topic'e mesaj yazar.
- Consumer: topic'ten mesaj okur.
- Topic: mesaj kategorisi.
- Partition: paralel okuma/yazma icin topic parcasi.
- Consumer group: birden fazla consumer'in partition'lari paylasmasi.
- Schema Registry: Avro semalarini merkezi olarak tutar.

Bu projedeki ana topic'ler:

| Topic | Amac |
| --- | --- |
| `game.orders.raw` | Oyuncudan gelen ham emirler |
| `game.orders.validated` | Dogrulanmis ve zenginlestirilmis emirler |
| `game.events.unit` | Birim olaylari |
| `game.events.region` | Bolge olaylari |
| `game.events.path` | Yol olaylari |
| `game.session` | Compacted son oyun snapshot'i |
| `game.broadcast` | UI icin dunya snapshot/event yayini |
| `game.ring.position` | Light-only Ring Bearer konumu |
| `game.ring.detection` | Dark taraf detection bilgisi |
| `game.dlq` | Gecersiz emirler |

## Kafka Streams

Kafka Streams Java tarafinda calisir.

Topology 1: order validation

- `game.orders.raw` okur.
- 8 validation kuralini uygular.
- Gecerli emirleri `game.orders.validated` topic'ine yazar.
- Gecersiz emirleri DLQ mantigiyla ayirir.

Topology 2: route-risk enrichment

- Validated route emirlerini ve world-state bilgisini kullanir.
- `routeRiskScore`, `threatenedPaths`, `blockedPaths` gibi V2 alanlarini ekler.
- Schema evolution icin yeni alanlar nullable/default degerlidir.

Not: Kafka Streams `EXACTLY_ONCE_V2` ile calisir. Go tarafinda normal eventler
hafif producer ile yazilir; GameOver gibi terminal eventler ise franz-go
tabanli transactional producer ile `read_committed` olarak dogrulanir.

## Go Tarafi

Go engine su islerden sorumludur:

- HTTP API
- SSE fan-out
- Side-specific `/game/state`
- Order intake
- Turn processor
- Combat/path/Maia/detection kurallari
- Route/intercept analysis
- In-memory serving cache
- pprof endpointleri

Onemli Go kavramlari:

- Goroutine: hafif paralel is birimi.
- Channel: goroutine'ler arasi guvenli mesaj kanali.
- Select loop: birden fazla channel/timer/signal'i ayni anda dinleme.
- Context: iptal ve timeout mekanizmasi.

## SSE

SSE, server'dan browser'a tek yonlu canli event akisidir.

UI `/events` endpoint'ine baglanir. Go engine, gelen olaylari Light veya Dark
tarafa gore filtreler. Bu sayede bilgi asimetrisi korunur.

## Bilgi Asimetrisi

Oyunun kritik kuralidir:

- Light taraf Ring Bearer konumunu bilir.
- Dark taraf Ring Bearer konumunu dogrudan bilmez.
- Dark taraf sadece detection event'leri ile sinirli bilgi alir.

Bu davranis router ve API state serializer tarafinda uygulanir.

## Docker ve nginx

Docker Compose tum sistemi tek komutla ayaga kaldirir:

```powershell
docker compose up -d --build
```

nginx, HTTP ve SSE isteklerini 3 Go engine arasinda dagitir. Bir Go instance
durursa kalan instance'lar uzerinden `/health` cevap vermeye devam eder.

## Fault Tolerance

Demo seviyesinde fault tolerance:

- 3 Go engine calisir.
- nginx kalan instance'lara route eder.
- Kafka event/order/session topic'lerini saklar.
- `game.session` compacted topic son snapshot'i tutar.

Sinir:

- Runtime serving cache bellektedir.
- `game.session` replay ile engine restart/failover demo seviyesinde toparlanir.
- Tum olasi crash noktalarini kapsayan exhaustive kaos matrisi production hardening olarak kalir; repo icinde kisa ve uzatilabilir chaos/soak smoke testi vardir: `scripts/chaos-soak-smoke.ps1`.

## Testler

Go testleri:

```powershell
cd C:\Users\hakan\termproject\option-b
go test ./...
```

Kafka Streams validation testi:

```powershell
cd C:\Users\hakan\termproject
.\scripts\demo-validation-k4.ps1
```

Schema kontrolu:

```powershell
Invoke-RestMethod http://localhost:8081/subjects
Invoke-RestMethod http://localhost:8081/subjects/game.orders.validated-value/versions
```

pprof kontrolu:

```powershell
Invoke-RestMethod "http://localhost/debug/pprof/goroutine?debug=1"
```

Fault tolerance smoke test:

```powershell
docker compose stop go-engine-2
Invoke-RestMethod http://localhost/health
docker compose up -d go-engine-2
```
