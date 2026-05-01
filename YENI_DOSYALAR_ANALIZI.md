# 📁 SONRADAN EKLENEN DOSYALAR — KAPSAMLI ANALİZİ

Proje 3 yeni dosya alarak daha da zenginleştirildi. İşte her birinin detaylı açıklaması:

---

## 1. 📚 **sonnet.md** — Kapsamlı Eğitim Dokümanı

### Amaç
Ödevin tüm teknik detaylarını Türkçe olarak, **diyalog formatında** derinlemesine anlatan rehber belge.

### İçerik Bölümleri

#### **Bölüm 1: Kafka Temelleri (20+ sayfa)**

**Neler Öğrenilir:**

| Konu | Derinlik | Başlık |
|------|----------|--------|
| **Topic Konsepti** | Derinlemesine | Kafka nedir? Neden var? |
| **Partition** | Pratik | Paralel çalışma, partition key seçimi |
| **Producer & Consumer** | Uygulamalı | Offset yönetimi, consumer group rebalance |
| **KTable** | Kritik | Anlık durum tabloları, stateful işleme |
| **Topology 1: Validation** | Kod ile | 8 kural sırayla nasıl uygulanır |
| **Topology 2: Risk Scoring** | Formülü | riskScore hesaplaması, zenginleştirme |
| **Exactly-Once Semantics** | GameOver örneği | Idempotence garantisi |
| **Schema Registry & Avro** | Evolution | V1→V2 migration, backward compat |

**Dikkat Çeken Kısımlar:**

```markdown
🤔 Kafka Nedir? Neden Var?

Naif çözüm: Direkt HTTP
  Tarayıcı → POST /order → Oyun Motoru
  ❌ Motor çökerse emir kaybolur
  ❌ Motor yavaşsa tarayıcı bekler
  ❌ Aynı emri iki servis işleyebilir

Kafka çözümü: Kalıcı event log
  Tarayıcı → Kafka → Motor
  ✓ Event'ler disk'te sıralı tutulur
  ✓ Motor çökse bile emirler kaybolmaz
  ✓ Replay mümkün
```

**Somut Kod Örnekleri:**

Öd devde tüm topologic'ler gerçek Kafka Streams kodları ile gösterilmiş:

- **Validation örneği**: "Kural 1: Tur numarası doğru mu? Hatalıysa WRONG_TURN → DLQ"
- **Risk formülü**: `riskScore = threat + surveillance*3 + blocked*5 + proximity*2`
- **Exactly-once**: Motor çöküp restart ederse GameOver sadece 1 kez

---

#### **Bölüm 2: Akka vs Go Karşılaştırması (15+ sayfa)**

**Kavramsal Düzey:**

| Boyut | Akka | Go | Özet |
|------|------|-----|------|
| **State Yönetimi** | Aktör içinde | Kafka'da | A: tightly coupled, B: decoupled |
| **Fault Tolerance** | Cluster + Persistence | Consumer Group | A: complex, B: Kafka handles it |
| **Concurrency** | Actor model | Goroutine + Channel | A: sealed communication, B: open |
| **Dil Kompleksliği** | Scala + Akka = dik | Go = sade | A: powerful, B: pragmatic |
| **Code Satırı** | Az ama ekspresif | Fazla ama net | A: 50%, B: baseline |

**Akka Aktör Model — Derin Analiz:**

```
┌──────────────────────────────┐
│      UnitActor (aragorn)     │
│                              │
│  Mailbox: [msg1, msg2...]    │  ← Mesajlar sıralı
│       ↓                       │
│   Behavior (pattern match)    │  ← Davranış seç
│       ↓                       │
│   State Update                │  ← ONLY this actor
│  {region: weathertop, str:5}  │
│                              │
│  Guarantee: Single-threaded   │  ← Race condition yok!
└──────────────────────────────┘
```

**Akka Persistence — Recovery:**

