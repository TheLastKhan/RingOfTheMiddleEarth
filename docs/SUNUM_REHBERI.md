# Ring of the Middle Earth - Detayli Yuz Yuze Sunum Rehberi

Bu belge yuz yuze sunumda projeyi daha rahat anlatmak icin hazirlandi.
Amac sadece "uygulama calisiyor" demek degil; hocanin bu projeyle olcmek istedigi dagitik sistem, Kafka, validation, event-driven state, fault tolerance, information hiding ve test edilebilirlik konularini anladigini gostermektir.

Teknoloji secimi:

```text
Option B - Go + Kafka
```

Kisa ozetle proje iki oyunculu, turn-based bir strateji oyunudur. Light Side, Ring Bearer'i The Shire'dan Mount Doom'a goturup yuzugu yok etmeye calisir. Shadow Side ise Ring Bearer'i bulmaya, yollarini kapatmaya, yakalamaya veya yok etmeye calisir. Teknik tarafta ise UI, Go game engine'leri, Kafka topicleri, Kafka Streams, Schema Registry, nginx ve Docker Compose beraber calisir.

---

## 1. Sunuma Baslamadan Once

Sunumdan once sistemi temiz sekilde ayaga kaldirmak icin:

```powershell
cd C:\Users\hakan\termproject
docker compose up -d --build
docker compose ps
```

Beklenen durum:

- `rotr-ui` ayakta olmali.
- `rotr-nginx` ayakta olmali.
- `rotr-go-1`, `rotr-go-2`, `rotr-go-3` ayakta olmali.
- `rotr-kafka-1`, `rotr-kafka-2`, `rotr-kafka-3` ayakta olmali.
- `rotr-schema-registry` ayakta olmali.
- `rotr-kafka-streams` ayakta olmali.
- `rotr-zookeeper` ayakta olmali.

Health kontrolu:

```powershell
Invoke-RestMethod http://localhost/health
```

Schema Registry kontrolu:

```powershell
Invoke-RestMethod http://localhost:8081/subjects
```

Go testleri:

```powershell
cd C:\Users\hakan\termproject\option-b
go test ./...
cd ..
```

Kafka Streams validation testi:

```powershell
.\scripts\demo-validation-k4.ps1
```

Exactly-once GameOver smoke testleri:

```powershell
.\scripts\check-gameover-idempotency.ps1
.\scripts\check-gameover-idempotency-dark.ps1
```

Full E2E ve fault tolerance testleri:

```powershell
.\scripts\full-e2e-smoke.ps1
.\scripts\chaos-soak-smoke.ps1
```

Browser/UI smoke testi:

```powershell
.\scripts\browser-smoke.ps1
```

Sunumda hepsini calistirmak zorunda degilsin. Sunum vaktine gore sec. En guvenli kisa kontrol:

```powershell
docker compose ps
Invoke-RestMethod http://localhost/health
Invoke-RestMethod http://localhost:8081/subjects
.\scripts\demo-validation-k4.ps1
```

---

## 2. 15-20 Dakikalik Sunum Plani

Sunumu su sirayla anlatmak en temiz akis olur:

1. Projenin amaci ve oyun kurallari.
2. Mimari: UI, nginx, Go engine'ler, Kafka, Kafka Streams, Schema Registry.
3. Browser demo: Light ve Shadow taraflarini acma.
4. Information hiding: Ring Bearer konumu neden Shadow tarafinda gizli?
5. Order flow: Browserdan gelen emir backend'de nereye gidiyor?
6. Turn processing: End Turn basinca hangi adimlar calisiyor?
7. Kafka topic/event altyapisi.
8. Kafka Streams validation ve schema evolution.
9. Analysis pipeline: route risk ve intercept analysis.
10. Fault tolerance: bir Go engine durursa oyun devam ediyor mu?
11. Testler ve kanitlar.
12. Kod turu ve Q&A.

