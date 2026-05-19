# Sonnet / Learning Notes Archive

Bu dosya eskiden uzun ve ham bir AI sohbet export'u iceriyordu. O export hem
okunmasi zor oldugu hem de bugunku kod durumuna gore eski/yanlis iddialar
tasidigi icin burada kisa arsiv ozetine indirildi.

Guncel source of truth icin su dosyalara bak:

- `TermProject_RingOfTheMiddleEarth.md`: resmi odev metni.
- `README.md`: hizli baslangic ve guncel durum.
- `CALISTIRMA_REHBERI.md`: calistirma ve test adimlari.
- `SUNUM_REHBERI.md`: demo akisi ve Q&A.
- `TEKNOLOJI_EGITIM.md`: teknoloji anlatimi.
- `architecture-document.md`: mimari, tradeoff ve rubric evidence.

## Kalici Dersler

- Kafka, bu projede kalici event log ve servisler arasi entegrasyon katmanidir.
- Go engine HTTP, SSE, turn processing, side-specific state ve demo API'lerini tasir.
- Kafka Streams validation ve route-risk enrichment icin kullanilir.
- Bilgi asimetrisi en kritik oyun kuralidir: Light Ring Bearer konumunu bilir, Dark dogrudan bilmez.
- `game.session` compacted topic son snapshot'i tutar, ancak Go runtime cache'in full replay recovery'si production hardening olarak kalir.
- GameOver icin uygulama seviyesinde duplicate suppression vardir; tum crash noktalarini kapsayan transactional producer iddiasi yoktur.

## Neden Kisaltildi?

Eski export icinde:

- Mojibake karakterler vardi.
- Henuz yazilmamis dosyalar "hazir" gibi anlatiliyordu.
- Bazi komutlar ve Make target'lari bugunku repo ile uyusmuyordu.
- "Tum state Kafka KTable'da" ve "Go producer idempotent transactional" gibi fazla guclu iddialar geciyordu.

Bu nedenle belge, resmi teslim dokumani yerine sadece arsiv notu olarak tutuldu.