```
Normal:
  Event → Journal → State Update → Kafka

Crash & Recovery:
  1. Son snapshot yükle
  2. Snapshot'tan sonraki event'leri replay et
  3. State kurtarıldı — devam et

Örnek:
  Snapshot: {region: weathertop, strength: 3}
  Event: UnitMoved(weathertop, rivendell)
  Sonuç: {region: rivendell, strength: 3}
```

**Go Goroutine Model — Paralel İşleme:**

```
Buffered Channel:
  Pipeline 1 (Route Risk)
  Dispatcher → ch (cap=20) → 4 worker
                              ↓
                          unbuffered ch
                              ↓
                          Aggregator

Neden buffered? Worker meşgulse Dispatcher bloklanmasın
Neden unbuffered? Aggregator sonuçları anında sıraya kop
```

**Go Select Loop — Event-Driven:**

```go
for {
    select {
    case msg := <-kafkaConsumerCh:
        router.route(msg)
    case conn := <-newConnectionCh:
        sessions[conn.PlayerID] = conn
    case req := <-analysisRequestCh:
        go pipeline.Dispatch(req)
    case snap := <-cacheUpdateCh:
        cache.Update(snap)
    case <-time.After(60 * time.Second):
        // Tur timer
    case sig := <-signalCh:
        // Graceful shutdown
    }
}
```

---

#### **Bölüm 3: Diyalog Formatı**

Belge soru-cevap şeklinde yazılmış:

```markdown
Q: Hangi seçeneği seçmeli?
A: Dürüst değerlendirme:

Akka seç eğer:
  - Scala deneyimin varsa
  - "State logic'in yanında olsun" fikrini beğeniyorsan
  
Go seç eğer:
  - Go biliyorsan
  - Kafka state store konseptini anlamak istiyorsan
```

---

### Dosyanın Özellikleri

| Özellik | Değer |
|---------|-------|
| Format | Markdown (Diyalog) |
| Dil | 100% Türkçe |
| Boyut | ~2000 satır |
| Kod Örnekleri | 50+ |
| Şema Diyagramları | ASCII art |
| Okuma Süresi | 2-3 saat |
| Hedef | Ödevin mantığını çürütmek |

---

## 2. 📄 **README.md** — Proje Setup Dokümanı

### Amaç
Projenin teknoloji seçimini, setup'ını ve çalıştırılmasını belgeleyen standart README.

### Temel İçerik

#### **A. Teknoloji Seçimi Açıklaması**

```md
# Ring of the Middle Earth

**Technology Choice: Option B — Go**

Why Go over Akka:
- Go + Kafka stateless architecture maps naturally
- All state in Kafka KTables → Go instances interchangeable
- Consumer group rebalance handles fault tolerance in seconds
- Information asymmetry enforced in EventRouter.route()
- Verified with go test -race
```

**Not:** Bu seçim açıkça belirtilmiş. Demek ki ekip Go'yu tercih etmiş! ✅

#### **B. Repository Yapısı**

```
ring-of-the-middle-earth/
├── docker-compose.yml        ← 9 servis tanımı
├── Makefile                  ← 12 komut
├── config/
│   ├── units.conf           ← 14 birim (config-driven)
│   └── map.conf             ← 22 bölge, 35 yol
├── kafka/
│   ├── schemas/             ← 11 Avro .avsc file
│   ├── streams/             ← Topology 1 & 2 (Java)
│   └── init/                ← Topic creation
├── option-b/                ← Go implementation
│   ├── internal/
│   │   ├── api/             ← HTTP + SSE
│   │   ├── cache/           ← WorldStateCache
│   │   ├── game/            ← TurnProcessor, Combat
│   │   ├── kafka/           ← Consumer/Producer
│   │   ├── pipeline/        ← Routes (P1), Intercept (P2)
│   │   ├── router/          ← Information asymmetry
│   │   └── validation/      ← 8 order rules
│   └── cmd/server/main.go   ← Entry point
└── ui/
    └── index.html           ← Vanilla JS + SSE
```

**Dizin Açıklaması:**

| Dizin | Amaç | Not |
|-------|------|-----|
| `kafka/` | Kafka Streams topology'leri | Java'da yazılı |
| `option-b/` | Go game engine | Stateless |
| `ui/` | Frontend | React değil, vanilla JS |