Kisa acilis cumlesi:

```text
Bu projede Option B, yani Go + Kafka mimarisini sectim. Amacim iki oyunculu bir turn-based oyun uzerinden Kafka topicleri, event-driven state, order validation, information hiding, fault tolerance ve coklu servis mimarisini gostermekti.
```

---

## 3. Mimariyi Anlatma

Ekranda `docker compose ps` veya `docker-compose.yml` dosyasini ac.

Daha gorsel anlatim icin `TEKNIK_AKIS_DIYAGRAMLARI.md` dosyasini ac. Bu dosyada sistem mimarisi, player order akisi, End Turn pipeline'i, Kafka topicleri, SSE information hiding, fault tolerance ve exactly-once GameOver akisi Mermaid diyagramlariyla gosteriliyor.

Ana mimari:

```text
Browser UI
   |
   | HTTP / SSE
   v
nginx load balancer
   |
   v
3 Go game engine instance
   |
   v
Kafka topics
   |
   v
Kafka Streams validation/enrichment
   |
   v
Schema Registry
```

Anlat:

```text
Burada tek bir monolitik uygulama yok. Docker Compose ile birden fazla servis beraber calisiyor. UI statik olarak nginx uzerinden servis ediliyor. API istekleri nginx load balancer'a gidiyor. Nginx de bu istekleri uc Go engine instance'indan birine yonlendiriyor. Kafka ise order, event ve session state icin merkezi event altyapisi olarak kullaniliyor.
```

Servisleri tek tek anlat:

- `ui`: Browser oyunu, `localhost:3000`.
- `nginx`: API icin load balancer, `localhost:80`.
- `go-engine-1`: Go game engine, host portu `8080`.
- `go-engine-2`: Go game engine, host portu `8082`.
- `go-engine-3`: Go game engine, host portu `8083`.
- `schema-registry`: Avro schema registry, host portu `8081`.
- `kafka-1`, `kafka-2`, `kafka-3`: Kafka brokerlari.
- `kafka-streams`: Java/Kafka Streams validation servisi.
- `zookeeper`: Kafka coordination.

Onemli not:

```text
8081 Go engine degil; Schema Registry portudur. Bu yuzden engine state karsilastirmasinda 8080, 8082 ve 8083 kullanilir.
```

---

## 4. Browser Demo: Iki Tarafi Acma

Iki browser penceresi ac:

```text
Light Side:
http://localhost:3000?side=light

Dark Side:
http://localhost:3000?side=dark
```

Light tarafinda Free Peoples olarak gir.
Dark tarafinda Shadow olarak gir.

Anlat:

```text
Iki oyuncu ayni oyuna bakiyor ama ayni bilgiyi gormuyor. Light Side Ring Bearer'in gercek konumunu bilir. Shadow Side ise Ring Bearer'in konumunu dogrudan gormez. Shadow tarafinin bilgi alabilmesi icin detection event'i olmasi gerekir.
```

Burada hocaya gosterebilecegin seyler:

- Harita iki tarafta da aciliyor.
- Light tarafinda Frodo/Ring Bearer takip edilebilir.
- Shadow tarafinda Ring Bearer bilgisi gizlenir.
- Connection status SSE baglantisini gosterir.
- `End Turn` butonu turn processing'i elle tetikler.
- `End Game` butonu demo icin oyunu sifirlar.

---

## 5. Oyun Kurallari

Light Side hedefi:

1. Ring Bearer'a rota atar.
2. Ring Bearer turn sonunda rota boyunca ilerler.
3. Ring Bearer Mount Doom'a ulasir.
4. Light oyuncu `DESTROY_RING` order'i verir.
5. Mount Doom'da aktif Shadow unit yoksa Light kazanir.

Shadow Side hedefi:

1. Nazgul, Sauron, Saruman ve diger Shadow unitlerini stratejik yerlere yonlendirir.
2. Pathleri search veya block eder.
3. Ring Bearer'i tespit etmeye calisir.
4. Ring Bearer exposed olursa veya combat ile yok edilirse Shadow kazanir.

