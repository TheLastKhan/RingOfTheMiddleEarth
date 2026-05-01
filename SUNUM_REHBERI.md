# 🏔️ Ring of the Middle Earth — Hoca Q&A ve Demo Rehberi

## 📋 BÖLÜM 1: HOCANIN SORABİLECEĞİ SORULAR VE CEVAPLARI

---

### Soru 1: "Bu projede hardcoding yok diyorsunuz, gösterir misiniz?"

**Cevap:**
```
Hocam, hiçbir game logic dosyasında unit ID'si string olarak yazılmamıştır.

Örneğin detection.go'da Nazgul'ü bulmak için şu şekilde yapıyoruz:
- ❌ YANLIŞ: if unitID == "witch-king" → bu hardcoding
- ✅ DOĞRU: if unitCfg.DetectionRange > 0 → config'den okunuyor

Sauron'u bulmak için:
- ❌ YANLIŞ: if unitID == "sauron"
- ✅ DOĞRU: if unitCfg.Maia && unitCfg.Indestructible && unitCfg.Side == "SHADOW"

Tüm birim davranışları config/units.conf'tan okunur. Eğer yarın yeni bir
Nazgul eklemek istersek, sadece config'e bir satır ekleriz, hiçbir Go 
kodunu değiştirmemiz gerekmez.
```

**Gösterilecek dosyalar:**
- `config/units.conf` → "Bakın, her birimin detectionRange, maia, cooldown gibi alanları var"
- `internal/game/detection.go` → `BuildDetectionInput()` fonksiyonu → "config.DetectionRange > 0 kullanıyoruz"

---

### Soru 2: "EventRouter nasıl çalışıyor? Bilgi gizleme nasıl sağlanıyor?"

**Cevap:**
```
EventRouter, bilgi asimetrisinin TEK enforcement noktasıdır.

3 kural var:
1. game.ring.position → SADECE Light Side SSE kanalına gider
   Dark Side kanalına ASLA gönderilmez

2. game.ring.detection → SADECE Dark Side SSE kanalına gider
   Light Side ASLA detection bilgisi almaz

3. game.broadcast (WorldStateSnapshot) → Her iki tarafa gider AMA:
   - Light Side: ringBearer.currentRegion = "weathertop" (gerçek konum)
   - Dark Side: ringBearer.currentRegion = "" (HER ZAMAN BOŞ)

stripRingBearer() fonksiyonu bunu garantiler.
go test -race ile doğruladık — race condition yok.
```

**Gösterilecek dosyalar:**
- `internal/router/event_router.go` → Route() fonksiyonu, switch-case yapısı
- `internal/router/router_test.go` → 3 test case

---

### Soru 3: "Savaş formülünü anlatır mısınız?"

**Cevap:**
```
Formül 4 bileşenden oluşur:

attacker_power = Σ(effective_strength)
defender_power = Σ(effective_strength)
              + terrain_bonus        (FORTRESS +2, MOUNTAINS +1)
              + fortification_bonus  (fortified ise +2)

effective_strength = base_strength + leadership_bonus
leadership_bonus = co-located leader'ın leadershipBonus değeri (config'den)

Özel durumlar:
- ignoresFortress: Uruk-hai terrain bonus'u bypass eder AMA fortification
  bonus'unu BYPASS ETMEZ (bu bir tuzak soru!)
- indestructible: Witch-King'in gücü minimum 1, asla 0'a düşmez
- leadership: Aragorn (leader) yanındaki Gimli'ye +1 verir

6 farklı test case ile doğruladık.
```

**Gösterilecek dosya:** `internal/game/combat_test.go`

---

### Soru 4: "Pipeline yapısını anlatır mısınız? Fan-out/fan-in nedir?"

**Cevap:**
```
2 pipeline var:

Pipeline 1 (Light Side — Route Risk):
- Input: 4 kanonik rota
- 4 worker goroutine, buffer cap 20
- Her worker bir rotanın riskScore'unu hesaplar
- riskScore = Σ(threatLevel) + Σ(surveillance×3) + BLOCKED×5 + THREATENED×2 + nazgulProximity×2
- Sonuç: en düşük riskli rota önerisi

Pipeline 2 (Dark Side — Intercept):
- Input: Her (Nazgul, rota) çifti
- 4 worker goroutine, buffer cap 30
- interceptWindow = rbTurnsToReach - turnsToIntercept
- score = 1.0 - (turnsToIntercept / routeLength)

Fan-out: Tek goroutine input channel'a veri pompalar
Fan-in: 4 worker paralel hesaplar, sonuçlar tek result channel'da toplanır
Context: 2 saniye timeout, context.WithTimeout ile cancel edilir
WaitGroup: Tüm workers bitmeden result channel kapanmaz
```