#### **C. Services (Docker Compose)**

| Service | Port | Rol |
|---------|------|-----|
| nginx | 80 | Load balancer → 3 Go instance |
| go-1/2/3 | 8080-8082 | Game engine (stateless) |
| kafka-1/2/3 | 29092-29094 | Broker'lar |
| schema-registry | 8081 | Avro şema merkezi |
| kafka-streams | 8090 | Topology 1 + 2 |
| zookeeper | 2181 | Coordination |

**Startup Order:**
```
zookeeper 
  ↓
kafka-1/2/3 
  ↓
schema-registry + kafka-init 
  ↓
kafka-streams + go-1/2/3 
  ↓
nginx
```

#### **D. Make Targets (12 Komut)**

```bash
make up              # 🚀 Hepsi ayağa kalk
make down            # ⬇️ Hepsi kapat
make test            # ✅ Unit test (no Docker)
make logs            # 📋 Go logs
make logs-kafka      # 📋 Kafka logs
make check-topics    # 📊 Topic bilgisi
make register-schemas # 📝 Avro registry
make fault-test      # 💥 Scenario 3: go-2 stop
make check-game-over # 🏆 GameOver sayısı
make clean           # 🧹 Tümünü sil
```

#### **E. Unit Tests**

```
option-b/
├── internal/game/combat_test.go       (6 case)
├── internal/router/router_test.go     (3 case)
├── internal/pipeline/pipeline1_test.go (2 case)
└── internal/pipeline/pipeline2_test.go (2 case)
```

**Test Şartı:** `go test -race ./...` → Race condition yok

#### **F. API Endpoints (8 Tane)**

| Endpoint | Method | Amaç |
|----------|--------|------|
| `/game/start` | POST | Oyunu başlat |
| `/game/state` | GET | Dünya durumu (Dark Side: RB konumu = "") |
| `/order` | POST | Order gönder (202 Accepted) |
| `/orders/available` | GET | Mevcut emirler |
| `/events` | GET | SSE stream (bilgi asimetri uygulanır) |
| `/analysis/routes` | GET | Light Side: route risk sıralaması |
| `/analysis/intercept` | GET | Dark Side: Nazgul planning |
| `/health` | GET | Service durumu |

#### **G. Demo Senaryoları**

**Senaryo 1 — Information Hiding:**
```bash
# Light Side: RingBearerDetected görmez
# Dark Side: RingBearerDetected görür
# curl /game/state → Dark Side: ring-bearer.currentRegion = ""
```

**Senaryo 3 — Fault Tolerance:**
```bash
docker stop go-2
# Kafka consumer group rebalance
# go-1 & go-3 devam eder

docker start go-2
# Partition replay, state kurtarma
# Hazır
```

---

### README.md Yapısı

| Bölüm | Satır | İçerik |
|-------|-------|--------|
| Title + Tech Choice | 1-15 | Ne neden Go? |
| Repo Structure | 16-50 | Dizin haritası |
| Prerequisites | 51-60 | Tool versiyonları |
| Quick Start | 61-80 | `make up` ve browser |
| Make Targets | 81-110 | 12 komut açıklaması |
| Unit Tests | 111-150 | Test dosyaları + çalıştırma |
| Services | 151-180 | Her servisin portu + rolü |
| Kafka Topics | 181-200 | 10 topic yapı tablosu |
| API Endpoints | 201-230 | 8 route detayı |
| Demo Scenarios | 231-280 | 3 proje senaryosu |

---

## 3. 🎨 **index.html** — Oyun Arayüzü (UI)

### Amaç
Tarayıcıdan iki oyuncunun (Light vs Dark) oyun oynayabilmesi için Vanilla JavaScript + SSE ile gerçeklenmiş web arayüzü.

### Tasarım Felsefesi

**Tema: Tolkien & Ortaçağ**

