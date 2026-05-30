# Ring of the Middle Earth - Video Demo Script

Bu dosya videoda okunabilecek detayli anlatim metnidir. Amac sadece oyunu gostermek degil; hocanin projeyle olctugu Kafka, dagitik sistem, event-driven state, validation, fault tolerance, information hiding, analysis pipeline, turn processing, SSE ve Docker multi-instance kavramlarini anladigimizi gostermektir.

Not: Metni birebir okuyabilirsin. Parantez icindeki "Ekranda goster" kisimlari video cekimi sirasinda yapacagin aksiyonlardir.

---

## 0. Acilis

Merhaba hocam. Bu videoda Ring of the Middle Earth term projesini anlatacagim.

Bu proje icin teknoloji secimi olarak Option B, yani Go + Kafka yolunu sectim. Projede iki oyunculu, turn-based bir strateji oyunu var. Bir oyuncu Light Side, digeri Shadow Side olarak oynuyor. Light Side'in amaci Ring Bearer yani Frodo'yu The Shire'dan Mount Doom'a goturup yuzugu yok etmek. Shadow Side'in amaci ise Ring Bearer'i bulmak, yollarini kapatmak, yakalamak veya yok etmek.

Ama projenin asil amaci sadece oyun yapmak degil. Bu oyun uzerinden dagitik sistem konularini gostermek:

- Kafka topic ve event altyapisi
- Order validation
- Event-driven game state
- Fault tolerance
- Information hiding
- Analysis pipelines
- Turn processing
- Browser UI ve SSE
- Docker ile coklu Go instance calistirma
- Kodun ve mimarinin aciklanabilir olmasi

Bu videoda once sistemi nasil calistirdigimi gosterecegim. Sonra iki browser uzerinden Light ve Shadow taraflarini acacagim. Daha sonra oyun akisini, orderlarin nasil isledigini, Kafka tarafini, SSE eventlerini, bilgi gizlemeyi, route/intercept analysis endpointlerini ve fault tolerance davranisini anlatacagim. Son olarak da kodda bu islerin nerede yapildigini gosterecegim.

---

## 1. Projeyi Calistirma

Ekranda goster:

```powershell
cd C:\Users\hakan\termproject
docker compose up -d --build
docker compose ps
```

Burada Docker Compose ile tum sistemi ayaga kaldiriyorum. Tek bir uygulama calismiyor; birden fazla servis beraber calisiyor.

Ana servisler sunlar:

- `rotr-ui`: Browser arayuzu. `localhost:3000` uzerinden aciliyor.
- `rotr-nginx`: Go engine instance'lari icin load balancer. API tarafinda `localhost:80` uzerinden calisiyor.
- `rotr-go-1`, `rotr-go-2`, `rotr-go-3`: Uc adet Go game engine instance'i.
- `rotr-kafka-1`, `rotr-kafka-2`, `rotr-kafka-3`: Kafka brokerlari.
- `rotr-schema-registry`: Avro schema registry.
- `rotr-kafka-streams`: Kafka Streams validation ve enrichment servisi.
- `rotr-zookeeper`: Kafka coordination icin kullaniliyor.

Bu mimarinin ana fikri su: Browser dogrudan tek bir oyun serverina baglanmiyor. UI, HTTP ve SSE isteklerini nginx'e gonderiyor. Nginx de bu istekleri uc Go engine instance'indan birine yonlendiriyor. Oyun emirleri ve eventler Kafka uzerinden akiyor.

Ekranda goster:

```powershell
Invoke-RestMethod http://localhost/health
```

Bu health endpoint'i Go engine'lerin ayakta oldugunu ve mevcut turn bilgisini gosteriyor. Burada `status: ok` donmesi sistemin calistigini gosterir.

Schema Registry'yi kontrol etmek icin:

```powershell
Invoke-RestMethod http://localhost:8081/subjects
```

Burada Kafka topiclerinde kullanilan Avro schema subject'lerini goruyoruz. Proje dokumaninda Schema Registry ve schema evolution istendigi icin bu kisim onemli.

---

## 2. Iki Tarafi Browserda Acma

Ekranda iki browser penceresi ac:

Light:

```text
http://localhost:3000?side=light
```

Shadow:

```text
http://localhost:3000?side=dark
```

Light tarafinda oyuncu adini giriyorum ve Free Peoples tarafini seciyorum. Shadow tarafinda da oyuncu adini girip Shadow tarafini seciyorum.

Bu noktada iki farkli oyuncunun ayni oyuna farkli bilgiyle baktigini gosterecegim. Harita ayni, unitler ayni sistemde, ama Ring Bearer konumu bilgi gizleme kuralina gore farkli davranmali.

Light Side icin Frodo'nun konumu gorulebilir. Shadow Side icin ise Ring Bearer'in gercek konumu normalde gorunmemeli. Shadow sadece detection event'i olursa bilgi alabilir.

Bu projenin en onemli kurallarindan biri information asymmetry, yani bilgi asimetrisidir.

---

## 3. Oyun Kurallarini Anlatma

Oyunda iki taraf var.

Light Side'in amaci:

1. Ring Bearer'a bir rota atamak.
2. Frodo'nun her turn sonunda bu rota uzerinde ilerlemesini saglamak.
3. Mount Doom'a ulastiginda `Destroy Ring` order'i vermek.
4. Mount Doom'da aktif Shadow unit yoksa Light kazanir.

Shadow Side'in amaci:

1. Nazgul unitlerini stratejik bolgelere veya path endpointlerine gondermek.
2. Pathleri block/search etmek.
3. Ring Bearer'i exposed hale getirmek.
4. Ring Bearer exposed iken ayni region'da yakalamak.

Oyun maksimum 40 turn surer. 40 turn sonunda kazanan yoksa beraberlik olur.

Ilk 3 turn detection kapali. Yani Shadow tarafinda Nazgul yakin olsa bile Ring Bearer tespit edilmez. Detection Turn 4'ten sonra aktif olur.

---

## 4. Turn Processing Nedir?

Ekranda `End Turn` butonunu goster.

Bu oyunda emir verdigimizde unit hemen hareket etmiyor. Emir once sisteme kaydediliyor. Sonra `End Turn` butonuna basinca veya turn timer bitince oyun motoru 13 adimlik turn processing sirasini calistiriyor.

Bu sirayi kodda gormek icin:

Ekranda goster:

```text
option-b/internal/game/turn.go
```

Bu dosyada `ProcessTurn` fonksiyonu var. Turn sonunda su adimlar calisiyor:

1. Validated orderlar toplanir.
2. AssignRoute ve RedirectUnit islenir.
3. BlockPath ve SearchPath islenir.
4. ReinforceRegion ve DeployNazgul islenir.
5. FortifyRegion islenir.
6. Maia abilities islenir.
7. Route atanmis unitler otomatik ilerler.
8. Combat cozulur.
9. Path timerlari guncellenir.
10. Fortification timerlari azalir.
11. Respawn ve cooldown timerlari azalir.
12. Ring detection hesaplanir.
13. Win condition kontrol edilir.

Bu sira onemli. Mesela Ring Bearer'a route atandiktan sonra auto-advance adiminda bir sonraki path'e ilerler. Detection ise hareketten sonra hesaplanir. Win condition ise en sonda kontrol edilir.

---

## 5. Light Side Gameplay: Frodo'ya Route Atama

Ekranda Light browser'a gec.

Simdi Frodo'yu seciyorum. Sol panelde Frodo Baggins'i tikliyorum.

Sonra `Assign Route` seciyorum.

Haritada hedef olarak Mount Doom'a tikliyorum.

Burada UI otomatik olarak path listesini dolduruyor. Bu path listesi Frodo'nun izleyecegi route. Ozellikle Mount Doom hedefinde Light Side icin Mordor uzerinden gecmek riskli oldugu icin otomatik route Cirith Ungol tarafini tercih edecek sekilde ayarlandi.

Ornek beklenen rota:

```text
shire-to-bree,
bree-to-rivendell,
rivendell-to-lothlorien,
lothlorien-to-emyn-muil,
emyn-muil-to-ithilien,
ithilien-to-cirith-ungol,
cirith-ungol-to-mount-doom
```

Burada rota order payload'i olarak backend'e gidecek.

Sonra `Issue Order` butonuna basiyorum. Ardindan `End Turn` ile turn'u isliyorum.