Draw:

```text
Max turn limitine kadar kazanan cikmazsa oyun draw olur.
```

Detection:

```text
Ilk 3 turn boyunca detection kapali. Bu, Light oyuncuya baslangicta gizlilik avantaji verir. Turn 4'ten sonra Nazgul range'i, Sauron bonusu ve path surveillance gibi mekanikler Ring Bearer'i expose edebilir.
```

---

## 6. Light Gameplay Demo

Light browserda:

1. Frodo Baggins'i sec.
2. `ASSIGN_ROUTE` veya UI'daki route atama modunu sec.
3. Haritada Mount Doom'a tikla.
4. UI otomatik path listesini doldurur.
5. `Issue Order` bas.
6. `End Turn` bas.

Anlat:

```text
Burada oyuncu sadece hedef region'a tikliyor. UI, mevcut harita graph'i uzerinden path listesini olusturuyor. Mount Doom hedefinde Light icin daha mantikli rota Cirith Ungol uzerinden gidiyor; Mordor uzerinden gecmek Sauron nedeniyle cok riskli.
```

Ornek Light route:

```text
shire-to-bree
bree-to-rivendell
rivendell-to-lothlorien
lothlorien-to-emyn-muil
emyn-muil-to-ithilien
ithilien-to-cirith-ungol
cirith-ungol-to-mount-doom
```

Turn ilerledikce:

- Frodo bir path ilerler.
- Event log hareket eventini gosterir.
- Light taraf Ring Bearer'in nerede oldugunu gorur.
- Shadow taraf bu bilgiyi dogrudan gormez.

---

## 7. Shadow Gameplay Demo

Dark browserda:

1. Witch-King veya Nazgul sec.
2. Stratejik hedef olarak Minas Morgul, Cirith Ungol, Mordor veya Mount Doom civarini dusun.
3. Path search/block veya route/redirect emirlerini goster.
4. `End Turn` ile turn processing'i calistir.

Anlat:

```text
Shadow tarafinda bilgisayar AI yok. Dokumanda iki insan oyuncu isteniyor. Bu yuzden Shadow unitleri kendiliginden oynamiyor; Shadow oyuncusu emir verirse hareket eder. Bu, projedeki oyun tasariminin bilincli bir parcasi.
```

Shadow stratejisi:

- Cirith Ungol ve Minas Morgul gecislerini tutmak.
- Mordor ve Mount Doom civarinda beklemek.
- Path search ile surveillance arttirmak.
- Ring Bearer exposed olursa ayni region'a girip yakalamak.

---

## 8. Order Flow: UI'dan Backend'e

Kodda ac:

```text
index.html
option-b/internal/api/server.go
option-b/internal/validation/validator.go
```

Anlat:

```text
Oyuncu UI'da Issue Order dediginde browser POST /order endpoint'ine JSON body gonderir. Go API bu order'i alir, player side ve unit ownership gibi kontroller yapar. Daha sonra order validation'dan gecirilir. Gecerli emirler oyun state'ine ve Kafka event akisina dahil edilir.
```

Basit order ornegi:

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

Burada onemli kontroller:

- Oyuncu kendi tarafina ait unit'e emir verebilir.
- Emir mevcut turn ile uyumlu olmali.
- Unit ayni turn icinde duplicate order almamali.
- `DESTROY_RING` sadece Ring Bearer tarafindan, Mount Doom'da ve Shadow unit yokken gecerli olur.
- `SEARCH_PATH` veya `BLOCK_PATH` icin unit path endpointinde olmalidir.

---

## 9. Turn Processing: End Turn Basinca Ne Oluyor?

Kodda ac:

```text
option-b/internal/game/turn.go
```

Ana fonksiyon:

```text
ProcessTurn
```