```css
Renkler:
  --gold:        #c9a84c      (Ana renk)
  --dark:        #0d0b07      (Arka plan)
  --parchment:   #e8dfc8      (Metin)
  --light-side:  #4a90d9      (Mavi)
  --dark-side:   #c0392b      (Kırmızı)

Font:
  Başlıklar:  'Cinzel Decorative', cursive    (Zarif, mittaval)
  Metin:      'IM Fell English', serif        (Classic, İngilizce)
  Font-icons: Cinzel, serif                   (Elegant)
```

**Atmosfer:**
```css
Efektler:
  - shadow: text-shadow: 0 0 30px rgba(201,168,76,0.4)
  - background gradients: radial-gradient (ışık vurgulu)
  - border: 1px solid rgba(201,168,76,0.15) (altın ton ince çerçeveler)
  - letter-spacing: kısım başlıkları için 0.3em (geniş aralık, resmi)
```

---

### HTML Yapısı (5 Ana Section)

#### **1. Header**

```html
<header>
  <h1>RING OF THE MIDDLE EARTH</h1>
  <div class="subtitle">⬥ DISTRIBUTED STRATEGY GAME ⬥</div>
</header>
```

**CSS Detayları:**
- Altın gölge (text-shadow)
- Dekoratif bullet'ler (⬥) sol/sağ
- Gradyan arka plan (gold 8%, transparent)

---

#### **2. Login Screen**

**İçerik:**
```html
<div id="login-screen">
  <div class="login-box">
    <h2>ENTER THE TALE</h2>
    <input type="text" id="player-name" placeholder="Your name...">
    
    <div class="side-select">
      <button class="side-btn" data-side="light">
        FREE PEOPLES
      </button>
      <button class="side-btn" data-side="dark">
        THE SHADOW
      </button>
    </div>
    
    <button class="btn" onclick="startGame()">
      BEGIN GAME
    </button>
  </div>
</div>
```

**UX:**
- Oyuncu adı girişi
- Side seçimi (Light / Dark) → toggle style ile visual feedback
- Başlat butonu (disabled state yoksa)

---

#### **3. Game Screen — Layout (3 Kolon)**

```
┌─────────────────────────────────┐
│   LEFT PANEL (Units List)       │ 260px
├──────────────────────────────────────────┤
│   CENTER PANEL (Map)  │ STATUS BAR        │ 1fr
│                       │ LEGEND            │
│   [Harita SVG]        │ [Analysis]        │
│                       │                   │
├──────────────────────────────────────────┤
│   RIGHT PANEL (Orders / Info)            │ 260px
└─────────────────────────────────────────┘
```

**Grid Layout:**
```css
.game-layout {
  display: grid;
  grid-template-columns: 260px 1fr 260px;
  grid-template-rows: auto 1fr auto;
  min-height: calc(100vh - 90px);
}
```

---

#### **4. Left Panel — Units**

**Gösterilen:**
- Kendi tarafın 7 birimi
- Her birim için:
  - İsim (Cinzel font, altın)
  - Güç: `[████░░] 5` (progress bar)
  - Konum: "Bree"
  - Status badge: ACTIVE / DESTROYED / RESPAWNING

**Interaksiyon:**
- Click unit → seç (border rengi degişir)
- Order options belirir

**CSS:**
```css
.unit-card {
  background: var(--dark3);
  border: 1px solid rgba(201,168,76,0.15);
  cursor: pointer;
}
.unit-card.selected {
  border-color: var(--gold);
  background: rgba(201,168,76,0.08);
}
.strength-bar {
  height: 3px;
  background: linear-gradient(90deg, var(--gold-dim), var(--gold));
}
.status-badge {
  position: absolute;
  top: 0.4rem; right: 0.5rem;
}
```

---

#### **5. Center Panel — Map (Harita)**

**İçerik:**
- MiddleEarthMap.svg embed'i
- Interactive regions / units overlay
- Click region → attack menu
- Click unit → move menu

---

#### **6. Right Panel — Orders & Info**

**İçerik:**
- Mevcut order'lar (seçili birim için)
- Path risk score (Light Side)
- Intercept plan (Dark Side)
- Statistics: kaç tur, kimin kazanma şansı?

---

### CSS Özellikleri (Responsive)

