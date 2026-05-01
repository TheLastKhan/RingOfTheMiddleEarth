# 🎓 Ring of the Middle Earth — Teknolojiler Temelden Anlatım

Bu belge, projede kullanılan tüm teknolojileri **en temelden** anlatır.
Amacı: projenin HER satırını anlaman ve hocana güvenle anlatabilmen.

---

## 📚 BÖLÜM 1: TEMEL KAVRAMLAR

---

### 1.1 Distributed System (Dağıtık Sistem) Nedir?

**Basit tanım:** Birden fazla bilgisayarın (veya process'in) bir arada çalışarak
tek bir iş yapmasıdır.

**Günlük hayat örneği:**
Bir pizza zinciri düşün. Merkez mutfak yerine 5 şubede pizza yapılıyor.
- Bir şube kapansa, diğer 4 şube devam eder (fault tolerance)
- Siparişler şubeler arası paylaştırılır (load balancing)
- Her şube aynı menüyü kullanır (shared state)

**Bu projede:**
```
Tarayıcı (UI) → Nginx → Go Engine (3 kopya) → Kafka → Go Engine → Tarayıcı
```
3 Go engine instance'ı var. Biri çökse → diğer 2 devam eder.

---

### 1.2 Message Queue / Event Streaming Nedir?

**Basit tanım:** Uygulamalar arası mesaj taşıyan bir kuyruk sistemi.

**Günlük hayat örneği:**
Bir restoranda garson (producer) siparişi bir parşömene yazar ve mutfak tezgahına
koyar (queue). Aşçı (consumer) sırayla alır ve yapar. Garson aşçının ne zaman
yapacağını bilmek zorunda değildir.

**Normal HTTP vs Event Streaming:**
```
HTTP (senkron):
  Tarayıcı → "Sipariş ver" → Server → "Tamam" → Tarayıcı
  Server cevap verene kadar tarayıcı BEKLİYOR

Event Streaming (asenkron):
  Tarayıcı → "Sipariş ver" → Kafka Queue → "202 Kabul edildi"
  ...
  Go Engine → Kafka Queue'dan siparişi alır → İşler → Sonucu yayınlar
  Tarayıcı BEKLEMEZ, sonuç hazır olunca SSE ile bildirilir
```

---

### 1.3 Stateless vs Stateful

**Stateful (durumlu):**
Server kendi belleğinde veri tutar. Çökerse veri kaybolur.
```
Go instance 1: "Ring Bearer Bree'de"  ← bellekte
Go instance 1 çöktü!
Veri kayboldu! 😱
```

**Stateless (durumsuz) — BU PROJE:**
Server hiçbir veri tutmaz. Tüm veri Kafka'da.
```
Go instance 1: Kafka'dan oku → "Ring Bearer Bree'de" → işle
Go instance 1 çöktü!
Go instance 2: Kafka'dan tekrar oku → "Ring Bearer Bree'de" → devam
Veri KAFKA'DA güvende! 😊
```

---

## 📚 BÖLÜM 2: KAFKA

---

### 2.1 Kafka Nedir?

Apache Kafka = **dağıtık event streaming platformu**.

Düşün ki büyük bir gazete matbaası var:
- **Producer**: Haberci (haber yazar, gazeteye gönderir)
- **Broker**: Matbaa (haberleri saklar, sırayla basar)
- **Consumer**: Okuyucu (gazeteden haberleri okur)
- **Topic**: Gazete bölümü (Spor, Ekonomi, Siyaset)

```
Producer → Topic → Broker → Consumer
  (yaz)    (bölüm)  (sakla)    (oku)
```

### 2.2 Topic ve Partition

**Topic** = Mesajların kategorisi. Bu projede 10 topic var:

```
game.orders.raw        → Oyuncuların gönderdiği emirler
game.orders.validated  → Doğrulanmış emirler
game.events.unit       → Birim hareketleri
game.events.region     → Bölge kontrol değişiklikleri
game.events.path       → Yol durumu değişiklikleri
game.session           → Oyun durumu (compacted)
game.broadcast         → Dünya durumu yayını
game.ring.position     → Ring Bearer konumu (SADECE Light Side)
game.ring.detection    → Ring Bearer tespiti (SADECE Dark Side)
game.dlq               → Hatalı emirler (Dead Letter Queue)
```

**Partition** = Topic'in parçaları. Paralellik sağlar.
```
game.orders.raw — 3 partition:
  [Partition 0] → Go-1 okur
  [Partition 1] → Go-2 okur
  [Partition 2] → Go-3 okur

3 Go instance, 3 partition = her biri bir parçayı okur
Bir instance çökerse → kalan 2, 3 partition'ı paylaşır (rebalance)
```

### 2.3 Consumer Group

3 Go instance aynı **consumer group**'a ait:
```
Consumer Group: "rotr-engine-group"
  ├── Go-1: Partition 0, 1 okur
  ├── Go-2: Partition 2 okur
  └── Go-3: (yedek, boşta)

Go-2 çöktü!
  ├── Go-1: Partition 0 okur
  └── Go-3: Partition 1, 2 okur ← REBALANCE
```

Bu mekanizma sayesinde **fault tolerance** (hata toleransı) sağlanır.

### 2.4 Avro Schema ve Schema Registry

**Avro** = Mesaj formatı (JSON'a benzer ama binary, daha hızlı).

```json
{
  "type": "record",
  "name": "OrderSubmitted",
  "fields": [
    {"name": "playerId",  "type": "string"},
    {"name": "unitId",    "type": "string"},
    {"name": "orderType", "type": "string"},
    {"name": "turn",      "type": "int"}
  ]
}
```

**Schema Registry** = Tüm şemaları merkezi olarak saklayan servis.
Producer mesaj göndermeden önce şemayı kontrol eder → format uyumsuzluğu önlenir.

**Schema Evolution (V2):**
```
V1: {playerId, unitId, orderType, turn}
V2: {playerId, unitId, orderType, turn, routeRiskScore?: int}
                                         ↑ yeni alan, nullable
```
V1 okuyucu V2 mesajını okuyabilir → routeRiskScore'u yok sayar. **Backward compatible!**

### 2.5 Kafka Streams

**Kafka Streams** = Kafka üzerinde çalışan gerçek zamanlı veri işleme kütüphanesi.

```
Topology 1 (Order Validation):
  game.orders.raw → [8 kuraldan geçir] → 
    Geçenler → game.orders.validated ✅
    Kalanlar → game.dlq ❌ (Dead Letter Queue)

Topology 2 (Route Risk):
  game.orders.validated + game.broadcast (KTable) →
    [risk score hesapla] → game.orders.validated (V2 ile)
```

**KTable** nedir?
```
Normal stream: her mesaj bir EVENT (olay)
  t1: Ring Bearer Shire'da
  t2: Ring Bearer Bree'de
  t3: Ring Bearer Weathertop'ta

KTable: son durumu tutar (state)
  Ring Bearer → Weathertop (en son değer)

Fark: Stream = geçmiş, KTable = şimdi
```

### 2.6 Dead Letter Queue (DLQ)

Hatalı mesajları YOK ETMEK yerine saklayan özel topic.
```
Oyuncu hatalı emir gönderdi:
  {"unitId": "gandalf", "orderType": "MAIA_ABILITY", "turn": 5}
  
  Kural 7: Gandalf cooldown'da → HATA!
  
  DLQ'ya yazılır:
  {
    "errorCode": "ABILITY_ON_COOLDOWN",
    "errorMessage": "Gandalf ability on cooldown for 2 more turns",
    "rawPayload": {...orijinal emir...},
    "timestamp": 1712480000
  }
```
Böylece hata analizi yapılabilir, hiçbir veri kaybolmaz.

### 2.7 Exactly-Once Semantics

**Problem:**
```
Go Engine → "GameOver: Light Side wins!" → Kafka
Ağ koptu! Mesaj gitti mi gitmedi mi?
Go Engine tekrar gönderir → İKİ KEZ "GameOver"??? 😱
```

**Çözüm: Idempotent Producer**
```
Producer: enable.idempotence = true
Kafka: Her mesaja sequence number atar
  Mesaj #42 geldi → Tamam, kaydettim
  Mesaj #42 tekrar geldi → Zaten var, atıyorum
  Sonuç: HER ZAMAN tam olarak 1 kez
```

---

## 📚 BÖLÜM 3: GO DİLİ

---

### 3.1 Go Nedir?

Google'ın 2009'da çıkardığı programlama dili. Özellikleri:
- **Basit**: Java'dan çok daha az syntax
- **Hızlı**: C/C++ seviyesine yakın performans
- **Concurrent**: Goroutine'ler ile binlerce paralel iş yapılabilir
- **Statically typed**: Derleme zamanında hata yakalama

### 3.2 Goroutine Nedir?

**Thread** = İşletim sistemi seviyesinde paralel iş birimi (ağır, ~1MB bellek)
**Goroutine** = Go'nun hafif thread'i (~2KB bellek, binlerce oluşturulabilir)

```go
// Normal (sıralı):
doWork1()  // 3 saniye
doWork2()  // 2 saniye
// Toplam: 5 saniye

// Goroutine ile (paralel):
go doWork1()  // arka planda başla
go doWork2()  // arka planda başla
// Toplam: ~3 saniye (en uzun)
```

Bu projede kullanımı:
```go
// Pipeline 1: 4 worker goroutine
for i := 0; i < 4; i++ {
    go func() {  // her biri ayrı goroutine
        for route := range inputCh {
            result := computeRisk(route)
            resultCh <- result
        }
    }()
}
```

### 3.3 Channel Nedir?

Goroutine'ler arası güvenli iletişim kanalı.

```go
// Buffered channel: 20 mesaj tutabilir
ch := make(chan Order, 20)

// Gönder (producer):
ch <- order  // channel'a koy

// Al (consumer):
order := <-ch  // channel'dan al

// Channel boşsa → consumer bekler
// Channel doluysa → producer bekler
```

**Bu projede:**
```go
LightSSECh  := make(chan Event, 100)  // Light Side SSE kanalı
DarkSSECh   := make(chan Event, 100)  // Dark Side SSE kanalı
OrderCh     := make(chan Order, 100)  // Emir kanalı
```

### 3.4 Select Statement

Birden fazla channel'ı AYNİ ANDA dinler. Hangisinde veri gelirse o çalışır:

```go
select {
case msg := <-kafkaCh:
    // Kafka'dan mesaj geldi → işle
case player := <-newConnectionCh:
    // Yeni oyuncu bağlandı → kaydet
case <-turnTimer.C:
    // Tur süresi doldu → yeni tur
case sig := <-signalCh:
    // Ctrl+C basıldı → kapat
}
```

Bu bir **event loop** — Node.js'deki event loop'a benzer,
ama type-safe ve compiler tarafından kontrol edilir.

### 3.5 sync.WaitGroup

"Tüm goroutine'ler BITMEDEN devam etme" demek:

```go
var wg sync.WaitGroup

for i := 0; i < 4; i++ {
    wg.Add(1)        // "1 iş daha var"
    go func() {
        defer wg.Done()  // "bu iş bitti"
        doWork()
    }()
}

wg.Wait()  // 4 iş de bitene kadar BURADA BEKLE
```

### 3.6 Context

İptal mekanizması — "2 saniye içinde bitmezse DURDUR":

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

select {
case result := <-resultCh:
    // Sonuç geldi
case <-ctx.Done():
    // 2 saniye doldu, timeout!
}
```

### 3.7 Race Condition ve -race Flag

**Race condition** = İki goroutine aynı değişkene aynı anda yazarsa:
```go
// YANLIŞ — race condition!
var count int
go func() { count++ }()  // goroutine 1
go func() { count++ }()  // goroutine 2
// count 1 mi, 2 mi? TANIMLANMAMIŞ!
```

`go test -race` bu hataları yakalar. Bu projede EventRouter'da
race condition olmadığını kanıtlamak için kullanılır.

---

## 📚 BÖLÜM 4: PROJE MİMARİSİ

---

### 4.1 Büyük Resim

```
┌──────────────┐     ┌──────────────┐
│ Light Side   │     │ Dark Side    │
│ Tarayıcı     │     │ Tarayıcı     │
└──────┬───────┘     └──────┬───────┘
       │  SSE                │  SSE
       ▼                     ▼
┌─────────────────────────────────────┐
│           NGINX (Load Balancer)      │
│         Round-robin → 3 instance     │
└──────────────┬──────────────────────┘
               │
    ┌──────────┼──────────┐
    ▼          ▼          ▼
┌────────┐ ┌────────┐ ┌────────┐
│ Go-1   │ │ Go-2   │ │ Go-3   │   ← Stateless
│        │ │        │ │        │
│ API    │ │ API    │ │ API    │
│ Router │ │ Router │ │ Router │   ← EventRouter
│ Cache  │ │ Cache  │ │ Cache  │   ← WorldStateCache
│Pipeline│ │Pipeline│ │Pipeline│   ← Route Risk / Intercept
└────┬───┘ └────┬───┘ └────┬───┘
     │          │          │
     ▼          ▼          ▼
┌─────────────────────────────────────┐
│          KAFKA CLUSTER               │
│  ┌─────────┐ ┌─────────┐ ┌────────┐│
│  │Broker 1 │ │Broker 2 │ │Broker 3││
│  └─────────┘ └─────────┘ └────────┘│
│                                      │
│  10 Topics, 3x replication           │
│  Schema Registry, KTables            │
└─────────────────────────────────────┘
         ▲
         │
┌────────┴──────────┐
│  Kafka Streams    │
│  (Java)           │
│  Topology 1: Validation
│  Topology 2: Risk Score
└───────────────────┘
```

### 4.2 Bilgi Akış Diyagramı

**Bir emir gönderildiğinde:**
```
1. Oyuncu → POST /order → Go Engine API
2. Go Engine → game.orders.raw → Kafka
3. Kafka Streams Topology 1 → 8 kural doğrula
   ├── Geçerli → game.orders.validated
   └── Geçersiz → game.dlq
4. Go Engine (consumer) ← game.orders.validated oku
5. TurnProcessor.ProcessTurn() → 13 adım işle
6. Sonuç event'leri → Kafka topics (unit, region, path, ring.*)
7. EventRouter.Route() → Light/Dark SSE kanallarına yönlendir
   ├── Light Side: tam bilgi (Ring Bearer konumu dahil)
   └── Dark Side: Ring Bearer konumu SİLİNMİŞ
8. SSE → Tarayıcıya push
```

### 4.3 Information Asymmetry (Bilgi Asimetrisi)

Bu projenin EN ÖNEMLİ konsepti:

```
Light Side bildiği şeyler:         Dark Side bildiği şeyler:
✅ Ring Bearer'ın gerçek konumu     ❌ Ring Bearer'ın konumu (her zaman "")
✅ Route risk analizi               ❌ Route risk analizi
❌ Detection bilgileri              ✅ Detection bilgileri
❌ Intercept planları               ✅ Intercept planları
```

**TEK enforcement point:** `EventRouter.Route()`

```go
func (r *EventRouter) Route(event Event) {
    switch event.Topic {
    case "game.ring.position":
        r.LightSSECh <- event      // ✅ Light'a gider
        // r.DarkSSECh ← ASLA!     // ❌ Dark'a ASLA gitmez
        
    case "game.broadcast":
        r.LightSSECh <- event              // ✅ Tam bilgi
        r.DarkSSECh <- stripRingBearer(event) // Ring Bearer silindi
    }
}
```

### 4.4 Config-Driven Design

Hiçbir game logic dosyasında birim ID'si yazılmaz:

```go
// ❌ YANLIŞ (hardcoding):
if unit.ID == "witch-king" {
    detectionRange = 2
}

// ✅ DOĞRU (config-driven):
if unit.Config.DetectionRange > 0 {
    // Bu bir Nazgul — DetectionRange config'den geliyor
}
```

**Neden?**
Yarın yeni bir Nazgul eklemek istersen:
1. `config/units.conf`'a yeni satır ekle
2. Hiçbir Go kodu değişmez!
3. "Khamûl the Easterling" → detectionRange=2, respawns=true

---

## 📚 BÖLÜM 5: SSE (Server-Sent Events)

---

### 5.1 SSE Nedir?

WebSocket'in basit versiyonu. Server → Client tek yönlü veri akışı.

```
Normal HTTP:
  Client → "Ne oldu?" → Server → "Şu oldu" → Client
  Client → "Şimdi ne oldu?" → Server → "Bu oldu" → Client
  (Her seferinde yeni istek — polling)

SSE:
  Client → "Beni bilgilendir" → Server
  Server → "Event 1" → Client
  Server → "Event 2" → Client
  Server → "Event 3" → Client
  (Tek bağlantı, sürekli akış)
```

Bu projede: Oyun olayları (birim hareketi, savaş sonucu, tur sonu)
SSE ile tarayıcıya gerçek zamanlı push edilir.

```javascript
// index.html'de:
const evtSource = new EventSource('/events?playerId=light-player');
evtSource.onmessage = (event) => {
    const data = JSON.parse(event.data);
    updateGameUI(data);
};
```

---

## 📚 BÖLÜM 6: DOCKER

---

### 6.1 Docker Nedir?

Uygulamayı "kontyener" içinde çalıştırır — her yerde aynı çalışır.

```
Fiziksel sunucu:
  └── Docker Engine
       ├── Container 1: Kafka Broker 1
       ├── Container 2: Kafka Broker 2
       ├── Container 3: Go Engine 1
       ├── Container 4: Go Engine 2
       └── ...
```

### 6.2 Docker Compose

Birden fazla container'ı tek komutla yönetir:

```yaml
# docker-compose.yml
services:
  kafka-1:        # Kafka broker
  go-engine-1:    # Go game engine
  nginx:          # Load balancer
```

```bash
docker compose up -d    # Hepsini başlat
docker compose down -v  # Hepsini durdur
docker compose logs -f  # Logları izle
```

### 6.3 Multi-Stage Docker Build

Go uygulamanın Docker image'ını küçük tutmak için:
```dockerfile
# Aşama 1: Derleme (büyük image, Go compiler var)
FROM golang:1.22 AS builder
RUN go build -o /rotr-engine

# Aşama 2: Çalışma (küçük image, sadece binary)
FROM alpine:3.19
COPY --from=builder /rotr-engine /rotr-engine
# Sonuç: ~15MB (Go compiler'sız)
```

---

## 📚 BÖLÜM 7: NGINX

---

### 7.1 Load Balancer Nedir?

Gelen istekleri birden fazla sunucuya dağıtır:

```
İstek 1 → Nginx → Go-1
İstek 2 → Nginx → Go-2
İstek 3 → Nginx → Go-3
İstek 4 → Nginx → Go-1  ← round-robin, başa döner
```

**Neden?**
- 1 instance yerine 3 → 3x kapsite
- 1 çökse, 2 devam eder
- Ölçekleme kolay: 3 → 5 → 10 instance

---

## 🏁 ÖZET: Tüm Teknolojiler Bir Arada

```
KULLANICI ↔ TARAYICI (HTML/JS/SSE)
         ↕
      NGINX (Load Balancer, round-robin)
         ↕
   GO ENGINE × 3 (Stateless, goroutine, channel, select)
   ├── EventRouter (bilgi asimetrisi)
   ├── TurnProcessor (13 adım)
   ├── CombatEngine (savaş formülü)
   ├── Pipeline 1+2 (fan-out/fan-in, 4 worker)
   └── WorldStateCache (thread-safe, sync.RWMutex)
         ↕
      KAFKA CLUSTER × 3 (Event streaming, partition, replication)
   ├── 10 Topic (orders, events, ring, broadcast, dlq)
   ├── Schema Registry (Avro, V1/V2 evolution)
   ├── Consumer Group (rebalance, fault tolerance)
   └── Exactly-Once (idempotent producer)
         ↕
   KAFKA STREAMS (Java, Topology 1+2, KTable, branch)
         ↕
      DOCKER COMPOSE (12 container, orchestration)
```

---

**Bu belgeyi baştan sona okursan, projenin HER satırını anlarsın ve
hocana güvenle anlatabilirsin. 💪**