Turn bitince Frodo route uzerinde bir adim ilerliyor. UI'da event log'da unit movement eventini goruyoruz. Ornegin:

```text
Frodo Baggins moved from The Shire to Bree
```

Bu event backend'de `game.events.unit` eventidir.

---

## 6. Order Sistemi: Browserdan Kafka'ya Akis

Simdi teknik olarak bir order'in nereden nereye gittigini anlatayim.

Oyuncu UI'da `Issue Order` dediginde browser `POST /order` endpoint'ine istek atiyor.

Ekranda goster:

```text
index.html
submitOrder()
```

Frontend burada order body olusturuyor:

```json
{
  "orderType": "ASSIGN_ROUTE",
  "playerId": "...",
  "playerSide": "FREE_PEOPLES",
  "unitId": "ring-bearer",
  "turn": 1,
  "payload": {
    "pathIds": [...]
  }
}
```

Bu istek Go API tarafinda su dosyaya geliyor:

```text
option-b/internal/api/server.go
handleOrder()
```

Burada once order normalize ediliyor. Sonra validation yapiliyor.

Ekranda goster:

```text
option-b/internal/validation/validator.go
Validate()
```

Validation'in amaci su:

- Bu unit bu oyuncuya ait mi?
- Turn numarasi dogru mu?
- Ayni unit'e ayni turn icinde ikinci order verildi mi?
- Path var mi?
- Path blocked mi?
- Maia ability cooldown uygun mu?
- Target region valid mi?

Order gecersizse hata donuyor. Gecerliyse `OrderCh` kanalina koyuluyor.

Sonra main loop tarafinda bu order Kafka topiclerine yaziliyor:

```text
game.orders.raw
game.orders.validated
```

Bu proje dokumaninda Kafka order akisi istendigi icin bu kisim onemli. Order sadece UI'da kalmiyor; Kafka event altyapisina dahil oluyor.

---

## 7. Kafka Topic/Event Altyapisi

Ekranda topic olusturma dosyasini goster:

```text
kafka/init/create-topics.sh
```

Bu projede Kafka bir event backbone olarak kullaniliyor. Yani sistemdeki onemli olaylar Kafka topiclerine yaziliyor.

Baslica topicler:

- `game.orders.raw`: Oyuncudan gelen ham orderlar.
- `game.orders.validated`: Dogrulanmis orderlar ve route risk bilgileri.
- `game.events.unit`: Unit hareket, respawn, hasar gibi eventler.
- `game.events.region`: Region kontrol ve battle eventleri.
- `game.events.path`: Path block/open/corruption eventleri.
- `game.session`: Compacted session snapshot. Mevcut world state'in son halini tutar.
- `game.broadcast`: UI'a giden world snapshot ve global eventler.
- `game.ring.position`: Light-only Ring Bearer position eventleri.
- `game.ring.detection`: Shadow tarafina giden detection eventleri.
- `game.dlq`: Invalid orderlar.

Bu tasarimda Kafka sadece mesaj gecirme araci degil; ayni zamanda replay/recovery ve audit trail icin de kullaniliyor.

---

## 8. Kafka Streams Validation ve Schema Evolution

Proje dokumaninda Kafka Streams validation topology isteniyor.

Ekranda goster:

```text
kafka/streams/src/main/java/rotr/streams/OrderValidator.java
kafka/streams/src/test/java/rotr/streams/OrderValidatorTest.java
```

Burada validation kurallarinin testleri var. Demo icin bu test scripti kullanilabilir:

```powershell
.\scripts\demo-validation-k4.ps1
```

Beklenen sonuc:

```text
Tests run: 9, Failures: 0, Errors: 0
BUILD SUCCESS
```

Schema Registry icin:

```powershell
Invoke-RestMethod http://localhost:8081/subjects
Invoke-RestMethod http://localhost:8081/subjects/game.orders.validated-value/versions
```

Burada `game.orders.validated-value` icin version 1 ve version 2 gorunmeli. Version 2'de nullable/default alanlar var:

- `routeRiskScore`
- `threatenedPaths`
- `blockedPaths`

Bu schema evolution demek. Yani yeni alanlar ekleniyor ama eski consumerlar bozulmadan okumaya devam edebiliyor.