Anlat:

```text
Bu oyun turn-based oldugu icin order verdikten sonra unitler hemen hareket etmiyor. End Turn basildiginda oyun motoru sirali bir pipeline calistiriyor. Bu pipeline'in sirasi onemli cunku once emirler uygulanir, sonra hareket olur, sonra combat, detection ve win condition kontrol edilir.
```

Turn processing adimlari:

1. Validated orderlar toplanir.
2. Route assignment islenir.
3. Path block/search emirleri islenir.
4. Reinforcement/redirect emirleri islenir.
5. Fortification islenir.
6. Maia ability emirleri islenir.
7. Route atanmis unitler otomatik ilerler.
8. Combat cozulur.
9. Path timerlari guncellenir.
10. Fortification timerlari guncellenir.
11. Respawn/cooldown mekanikleri calisir.
12. Detection hesaplanir.
13. Win condition kontrol edilir.

Hocaya soyle:

```text
Burada oyun state'i rastgele degismiyor. Her turn ayni deterministic sirayla isleniyor. Bu sayede test etmek ve debug etmek daha kolay oluyor.
```

---

## 10. Kafka Topic ve Event Altyapisi

Kafka topiclerini anlat:

| Topic | Ne ise yarar |
| --- | --- |
| `game.orders.raw` | UI/API tarafindan gelen ham player orderlari |
| `game.orders.validated` | Validation sonrasi kabul edilen/enriched orderlar |
| `game.events.unit` | Unit movement, unit status gibi eventler |
| `game.events.region` | Region control ve combat sonuc eventleri |
| `game.events.path` | Path block/search/surveillance eventleri |
| `game.broadcast` | World snapshot ve GameOver gibi global eventler |
| `game.ring.position` | Light-only Ring Bearer position eventleri |
| `game.ring.detection` | Shadow tarafina gidebilen detection eventleri |
| `game.session` | Compacted latest game/session snapshot |
| `game.dlq` | Invalid order veya hata durumlari |

Anlat:

```text
Kafka burada sadece mesaj kuyrugu gibi degil, oyun eventlerinin merkezi kaydi gibi kullaniliyor. Orderlar, eventler ve session snapshotlari topiclere ayrilmis durumda. Bu ayrim hem okunabilirlik hem de fault tolerance icin onemli.
```

Schema Registry:

```powershell
Invoke-RestMethod http://localhost:8081/subjects
Invoke-RestMethod http://localhost:8081/subjects/game.orders.validated-value/versions
```

Anlat:

```text
Schema Registry, Kafka mesajlarinin beklenen formunu takip etmek icin kullaniliyor. Bu projede schema evolution da gosteriliyor; validated order schema'sinin birden fazla versiyonu var.
```

---

## 11. Kafka Streams Validation

Kod/dizin:

```text
kafka/streams
kafka/streams/src/test
```

Test:

```powershell
.\scripts\demo-validation-k4.ps1
```

Anlat:

```text
Kafka Streams tarafinda order validation ve enrichment var. Validation sayesinde gecersiz emirler oyun state'ine dogrudan uygulanmiyor. Ornegin yanlis tarafin unitine emir verme, duplicate order verme, invalid path veya invalid target gibi durumlar yakalaniyor.
```

Hocaya kisa cevap:

```text
Kafka Streams burada raw order akisini okuyup valid order akisini ureten katman olarak kullaniliyor. Ayrica route-risk enrichment gibi ek bilgi ekleme gorevi de var.
```

Exactly-once:

```text
Kafka Streams tarafinda EXACTLY_ONCE_V2 ayari kullaniliyor. GameOver tarafinda da Go engine transactional producer ile game.broadcast topicine GameOver event'i yazar. Light ve Dark victory icin ayri smoke testler GameOver'in sadece bir kere committed gorundugunu dogrular.
```

---

## 12. Information Hiding

Kodda ac:

```text
option-b/internal/router/event_router.go
option-b/internal/api/server.go
option-b/internal/cache/cache.go
```

API ile gostermek:

```powershell
Invoke-RestMethod "http://localhost/game/state?playerId=light-player&side=FREE_PEOPLES"
Invoke-RestMethod "http://localhost/game/state?playerId=dark-player&side=SHADOW"
```

Anlat:

```text
Bu projede information hiding cok onemli. Light taraf Ring Bearer'in gercek konumunu gorebilir. Shadow taraf ise dogrudan gormemeli. Bu sadece UI'da saklamak degil; backend tarafinda da Dark response'una Ring Bearer konumu verilmez.
```

Teknik olarak:

- `game.ring.position` Light-only eventtir.
- Dark SSE kanalina Ring Bearer position eventi gitmez.
- Broadcast world snapshot Dark tarafina giderken Ring Bearer region alani temizlenir.
- `DarkView.RingBearerRegion` bos tutulur.

Hocadan soru gelirse:

```text
Bu bilgi gizleme sadece frontend hilesi degil. Event router ve API serializer tarafinda uygulanir. Yani Dark browser developer tools acsa bile normal state response'unda Ring Bearer currentRegion bilgisini alamaz.
```

---

## 13. SSE: Browser Nasil Canli Guncelleniyor?

Kodda ac:

```text
index.html
option-b/internal/api/server.go
```

Anlat:

```text
Browser, backend'e Server-Sent Events baglantisi acar. Backend event olustukca browser'a push eder. Bu sayede her turn sonunda hareket, detection, GameOver ve world snapshot eventleri UI'a gelir.
```

SSE neden kullanildi?

- Browser icin basit.
- Tek yonlu event stream icin yeterli.
- WebSocket kadar karmasik degil.
- Oyun event log'u ve state update icin uygun.

Sunumda goster:

- Connection status.
- Event log.
- Turn degisimi.
- Unit moved eventleri.

---

## 14. Analysis Pipelines

Endpointler:

```powershell
Invoke-RestMethod http://localhost/analysis/routes
Invoke-RestMethod http://localhost/analysis/intercept
```

Kodda ac:

```text
option-b/internal/pipeline
```

Anlat:

```text
Analysis pipeline'lar oyun state'inden karar destek bilgisi uretir. Route analysis Light oyuncuya hangi rotanin ne kadar riskli oldugunu gosterir. Intercept analysis Shadow tarafina Ring Bearer'i yakalamak icin hangi bolge veya pathlerin onemli oldugunu hesaplar.
```

Neden onemli?

```text
Hocanin dokumaninda sadece oyun degil, state uzerinden analysis pipeline bekleniyor. Bu endpointler oyun state'ini okuyup anlamli analiz ciktisi uretiyor.
```

---

## 15. Fault Tolerance Demo

Komutlar:

```powershell
docker compose stop go-engine-2
for ($i=1; $i -le 5; $i++) { Invoke-RestMethod http://localhost/health }
docker compose up -d go-engine-2
```

Daha guclu test:

```powershell
.\scripts\full-e2e-smoke.ps1
.\scripts\chaos-soak-smoke.ps1
```

Anlat:

```text
Projede uc Go engine instance'i var. Nginx load balancer olarak calisiyor. Bir engine durursa nginx kalan engine'lere route etmeye devam edebiliyor. Kafka topicleri ve game.session snapshot'i de state'in takip edilebilmesi icin kullaniliyor.
```

State karsilastirma:

```powershell
Invoke-RestMethod "http://localhost:8080/game/state?playerId=light-player&side=FREE_PEOPLES"
Invoke-RestMethod "http://localhost:8082/game/state?playerId=light-player&side=FREE_PEOPLES"
Invoke-RestMethod "http://localhost:8083/game/state?playerId=light-player&side=FREE_PEOPLES"
```

Anlat:

```text
Burada 8080, 8082 ve 8083 Go engine instance'laridir. 8081 Schema Registry oldugu icin burada yok.
```

Dikkatli ve durust aciklama:

```text
Bu proje demo seviyesinde fault tolerance gosteriyor. Engine stop/start, nginx failover ve game.session replay testleri var. Exhaustive production crash-matrix testleri ise ayrica production hardening konusu olurdu.
```

---

## 16. GameOver Exactly-Once Testleri

Komutlar:

```powershell
.\scripts\check-gameover-idempotency.ps1
.\scripts\check-gameover-idempotency-dark.ps1
```

Light testi:

```text
Frodo Mount Doom'a gider, DESTROY_RING order'i verilir ve FREE_PEOPLES GameOver event'i beklenir.
```

Dark testi:

```text
Frodo bilerek Mordor'a giden riskli rotaya sokulur. Sauron ile combat sonucunda Ring Bearer yok edilir ve SHADOW GameOver event'i beklenir.
```

Bu testlerin asil amaci:

```text
Light veya Dark'in kazanmasini gostermekten cok, GameOver event'inin Kafka game.broadcast topicinde read_committed consumer ile sadece bir kere gorundugunu kanitlamaktir. Oyun bittikten sonra ekstra turn advance cagrilari yapilir ve yeni GameOver uretilmedigi dogrulanir.
```

---

## 17. Kod Turu

Sunum sonunda kodu gostermek icin en iyi dosya sirasi:

1. `docker-compose.yml`
   - Hangi servisler var?
   - Portlar nasil maplenmis?
   - 3 Go engine nasil calisiyor?

2. `index.html`
   - UI nerede?
   - Order nasil gonderiliyor?
   - SSE nasil baglaniyor?
   - Map canvas nasil ciziliyor?

3. `option-b/internal/api/server.go`
   - Endpointler nerede?
   - `/order`, `/game/state`, `/events`, `/game/advance-turn` nasil calisiyor?

4. `option-b/internal/game/turn.go`
   - Turn processing pipeline.
   - Movement, combat, detection, win condition.

5. `option-b/internal/validation/validator.go`
   - Order validation kurallari.
   - Unit ownership, turn mismatch, invalid target, destroy ring condition.

6. `option-b/internal/router/event_router.go`
   - Information hiding.
   - Light/Dark SSE ayrimi.

7. `option-b/internal/pipeline`
   - Route risk ve intercept analysis.

8. `kafka/streams`
   - Kafka Streams validation/enrichment.

9. `scripts`
   - Smoke testler ve demo kanitlari.

---

## 18. Hocadan Gelebilecek Sorular ve Cevaplar

**Soru: Bu projede Kafka neden kullanildi?**

Kafka order, event ve session snapshot akisini tasimak icin kullanildi. Oyun state'i turn-based oldugu icin event-driven model uygun. Ayrica Kafka topicleri sayesinde eventleri ayirmak, replay etmek, consumerlarla analiz yapmak ve fault tolerance gostermek mumkun oldu.

**Soru: Kafka Streams ne yapiyor?**

Kafka Streams raw order akisini okuyup validation ve enrichment yapiyor. Gecerli orderlari validated topic'e, gecersizleri DLQ mantigina ayiriyor. Route orderlari icin risk enrichment de yapiliyor.

**Soru: UI sadece local state mi gosteriyor?**

Hayir. UI backend state'ini HTTP ve SSE ile aliyor. Turn sonunda backend event uretir, browser SSE ile eventleri alir ve UI guncellenir.

**Soru: Dark taraf Ring Bearer konumunu nasil gizliyor?**

Bu sadece CSS veya frontend gizleme degil. Backend Dark tarafina giden state response'undan Ring Bearer currentRegion bilgisini temizliyor. Ayrica Light-only Ring Bearer position eventleri Dark SSE kanalina gonderilmiyor.

**Soru: Dusman AI var mi?**

