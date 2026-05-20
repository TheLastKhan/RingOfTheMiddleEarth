# Ring of the Middle Earth - Calistirma Rehberi

Bu rehber projeyi temiz bir sekilde baslatmak, test etmek ve demo oncesi kontrol etmek icindir.

## 1. On Gereksinimler

Gerekli araclar:

- Docker Desktop
- Docker Compose v2
- Go 1.25+ Go testleri icin
- Git

Docker Desktop acik olmali. Kontrol:

```powershell
docker --version
docker compose version
```

## 2. Projeyi Baslatma

```powershell
cd C:\Users\hakan\termproject
docker compose up -d --build
```

Ilk calistirmada image indirme ve Maven/Go build surebilir.

## 3. Servisleri Kontrol Etme

```powershell
docker ps
```

Beklenen ana containerlar:

- `rotr-zookeeper`
- `rotr-kafka-1`
- `rotr-kafka-2`
- `rotr-kafka-3`
- `rotr-schema-registry`
- `rotr-kafka-streams`
- `rotr-go-1`
- `rotr-go-2`
- `rotr-go-3`
- `rotr-nginx`
- `rotr-ui`

Health check:

```powershell
Invoke-RestMethod http://localhost/health
```

Beklenen:

```json
{
  "status": "ok"
}
```

## 4. Oyunu Acma

Light side:

```text
http://localhost:3000?side=light
```

Dark side:

```text
http://localhost:3000?side=dark
```

Iki farkli tarayici sekmesi acarak iki tarafi ayni anda gosterebilirsin.

## 5. Testler

Go testleri:

```powershell
cd C:\Users\hakan\termproject\option-b
go test ./...
cd ..
```

Kafka Streams validation testleri:

```powershell
.\scripts\demo-validation-k4.ps1
```

Beklenen Maven sonucu:

```text
Tests run: 9, Failures: 0, Errors: 0, Skipped: 0
BUILD SUCCESS
```

Tam E2E smoke testi:

```powershell
.\scripts\full-e2e-smoke.ps1
```

Bu test yeni oyun baslatir, 3 Go engine'in `game.session` uzerinden ayni state'e geldigini kontrol eder, duplicate order concurrency denemesi yapar, turn ilerletir, bir engine'i durdurup oyunun nginx uzerinden devam ettigini dogrular, engine'i geri baslatir ve SSE event akisini kontrol eder.

Transactional GameOver exactly-once testi:

```powershell
.\scripts\check-gameover-idempotency.ps1
```

Bu test Light victory senaryosunu oynatir, `read_committed` Kafka consumer ile `game.broadcast` icindeki GameOver sayisinin sadece 1 arttigini ve oyun bittikten sonra ekstra turn advance cagrilarinin yeni GameOver uretmedigini dogrular.

## 6. Schema Registry Kontrolu

```powershell
Invoke-RestMethod http://localhost:8081/subjects
```

Beklenen subject sayisi: 10.

Surum kontrolleri:

```powershell
Invoke-RestMethod http://localhost:8081/subjects/game.orders.validated-value/versions
Invoke-RestMethod http://localhost:8081/subjects/game.session-value/versions
```

Beklenen:

- `game.orders.validated-value`: `1, 2`
- `game.session-value`: `1`

## 7. Analiz Endpointleri

Route risk:

```powershell
Invoke-RestMethod http://localhost/analysis/routes
```

Intercept:

```powershell
Invoke-RestMethod http://localhost/analysis/intercept
```

Ikisi de bos/null degil, dolu veri donmeli.

## 8. pprof / Goroutine Kontrolu

```powershell
Invoke-RestMethod "http://localhost/debug/pprof/goroutine?debug=1"
```

Uzun test icin:

```powershell
.\scripts\check-pprof-10turns.ps1
```

Bu script baslangic ve bitis goroutine toplamlarini yazar.

## 9. Fault Tolerance Testi

En kapsamli test:

```powershell
.\scripts\full-e2e-smoke.ps1
```

Manuel kontrol icin bir Go instance durdur:

```powershell
docker compose stop go-engine-2
```

Nginx uzerinden saglik kontrolu:

```powershell
Invoke-RestMethod http://localhost/health
```

Hala `ok` donmeli.

Oyun state recovery kontrolu icin turn bilgisini dogrula:

```powershell
Invoke-RestMethod "http://localhost:8080/game/state?playerId=light-player&side=FREE_PEOPLES"
Invoke-RestMethod "http://localhost:8082/game/state?playerId=light-player&side=FREE_PEOPLES"
Invoke-RestMethod "http://localhost:8083/game/state?playerId=light-player&side=FREE_PEOPLES"
```

Geri baslat:

```powershell
docker compose up -d go-engine-2
```

## 10. Eski UI Gorunurse

```powershell
docker compose up -d --build ui go-engine-1 go-engine-2 go-engine-3 kafka-streams
```

Tarayicida hard refresh:

```text
Ctrl + F5
```

## 11. Kapatma

Sadece durdur:

```powershell
docker compose down
```

Volume dahil sifirla:

```powershell
docker compose down -v
```

## 12. Demo Oncesi Hizli Kontrol

```powershell
cd C:\Users\hakan\termproject
docker compose up -d --build
docker ps
Invoke-RestMethod http://localhost/health
Invoke-RestMethod http://localhost:8081/subjects
Invoke-RestMethod http://localhost/analysis/routes
Invoke-RestMethod http://localhost/analysis/intercept
go test -C option-b ./...
.\scripts\demo-validation-k4.ps1
.\scripts\full-e2e-smoke.ps1
.\scripts\check-gameover-idempotency.ps1
```

Not: Go surumun `go test -C option-b ./...` desteklemiyorsa klasik sekilde `cd option-b; go test ./...; cd ..` kullan.
