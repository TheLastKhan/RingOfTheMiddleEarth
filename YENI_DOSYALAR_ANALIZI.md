# Proje Dokuman Haritasi

Bu dosya repo icindeki Markdown belgelerinin guncel rolunu ozetler.
`TermProject_RingOfTheMiddleEarth.md` resmi odev metnidir ve bu guncellemede
bilerek degistirilmemistir.

## Dokumanlar

| Dosya | Rol |
| --- | --- |
| `TermProject_RingOfTheMiddleEarth.md` | Resmi proje gereksinimleri. Kaynak dokuman. |
| `README.md` | Hizli baslangic, servisler, endpointler ve son test durumu. |
| `CALISTIRMA_REHBERI.md` | Projeyi bastan sona ayaga kaldirma ve test etme runbook'u. |
| `SUNUM_REHBERI.md` | Demo akisi, Q&A cevaplari ve sunum komutlari. |
| `TEKNOLOJI_EGITIM.md` | Kafka, Go, SSE, Docker ve mimari kavramlarinin sade anlatimi. |
| `architecture-document.md` | Mimari kararlar, sinirlar, rubric evidence ve LLM usage log. |
| `sonnet.md` | Eski uzun AI notlarinin arsiv ozeti; source of truth degildir. |

## Kod ve Konfigurasyon Haritasi

| Yol | Icerik |
| --- | --- |
| `option-b/` | Go implementation: API, game logic, router, cache, pipeline, tests. |
| `kafka/streams/` | Java Kafka Streams validation/enrichment uygulamasi. |
| `kafka/schemas/` | Avro semalari ve schema evolution dosyalari. |
| `kafka/init/` | Topic olusturma scriptleri. |
| `config/` | Map, unit ve oyun konfigurasyonlari. |
| `nginx/` | Load balancer konfigurasyonu. |
| `scripts/` | Demo/test yardimci scriptleri. |
| `index.html` | Browser UI. |
| `docker-compose.yml` | Tum servislerin orkestrasyonu. |

## Guncel Kanitlar

Son genel kontrolde:

- Go testleri gecti.
- Kafka Streams validation script'i 9 test, 0 failure ile gecti.
- Docker Compose full stack ayaga kalkti.
- Schema Registry 10 subject gosterdi.
- `game.orders.validated-value` schema versiyonlari `[1,2]` olarak goruldu.
- `game.session-value` schema versiyonu `[1]` olarak goruldu.
- `/health`, `/analysis/routes`, `/analysis/intercept` ve pprof endpointleri cevap verdi.
- `go-engine-2` durdurulunca nginx uzerinden health istekleri basarili kaldi.

## Source of Truth Sirasi

Bir konuda celiski olursa su sirayla guven:

1. `TermProject_RingOfTheMiddleEarth.md`
2. Calisan kod ve testler
3. `architecture-document.md`
4. `README.md` ve `CALISTIRMA_REHBERI.md`
5. Sunum/egitim notlari
6. `sonnet.md` arsiv ozeti

## Bilerek Acik Yazilan Sinirlar

Proje demo icin guclu durumdadir, ancak asagidaki kisimlar production seviyesi
olarak iddia edilmez:

- Go engine runtime cache'i tamamen Kafka replay ile rebuild eden hardening.
- Go producer icin tum crash noktalarini kanitlayan transactional exactly-once.
- Butun validation'in yalnizca Kafka Streams uzerinden gectigi tertemiz ayrim.

Bu sinirlar gizlenmemis, `README.md` ve `architecture-document.md` icinde acikca
belirtilmistir.
