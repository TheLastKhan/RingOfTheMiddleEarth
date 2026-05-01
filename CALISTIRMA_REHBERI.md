# 🏔️ Ring of the Middle Earth — Adım Adım Çalıştırma Rehberi

---

## 📋 ÖN GEREKSINIMLER

Projeyi çalıştırmadan önce şunları kur:

### 1. Docker Desktop (ZORUNLUubr)
```
İndir: https://www.docker.com/products/docker-desktop/
```
- Windows'ta WSL2 backend gerekli
- Kurulumdan sonra bilgisayarı yeniden başlat
- Docker Desktop'u aç ve çalıştığını doğrula:
```powershell
docker --version
# Docker version 24.x.x
docker compose version
# Docker Compose version v2.x.x
```

### 2. Go 1.22+ (Test için)
```
İndir: https://go.dev/dl/
```
```powershell
go version
# go1.22.x windows/amd64
```

### 3. Git (Opsiyonel)
```powershell
git --version
```

---

## 🚀 ADIM ADIM ÇALIŞTIRMA

### Adım 1: Proje dizinine git
```powershell
cd c:\Users\hakan\termproject
```

### Adım 2: Önce testleri çalıştır (Docker olmadan)
```powershell
cd option-b
go test -v ./...
```

**Beklenen çıktı:**
```
=== RUN   TestCombat_PlainsTie
--- PASS: TestCombat_PlainsTie (0.00s)
=== RUN   TestCombat_FortressDefense
--- PASS: TestCombat_FortressDefense (0.00s)
=== RUN   TestCombat_IgnoresFortress
--- PASS: TestCombat_IgnoresFortress (0.00s)
=== RUN   TestCombat_IgnoresFortressButFortified
--- PASS: TestCombat_IgnoresFortressButFortified (0.00s)
=== RUN   TestCombat_LeadershipBonus
--- PASS: TestCombat_LeadershipBonus (0.00s)
=== RUN   TestCombat_Indestructible
--- PASS: TestCombat_Indestructible (0.00s)
ok      rotr/internal/game

=== RUN   TestRouter_DarkSideStripped
--- PASS: TestRouter_DarkSideStripped (0.00s)
=== RUN   TestRouter_RingBearerMovedNeverDarkSide
--- PASS: TestRouter_RingBearerMovedNeverDarkSide (0.00s)
=== RUN   TestRouter_CacheUpdateNeverExposesRingBearer
--- PASS: TestRouter_CacheUpdateNeverExposesRingBearer (0.00s)
ok      rotr/internal/router

=== RUN   TestPipeline1_KnownRiskScore
--- PASS: TestPipeline1_KnownRiskScore (0.00s)
=== RUN   TestPipeline1_NazgulProximity
--- PASS: TestPipeline1_NazgulProximity (0.00s)
=== RUN   TestPipeline2_PositiveIntercept
--- PASS: TestPipeline2_PositiveIntercept (0.00s)
=== RUN   TestPipeline2_NegativeIntercept
--- PASS: TestPipeline2_NegativeIntercept (0.00s)
ok      rotr/internal/pipeline
```
**13 test, hepsi PASS.**

### Adım 3: Docker ile tüm sistemi başlat
```powershell
cd c:\Users\hakan\termproject
docker compose up -d --build
```

Bu komut şunları yapar (ilk seferde 5-10 dakika sürer):
1. Zookeeper'ı başlatır
2. 3 Kafka broker'ı başlatır
3. Schema Registry'yi başlatır
4. Kafka Init: 10 topic oluşturur + Avro schema'ları kaydeder
5. Kafka Streams Java uygulamasını başlatır
6. 3 Go game engine instance'ını derleyip başlatır
7. Nginx load balancer'ı başlatır
8. UI'ı başlatır

### Adım 4: Servisleri kontrol et
```powershell
docker compose ps
```

**Beklenen çıktı (tüm servisler "running"):**
```
NAME                  STATUS
rotr-zookeeper        running
rotr-kafka-1          running
rotr-kafka-2          running
rotr-kafka-3          running
rotr-schema-registry  running
rotr-kafka-init       exited (0)  ← run-once, normal
rotr-kafka-streams    running
rotr-go-1             running
rotr-go-2             running
rotr-go-3             running
rotr-nginx            running
rotr-ui               running
```

### Adım 5: Health check
```powershell
curl http://localhost:8080/health
```
```json
{"status":"ok","turn":0,"timestamp":1712480000}
```

### Adım 6: Tarayıcıda aç
- **UI**: http://localhost:3000
- **Light Side**: http://localhost:3000?side=light
- **Dark Side**: http://localhost:3000?side=dark

### Adım 7: Logları izle
```powershell
# Tüm servisler
docker compose logs -f

# Sadece Go engine
docker compose logs -f go-engine-1

# Sadece Kafka Streams
docker compose logs -f kafka-streams
```

### Adım 8: Fault tolerance testi
```powershell
# Bir Go instance'ı durdur
docker compose stop go-engine-2

# 10 saniye bekle, sonra kontrol et
curl http://localhost:8080/health
# Hala çalışıyor!

# Geri başlat
docker compose start go-engine-2
```

### Adım 9: Kapatma
```powershell
docker compose down -v
```

---

## 🔧 SORUN GİDERME

| Sorun | Çözüm |
|-------|-------|
| Docker Desktop yok | https://docker.com dan indir |
| Port 80 kullanımda | Docker Desktop'tan kullanılabilir portları kontrol et |
| Kafka başlatma hatası | `docker compose down -v` sonra tekrar `docker compose up -d --build` |
| Go build hatası | `cd option-b && go vet ./...` ile kontrol et |
| Memory yetersiz | Docker Desktop Settings → Resources → RAM'i 6GB+ yap |

---

## 📊 SERVİS PORTLARI

| Servis | Port | URL |
|--------|------|-----|
| UI | 3000 | http://localhost:3000 |
| Game Engine (Nginx) | 80 | http://localhost:80 |
| Go Engine 1 | 8080 | http://localhost:8080 |
| Go Engine 2 | 8082 | http://localhost:8082 |
| Go Engine 3 | 8083 | http://localhost:8083 |
| Schema Registry | 8081 | http://localhost:8081 |
| Kafka 1/2/3 | 9092/9093/9094 | — |
| Zookeeper | 2181 | — |