**Breakpoints:**
```css
/* Mobile */
@media (max-width: 768px) {
  .game-layout {
    grid-template-columns: 1fr;  /* Panels saklanır, menu toggle */
  }
}
```

**Renk Tema:**
```css
--light-side: #4a90d9  /* Light'ın emirleri mavi */
--dark-side:  #c0392b  /* Dark'ın emirleri kırmızı */
```

**Typography:**
- `0.65rem` - 0.95rem: Sidebarlar (yoğun bilgi)
- `1rem` - 1.8rem: Başlıklar (responsive clamp)

---

### JavaScript Entegrasyonu (Bağlantılar)

```html
<script>
  // SSE Connection
  const eventSource = new EventSource(`/events?playerId=${playerId}`);
  
  eventSource.addEventListener('WorldStateSnapshot', (e) => {
    const state = JSON.parse(e.data);
    // Left panel güncellensin
    updateUnits(state.units);
    // Center panel güncellensin
    updateMap(state.regions);
  });
  
  // Order gönderme
  async function sendOrder(order) {
    const response = await fetch('/order', {
      method: 'POST',
      body: JSON.stringify(order)
    });
    // 202 Accepted alırız
  }
</script>
```

---

### İçerik Akışı (Event Loop)

```
┌─────────────────────────────────────┐
│    Browser (index.html)              │
│                                      │
│  EventSource → /events (SSE)        │
│       ↓                              │
│  WorldStateSnapshot al              │
│  (Dark Side: RB konum = "")          │
│       ↓                              │
│  UI güncelle:                        │
│    - Units panel                     │
│    - Map regions                     │
│    - Turn display                    │
│       ↓                              │
│  User click Aragorn → order yap     │
│       ↓                              │
│  POST /order                         │
│  {orderType:"AssignRoute",           │
│   pathIds:[...]}                     │
│       ↓                              │
│  202 Accepted                        │
│  (Order Kafka'ya yollandı)           │
└─────────────────────────────────────┘
```

---

### UI Bölümleri Detaylı Tablo

| Komp | CSS Class | Amaç | Responsive |
|------|-----------|------|------------|
| Header | `header` | Logo + subtitle | `clamp(1rem, 3vw, 1.8rem)` |
| Login | `.login-box` | Oyuncu info | max-width: 420px |
| Status Bar | `.status-bar` | Turn + player info | Flex row |
| Left Panel | `.panel-left` | Units list | `max-height: calc(100vh - 160px)` |
| Unit Card | `.unit-card` | Unit display | Grid |
| Center Map | `.panel-center` | Harita SVG | Position: relative |
| Right Panel | `.panel-right` | Orders/info | `max-width: 260px` |

---

## 📊 3 Dosyanın Birlikte Rolü

```
sonnet.md (Eğitim)
    ↓
    "Kafka konusunu öğrendin,
     şimdi Go + harita nasıl
     çalışıyor?"
    ↓
README.md (Setup)
    ↓
    "Projeyi çalıştırmak için
     make up yap, bu servisler
     başlayacak"
    ↓
index.html (Uygulama)
    ↓
    "Browser'da oyun oynan,
     SSE ile state güncellensin,
     order'lar POST edilsin"
```

---

## ✅ Özet: Neden Bu 3 Dosya Önemli?

| Dosya | Kimin İçin | Okuma Süresi | Amacı |
|-------|-----------|--------------|-------|
| **sonnet.md** | Geliştirici | 2-3 saat | Teknik derinlik |
| **README.md** | DevOps + Setup | 30 dakika | Proje çalıştırma |
| **index.html** | Frontend Dev | 1 saat | Gaming UX |

---

## 🎯 Tavsiyeler

1. **sonnet.md'yi önce oku** — Proje felsefesini anlaşanak
2. **README.md'deki `make up` çalıştır** — Sistemini test et
3. **index.html'deki CSS'i öğren** — UI tema hakkında bilgi al
4. **Browser'da oyun oyna** — Tüm bileşenler entegrasyonunu gör

---

Harika bir proje seti! 🚀 Her parçası birbiriyle mükemmel entegre...