**Gösterilecek dosya:** `internal/pipeline/pipeline.go` → `ComputeRouteRisk()` fonksiyonu

---

### Soru 5: "Select loop'taki 7 case ne?"

**Cevap:**
```
Go'nun select statement'ı, birden fazla channel'ı aynı anda dinler.
Hangi channel'da veri gelirse o case çalışır:

1. kafkaConsumerCh    → Kafka'dan gelen event'leri router'a yönlendir
2. newConnectionCh    → Yeni SSE bağlantısı (tarayıcı açıldı)
3. disconnectCh       → SSE bağlantı koptu (tarayıcı kapandı)
4. analysisRequestCh  → Pipeline analizi istendi
5. cacheUpdateCh      → WorldState cache'ini güncelle
6. turnTimer          → Tur süresi doldu, yeni tur başla
7. signalCh           → SIGTERM/SIGINT → graceful shutdown

Bu yapı Go'da tek goroutine'de multiplexing sağlar.
Goroutine leak olmaz çünkü: 
- context.Cancel tüm child goroutine'leri kapatır
- pprof ile izlenebilir
```

**Gösterilecek dosya:** `cmd/server/main.go` → select loop

---

### Soru 6: "Kafka Streams topology'leri nasıl çalışıyor?"

**Cevap:**
```
Java ile yazılmış 2 topology tek JVM'de çalışır:

Topology 1 — Order Validation:
  game.orders.raw → branch() → 
    valid   → game.orders.validated
    invalid → game.dlq (DLQ = Dead Letter Queue)

  8 kural:
  1. Turn number kontrolü
  2. Unit ownership (birim doğru tarafa mı ait?)
  3. Path blocked kontrolü (Ring Bearer için)
  4. Path in route kontrolü
  5. Unit at endpoint kontrolü
  6. Attack target kontrolü
  7. Maia cooldown kontrolü
  8. Duplicate order kontrolü

Topology 2 — Route Risk Enrichment:
  game.orders.validated + game.broadcast (KTable) → leftJoin → 
    → routeRiskScore eklenerek game.orders.validated'a V2 yazılır

Schema Evolution (K3):
  V1: {playerId, unitId, orderType, payload, turn, timestamp}
  V2: V1 + {routeRiskScore: ["null","int"]}  ← nullable, backward compatible
  V1 consumer'lar V2 mesajını okuyabilir (routeRiskScore'u yok sayar)
```

---

### Soru 7: "Bir Go instance çökerse ne olur?"

**Cevap:**
```
Fault tolerance Kafka consumer group rebalance ile sağlanır.

Senaryo: 3 Go instance çalışıyor, go-2 çöktü.

1. go-2'nin kalp atışı durur (heartbeat timeout: 10s)
2. Kafka consumer group coordinator, go-2'nin partition'larını 
   go-1 ve go-3'e yeniden atar (rebalance)
3. go-1 ve go-3, eski partition'ları en son committed offset'ten 
   okumaya devam eder
4. KTable: Kafka'nın kendisi state store → rebuild otomatik
5. Oyun hiç kesilmeden devam eder

Demo: 
  docker compose stop go-engine-2
  # 10 saniye bekle
  curl localhost:8080/health  → hala çalışıyor!
  docker compose start go-engine-2  → tekrar katılır
```

---

### Soru 8: "Exactly-once semantics nasıl sağlanıyor?"

**Cevap:**
```
Kafka Streams'de: PROCESSING_GUARANTEE = EXACTLY_ONCE_V2
Go Producer'da: enable.idempotence = true

Bu ne demek?
- Aynı GameOver event'i, sistem çökse bile SADECE BİR KEZ üretilir
- Kafka broker sequence number tutarak duplicate'leri reddeder
- Consumer offset commit + message produce atomik transaction

Neden önemli?
- İki kez GameOver üretilirse UI'da iki kez "OYUN BİTTİ" görünür
- Hiç üretilmezse oyun sonsuza kadar devam eder
- Exactly-once her iki durumu da önler
```

---

## 🎬 BÖLÜM 2: DEMO SENARYOLARI (15 DAKİKA)