---

## 9. Event-Driven Game State

Bu oyunda state dogrudan UI'dan degistirilmiyor. Her sey order ve event zinciriyle ilerliyor.

Akis soyle:

```text
Browser order verir
Go API order'i alir
Validation yapilir
Order Kafka'ya yazilir
TurnProcessor turn sonunda order'i uygular
UnitMoved / PathChanged / WorldStateSnapshot eventleri uretilir
Kafka'ya ve SSE'ye gider
UI state'i yeniler
```

Kodda turn sonunda eventlerin uretildigi yer:

```text
option-b/internal/game/turn.go
ProcessTurn()
```

Eventlerin publish edildigi yer:

```text
option-b/cmd/server/main.go
publishEvent()
```

World snapshot hem `game.broadcast` topicine hem de `game.session` topicine gidiyor. `game.session` compacted topic olarak son state'i tutuyor. Bu da restart veya replay icin kullaniliyor.

---

## 10. Browser UI ve SSE

Ekranda `index.html` dosyasini goster.

UI vanilla JavaScript ile yapildi. React veya Vue kullanilmiyor; proje dokumaninda Vanilla JS + SSE isteniyor.

Browser tarafinda SSE baglantisi su endpoint'e aciliyor:

```text
GET /events
```

Kodda:

```text
index.html
connectSSE()
```

SSE yani Server-Sent Events, server'in browser'a canli event gondermesini saglar. Browser surekli "state degisti mi?" diye sormak zorunda kalmaz. Server event geldikce haber verir.

UI su eventleri dinliyor:

- `game.broadcast`
- `game.ring.position`
- `game.ring.detection`
- `game.events.unit`
- `game.events.path`
- `game.events.region`

Son yaptigimiz duzeltmelerden biri de burada. Backend SSE eventlerini su sekilde sararak gonderiyor:

```json
{
  "topic": "game.events.unit",
  "data": {
    "unitId": "ring-bearer",
    "from": "the-shire",
    "to": "bree"
  }
}
```

Frontend artik bu `data` zarfini aciyor ve event log'da anlamli mesaj yaziyor:

```text
Frodo Baggins moved from The Shire to Bree
```

Eskiden burada `undefined` gorunebiliyordu, cunku frontend payload'i bir seviye yanlis okuyordu.

---

## 11. Information Hiding: Light ve Shadow Farki

Bu projede bilgi gizleme cok kritik.

Light Side Ring Bearer'in gercek konumunu gorebilir. Shadow Side gorememeli.

Kodda bu ayrim iki yerde yapiliyor:

```text
option-b/internal/router/event_router.go
option-b/internal/api/server.go
```

EventRouter tarafinda:

- `game.ring.position` sadece Light SSE kanalina gider.
- `game.ring.detection` sadece Shadow SSE kanalina gider.
- `game.broadcast` iki tarafa da gider ama Dark icin Ring Bearer konumu temizlenir.

Bu nedenle Shadow browser'da Frodo'nun gercek konumunu normalde gormemeliyiz.

API ile de gosterebilirim:

```powershell
Invoke-RestMethod "http://localhost/game/state?playerId=light-player&side=FREE_PEOPLES"
Invoke-RestMethod "http://localhost/game/state?playerId=dark-player&side=SHADOW"
```

Light response'unda Ring Bearer region gorunurken, Dark response'unda bu alan bos olmali veya gizlenmis olmali.

Bu hocanin Q&A'da sorabilecegi en onemli noktalardan biri:

"Ring Bearer konumu Dark tarafindan nasil gizleniyor?"

Cevap:

"EventRouter ve API serializer tarafinda. Light-only eventler Shadow kanalina hic gonderilmiyor. Broadcast state Dark'a giderken Ring Bearer currentRegion temizleniyor."

---

## 12. Analysis Pipelines

Projede iki analysis endpoint'i var.

Light icin:

```text
GET /analysis/routes
```

Shadow icin:

```text
GET /analysis/intercept
```

Ekranda goster:

```powershell
Invoke-RestMethod http://localhost/analysis/routes
Invoke-RestMethod http://localhost/analysis/intercept
```