Hayir. Dokumanda iki human player var. Shadow unitleri otomatik AI gibi oynamaz. Shadow oyuncusu kendi emirlerini verir.

**Soru: Oyun neden turn-based?**

Dokumanda turn-based oyun isteniyor. Turn-based tasarim distributed system icin de avantajli cunku orderlar toplanip deterministic bir sira ile isleniyor.

**Soru: End Turn ne yapiyor?**

End Turn, backend'de turn processor'i calistirir. Route assignment, movement, combat, detection ve win condition gibi adimlar sirayla uygulanir.

**Soru: Exactly-once iddiasi ne kadar guclu?**

Kafka Streams tarafinda `EXACTLY_ONCE_V2` kullaniliyor. GameOver icin Go tarafinda transactional producer var. Light ve Dark victory smoke testleri `read_committed` consumer ile GameOver event'inin sadece bir kere arttigini ve oyun bittikten sonra tekrar uretilmedigini dogrular.

**Soru: Production seviyesinde mi?**

Bu proje demo ve term project seviyesinde guclu bir distributed system ornegi. Coklu engine, Kafka, Schema Registry, validation, SSE, fault tolerance ve smoke testler var. Daha ileri production hardening icin uzun sureli chaos test, daha kapsamli browser automation ve daha genis crash-matrix testleri eklenebilir.

**Soru: Hardcoding var mi?**

Oyun davranisinin buyuk kismi config ve state uzerinden calisir. Unit class, side, strength, terrain, path ve region bilgileri config'ten gelir. Testlerde fixture kullanmak hardcoding degil; beklenen davranisi dogrulamak icin kontrollu veri kullanmaktir.

**Soru: 8081 neden engine degil?**

8081 Schema Registry portudur. Go engine portlari 8080, 8082 ve 8083'tur. Bu yuzden engine state karsilastirmasinda 8081 kullanilmaz.

---

## 19. Sunumda En Guvenli Demo Sirasi

Zaman azsa bu sirayi kullan:

1. `docker compose ps`
2. `Invoke-RestMethod http://localhost/health`
3. Light browser ac: `http://localhost:3000?side=light`
4. Dark browser ac: `http://localhost:3000?side=dark`
5. Frodo'ya Mount Doom route ata.
6. `End Turn` ile 1-2 turn ilerlet.
7. Light tarafin Ring Bearer'i gordugunu, Dark tarafin gormedigini anlat.
8. `/analysis/routes` ve `/analysis/intercept` endpointlerini goster.
9. `demo-validation-k4.ps1` testini goster.
10. `docker compose stop go-engine-2` ile fault tolerance anlat.
11. Kodda `turn.go`, `event_router.go`, `server.go` dosyalarini ac.
12. Exactly-once testleri isim olarak goster; zaman varsa birini calistir.

---

## 20. Kapanis Cumlesi

Sunumu su cumleyle kapatabilirsin:

```text
Bu projede sadece browserda calisan bir oyun yapmadim. Oyunu Kafka topicleri, Go engine instance'lari, Kafka Streams validation, Schema Registry, SSE, event-driven state, information hiding ve fault tolerance kavramlarini gostermek icin kullandim. Kodda her major gereksinimin bir karsiligi var ve smoke testlerle bu davranislari dogrulayabiliyorum.
```

---

## 21. Hata Olursa Hizli Kurtarma

UI eski gorunuyorsa:

```powershell
docker compose up -d --build ui
```

Tam sistemi yeniden baslatmak icin:

```powershell
docker compose down
docker compose up -d --build
```

State karismissa ve temiz baslamak istiyorsan:

```powershell
docker compose down -v
docker compose up -d --build
```

PowerShell script policy hata verirse:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\demo-validation-k4.ps1
```

Docker Desktop kapaliysa:

```text
Docker Desktop'i ac, engine tamamen baslayana kadar bekle, sonra docker compose komutunu tekrar calistir.
```