---

### Demo 1: Information Hiding (5 dakika)

**Adımlar:**
1. İki tarayıcı tab aç:
   - Tab 1: `http://localhost:3000?side=light` (Light Side)
   - Tab 2: `http://localhost:3000?side=dark` (Dark Side)

2. Light Side'dan Ring Bearer'ı hareket ettir:
   - Ring Bearer'ı The Shire → Bree'ye taşı

3. **KONTROL** — Light Side tab'ında:
   - ✅ Ring Bearer'ın konumu "Bree" olarak görünür
   - ✅ Haritada yeni konum işaretli

4. **KONTROL** — Dark Side tab'ında:
   - ✅ Ring Bearer'ın konumu "???" olarak görünür
   - ✅ currentRegion alanı BOŞ
   - ❌ Gerçek konum GÖRÜNMEZ

5. Hocaya göster: "Bakın, aynı anda iki tab açık ama bilgi farklı"

**Teknik açıklama:**
> EventRouter.Route() fonksiyonunda game.broadcast event'i Dark Side'a gönderilmeden 
> önce stripRingBearer() çağrılır. Bu fonksiyon ring-bearer'ın currentRegion'ını 
> boş string yapar.

---

### Demo 2: Maia Abilities + Path Mechanics (5 dakika)

**Adımlar:**
1. Dark Side olarak Saruman'ı seç
2. MAIA_ABILITY emri ver → path "fangorn-to-isengard" üzerinde
3. **KONTROL**: Path status BLOCKED olarak değişir
4. Light Side'dan Ring Bearer'ı bu path'ten geçirmeye çalış
5. **KONTROL**: Validasyon hatası → "PATH_BLOCKED"
6. Dark Side'dan Saruman'a tekrar MAIA_ABILITY ver
7. **KONTROL**: "ABILITY_ON_COOLDOWN" hatası → config.cooldown = 2 tur
8. 2 tur bekle, tekrar dene → başarılı

**Teknik açıklama:**
> Saruman'ın hangi path'leri bozabileceği config/units.conf'taki 
> maiaAbilityPaths listesinden okunur. Cooldown süresi de config.cooldown'dan gelir.
> Hiçbir hardcoding yok.

---

### Demo 3: Fault Tolerance (5 dakika)

**Adımlar:**
1. Terminalde: `docker compose ps` → 3 Go instance çalışıyor
2. `docker compose stop go-engine-2` → 1 instance durdur
3. 10 saniye bekle (consumer group rebalance)
4. `curl localhost:8080/health` → hala çalışıyor!
5. Tarayıcıda oyun devam ediyor → SSE bağlantısı aktif
6. `docker compose logs go-engine-1 | tail -20` → "rebalance" log mesajı
7. `docker compose start go-engine-2` → instance geri geldi
8. `docker compose ps` → 3/3 çalışıyor

**Teknik açıklama:**
> Go instance'ları stateless — hiçbir local state tutmazlar.
> Tüm state Kafka KTable'larda. Bir instance çöktüğünde:
> 1. Consumer group coordinator partition'ları yeniden dağıtır
> 2. Kalan instance'lar eski partition'ları alır
> 3. Committed offset'ten okumaya devam eder
> 4. KTable otomatik rebuild olur

---

## 🎯 BÖLÜM 3: SUNUM İPUÇLARI

### Sunum Sırası (15 dakika):
1. **Mimari tanıtım** (2 dk): Diagram göster, "Go stateless, Kafka state tutuyor"
2. **Demo 1** (4 dk): Bilgi gizleme — en etkileyici demo
3. **Demo 2** (4 dk): Maia + path — config-driven tasarım
4. **Demo 3** (4 dk): Fault tolerance — distributed systems gücü
5. **Q&A** (1 dk): "Sorularınız?"

### Kritik Kelimeler (Hocayı Etkilemek):
- "Config-driven" — "Hiçbir unit ID hardcoded değil"
- "Information asymmetry" — "Tek enforcement point: EventRouter"
- "Fan-out/fan-in" — "4 worker, buffered channel, WaitGroup"
- "Exactly-once semantics" — "Idempotent producer ile garanti"
- "Consumer group rebalance" — "Partition reassignment"
- "Schema evolution" — "V2 backward compatible, nullable field"
- "Dead Letter Queue" — "Invalid orders kaybolmaz, DLQ'ya gider"