Route analysis, canonical route'lari risk skoruna gore siraliyor. Risk hesabi threat level, surveillance level, blocked path, threatened path ve Nazgul yakinligi gibi degerlerle yapiliyor.

Kodda:

```text
option-b/internal/pipeline/pipeline.go
ComputeRouteRisk()
```

Intercept analysis ise Shadow icin Nazgul'larin hangi bolgede Ring Bearer'i yakalama ihtimalinin yuksek oldugunu hesapliyor.

Kodda:

```text
option-b/internal/pipeline/pipeline.go
ComputeInterception()
```

UI'da `Analyse Routes` butonu bu endpointleri cagiriyor. Light tarafinda best route, Shadow tarafinda best intercept gosteriliyor.

Bu pipeline'lar hocanin istedigi "analysis endpoints" maddesini karsiliyor.

---

## 13. Shadow Side Gameplay

Ekranda Shadow browser'a gec.

Shadow tarafinda Witch-King veya Nazgul unitlerinden birini seciyorum.

Shadow'un amaci Frodo'yu bulmak. Bunun icin:

- Nazgul'lari chokepointlere gonderebilir.
- Search Path yapabilir.
- Block Path yapabilir.
- Saruman ile Maia ability kullanabilir.
- Sauron pasif olarak detection gucunu destekler.

Ornegin Nazgul'lari Cirith Ungol, Minas Morgul, Mordor gibi stratejik bolgelere yonlendirmek mantikli. Light tarafinin Mount Doom'a ulasmasi icin bu bolgelerden gecmesi gerekir.

Burada asil anlatmak istedigim sey su: Shadow oyuncusu normalde Frodo'nun nerede oldugunu kesin olarak bilmez. Analiz endpointleri ve detection eventleri ona tahmin/strateji uretir.

---

## 14. Path Mechanics ve Maia Ability

Path mekanigi su:

- Path `OPEN` ise unit gecilebilir.
- Path `BLOCKED` ise Ring Bearer o path'ten gecemez.
- Gandalf `MAIA_ABILITY` ile blocked path'i 2 turn boyunca `TEMPORARILY_OPEN` yapabilir.
- Saruman bazi pathleri corrupt/block etkisiyle daha tehlikeli hale getirebilir.

Kodda:

```text
option-b/internal/game/turn.go
step6ProcessMaiaAbilities()
step9UpdatePathTimers()
```

Bu belgeye gore Gandalf'in yetenegi gecici olmalidir; 2 turn sonra path eski durumuna doner. Saruman'in etkisi ise Shadow tarafinin path kontrolu icin kullanilir.

Gandalf ve Saruman'in ayni `MAIA_ABILITY` order tipini kullanmasi ama config'e gore farkli davranmasi bekleniyor. Bu da hocanin sorabilecegi Q&A sorularindan biri.

---

## 15. Combat ve Win Condition

Combat bolge uzerinden cozuluyor.

Combat gucu:

```text
unit strength
+ terrain bonus
+ fortification bonus
+ leadership bonus
```

Aragorn leadership bonusu verebilir. Gondor Army fortify yapabilir. Terrain de savunma/saldiri etkileyebilir.

Light win condition:

```text
Ring Bearer Mount Doom'da
DestroyRing order'i bu turn verildi
Mount Doom'da aktif Shadow unit yok
```

Shadow win condition:

```text
Ring Bearer exposed
Nazgul ayni region'da
```

Draw:

```text
40 turn sonunda kazanan yok
```

Kodda:

```text
option-b/internal/game/turn.go
step13CheckWinConditions()
```

GameOver event'i `game.broadcast` topicine gidiyor. Projede GameOver icin duplicate prevention ve transactional producer destegi de var.

---

## 16. Fault Tolerance

Simdi fault tolerance kismini anlatacagim.

Bu sistemde tek Go server yok. Uc Go engine instance var:

- go-engine-1
- go-engine-2
- go-engine-3

Nginx bunlarin onunde load balancer olarak duruyor.

Bir instance'i durduruyorum:

```powershell
docker compose stop go-engine-2
```

Sonra health check atiyorum:

```powershell
Invoke-RestMethod http://localhost/health
```

Sistem hala `ok` donmeli. Cunku nginx kalan engine'lere istek yonlendirebilir.

Sonra tekrar baslatiyorum:

```powershell
docker compose up -d go-engine-2
```

Bu sirada Kafka topicleri eventleri ve session snapshotlarini tuttugu icin yeniden baslayan engine state'i Kafka'dan yakalayabilir.

Kodda session replay:

```text
option-b/cmd/server/main.go
pollSessionSnapshots()
game.session
```

Burada durust olmak gerekir: Bu demo seviyesinde Kafka-backed session recovery sagliyor. Tam production seviyesinde her crash boundary icin exhaustive replay/correctness ispatlamak daha buyuk bir is olur. Ama proje demosu icin Kafka topicleri, compacted session snapshot ve engine failover davranisi gosteriliyor.

---

## 17. End Turn ve End Game

UI'da iki buton var:

- `End Turn`
- `End Game`

`End Turn`, mevcut turn'u manuel olarak isler. Bu videoda demo icin 60 saniye beklememek adina kullaniliyor.

Kodda:

```text
index.html
advanceTurn()

option-b/internal/api/server.go
handleAdvanceTurn()
```

`End Game`, oyunu Turn 1'e resetlemek icin kullaniliyor.

Kodda:

```text
index.html
endGame()

option-b/internal/api/server.go
handleGameStart()
```

Backend tarafinda reset `game/start` uzerinden yapiliyor. Son duzeltmede EventRouter kanallari bloklamayacak hale getirildi, cunku dolu SSE/cache kanallari engine loop'unu kilitleyebiliyordu. Artik reset daha guvenilir sekilde Turn 1'e donuyor.

Kodda:

```text
option-b/internal/router/event_router.go
sendEvent()
```

Burada non-blocking send kullaniliyor. Boylece bir event kanali dolu olsa bile engine loop tamamen kilitlenmiyor.

---

## 18. Tests ve Kanitlar

Projede sadece elle demo yok, test ve smoke scriptleri de var.

Go testleri:

```powershell
cd option-b
go test ./...
```

Bu testler game logic, router, pipeline gibi Go taraflarini kontrol ediyor.

Kafka Streams validation:

```powershell
.\scripts\demo-validation-k4.ps1
```

GameOver idempotency / exactly-once smoke:

```powershell
.\scripts\check-gameover-idempotency.ps1
.\scripts\check-gameover-idempotency-dark.ps1
```

Ilk test Light victory icin, ikinci test Dark victory icin ayni exactly-once kontrolunu yapar.

Full E2E smoke:

```powershell
.\scripts\full-e2e-smoke.ps1
```

Chaos/failover smoke:

```powershell
.\scripts\chaos-soak-smoke.ps1
```

Browser smoke:

```powershell
.\scripts\browser-smoke.ps1
```

Bu scriptler hocaya projenin sadece UI'da calismadigini, backend ve distributed sistem tarafinda da test edildigini gostermek icin kullanilabilir.

---

## 19. Kod Dosyalari Uzerinden Hoca Icin Harita

Videoda kodu gostereceksem su sirayla gostermek mantikli:

1. `docker-compose.yml`
   - Tum servislerin nasil ayaga kalktigini gosterir.
   - Kafka, Go engines, nginx, UI, Schema Registry burada.

2. `kafka/init/create-topics.sh`
   - Topicler ve schema registration burada.

3. `index.html`
   - Browser UI, map canvas, order submit, SSE client burada.

4. `option-b/internal/api/server.go`
   - HTTP endpointler: `/order`, `/game/state`, `/events`, `/analysis/routes`, `/analysis/intercept`.

5. `option-b/internal/validation/validator.go`
   - Order validation kurallari.

6. `option-b/cmd/server/main.go`
   - Ana select loop, turn timer, Kafka publish, session replay.

7. `option-b/internal/game/turn.go`
   - 13-step turn processing ve win conditions.

8. `option-b/internal/router/event_router.go`
   - Information hiding ve SSE routing.

9. `option-b/internal/pipeline/pipeline.go`
   - Route risk ve intercept analysis.

10. `architecture-document.md`
   - Mimari kararlar, tradeofflar ve rubric evidence.

---

## 20. Hocanin Ogrenmemizi Istedigi Seylere Karsilik Gelen Cevaplar

Kafka topic/event altyapisi:

Bu projede orderlar, unit eventleri, path eventleri, region eventleri, world snapshotlari ve Ring Bearer detection eventleri Kafka topicleri uzerinden akiyor. Kafka sistemin event backbone'u.

Order validation:

Oyuncu orderlari direkt uygulanmiyor. Once ownership, turn, duplicate, path, target, cooldown gibi kurallarla validate ediliyor.

Event-driven game state:

Oyun state'i order ve eventlerin sonucunda guncelleniyor. TurnProcessor event uretiyor; WorldStateSnapshot UI'a gidiyor.

Fault tolerance:

Uc Go engine instance var. Nginx load balancing yapiyor. Kafka topicleri ve `game.session` snapshotlari state recovery icin kullaniliyor.

Information hiding:

Light, Ring Bearer position eventini aliyor. Shadow almiyor. Dark state icinden Ring Bearer currentRegion temizleniyor.

Analysis pipelines:

`/analysis/routes` Light icin route risk hesapliyor. `/analysis/intercept` Shadow icin Nazgul yakalama plani hesapliyor.

Turn processing:

End Turn ile 13 adimlik sabit oyun motoru sirasi calisiyor.

Browser UI + SSE:

UI browserda calisiyor. SSE ile server eventleri canli olarak browser'a geliyor.

Docker ile coklu instance:

Docker Compose ile 3 Go engine, 3 Kafka broker, Schema Registry, Kafka Streams, nginx ve UI birlikte calisiyor.

Aciklanabilir mimari:

Her major gereksinimin kodda bir karsiligi var. Hangi endpoint nerede, hangi topic ne ise yariyor, hangi event hangi tarafa gidiyor, hangi test neyi kanitliyor gosterebiliyorum.

---

## 21. Kapanis

Ozetlemek gerekirse bu projede iki oyunculu turn-based bir oyun yaptim ama asil hedef dagitik sistem konseptlerini oyun uzerinden gostermekti.

Browser tarafinda iki oyuncu var. UI order gonderiyor. Go API order'i aliyor, validation yapiyor ve Kafka'ya/event akisia dahil ediyor. TurnProcessor her turn sonunda oyun kurallarini sabit sirayla isliyor. Eventler Kafka topiclerine ve SSE ile browserlara gidiyor. EventRouter Light ve Shadow arasinda bilgi gizlemeyi sagliyor. Analysis endpointleri route risk ve intercept planlari hesapliyor. Docker Compose ile coklu Go instance ve Kafka altyapisi birlikte calisiyor. Fault tolerance icin nginx, Kafka topicleri ve session snapshotlari kullaniliyor.

Bu nedenle proje sadece "oyun ekraninda hareket eden unitler" degil; Kafka, Go concurrency, SSE, validation, information hiding, event-driven architecture ve fault tolerance kavramlarini bir araya getiren bir distributed application olarak tasarlandi.

Videoda gosterdigim testler ve kod referanslari da bu mimarinin calistigini ve hocanin rubrikte istedigi basliklari karsiladigini gostermek icin kullanildi.

Tesekkur ederim.

---

## 22. Video Cekimi Icin Kisa Checklist

Videoya baslamadan once:

```powershell
cd C:\Users\hakan\termproject
docker compose up -d --build
docker compose ps
Invoke-RestMethod http://localhost/health
```

Iki browser:

```text
http://localhost:3000?side=light
http://localhost:3000?side=dark
```

Gosterilecek komutlar:

```powershell
Invoke-RestMethod http://localhost:8081/subjects
Invoke-RestMethod http://localhost/analysis/routes
Invoke-RestMethod http://localhost/analysis/intercept
.\scripts\browser-smoke.ps1
cd option-b
go test ./...
cd ..
```

Fault tolerance:

```powershell
docker compose stop go-engine-2
Invoke-RestMethod http://localhost/health
docker compose up -d go-engine-2
```

Kodda gosterilecek dosyalar:

```text
index.html
option-b/internal/api/server.go
option-b/internal/validation/validator.go
option-b/internal/game/turn.go
option-b/internal/router/event_router.go
option-b/internal/pipeline/pipeline.go
option-b/cmd/server/main.go
kafka/init/create-topics.sh
architecture-document.md
```
