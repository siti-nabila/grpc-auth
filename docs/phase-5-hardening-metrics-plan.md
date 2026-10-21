# Plan implementasi Phase 5 — Hardening dan metrics lokal

Tanggal: 20 Oktober 2026. Status implementasi: **COMPLETE**; lihat
[evidence Phase 5](../openspec/changes/archive/2026-10-20-auth-phase-5-hardening-metrics/evidence/progress.md).
Owner: backend `grpc-auth`. Bagian baseline merekam kondisi sebelum implementasi.

Keputusan user 20 Oktober 2026: service admission baru 600 request/menit burst
100 per verified service untuk seluruh 37 RPC, termasuk Logout/revoke, sesudah
Service dan sebelum Attempt/Session. Stage ini terpisah, tidak direfund ketika
downstream menolak. Existing logger sinkron dipakai untuk log periodik; shutdown
boleh menunggu write selesai. Keputusan ini menggantikan usulan awal terkait
quota baru dan cancellation sink. Change diarsipkan: `2026-10-20-auth-phase-5-hardening-metrics`.

## 1. Tujuan dan batas phase

Melengkapi observasi lokal dan menutup gap hardening yang terbukti, menggunakan
limiter, audit, pool database, logger, dan lifecycle existing. Alur bisnis harus
tetap mudah dibaca; tidak membuat framework observability atau command baru.

Sumber keputusan:

- [Spec MVP bagian 8](grpc-auth-mvp-spec.md#8-rate-limiter-local-mvp): quota,
  atomic admission, trusted IP, cardinality, dan metrics minimum.
- [M-10/M-11](grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence): limiter,
  independent failed attempt, redaksi, no-op/replay, dan pending outbox.
- [Roadmap Phase 5/6](grpc-auth-mvp-spec.md#10-tahapan-dependencies-dan-syarat-selesai).
- [Hasil Phase 4](auth-mutation-audit-reconciliation-plan.md#9-hasil-implementasi)
  dan [evidence reconciliation](../openspec/changes/archive/2026-10-18-auth-mutation-audit-reconciliation/evidence/progress.md).

Deliverable Phase 5: metrics yang dapat dibaca secara lokal, resource observation
yang terbatas, gap ledger M-10/M-11, regression evidence, serta petunjuk membaca
counter/gauge. Historical evidence Phase 4 adalah baseline, bukan PASS baru.

Di luar scope: REST/UI, actual proxy sanitization pada consumer, deployment,
rotation credential environment target, Redis/distributed quota, RabbitMQ relay,
CLM, Prometheus/Grafana/OpenTelemetry, perubahan delapan quota identity existing, RPC/schema/error
publik baru, migration, dan dependency update. Phase 6 DEP-01 tetap mandatory
dan terpisah; metrics pending outbox tidak membuktikan delivery broker.

## 2. Baseline source yang sudah diperiksa

Module: `github.com/siti-nabila/grpc-auth`, Go `1.26.2`.
HEAD/graph: `2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec`.
Working tree memiliki banyak perubahan increment sebelumnya; identitas HEAD
tidak mewakili seluruh source saat ini. Capture digest aman saat implementasi.
Tidak ada active change OpenSpec pada saat plan dibuat.

Dependency pinned saat inspeksi: `api-contracts
v0.0.0-20261018073250-b4ad807ad5fa`, `orm v1.8.0-support-subquery`.
Plan tidak mengubah keduanya.

| Boundary | Source existing | Kondisi dan arah pekerjaan |
| --- | --- | --- |
| Limiter | [store.go](../internal/ratelimit/store.go) | Delapan group, atomic layered admission, mutex, controlled clock, TTL/cap/cleanup; `Counts()` berisi quota/capacity rejection dan entries. Reuse, jangan membangun store kedua. |
| Admission HTTP caller/gRPC | [rate_interceptor.go](../internal/interceptors/rate_interceptor.go), [refresh_admission.go](../internal/interceptors/refresh_admission.go) | Verified caller/IP fallback, HMAC account, verified user dan proven refresh session; RetryInfo sudah tersedia. |
| Failed attempt | [security_attempt.go](../internal/interceptors/security_attempt.go), [login_failure.go](../internal/features/auth_feature/login_failure.go), [common/security_attempt.go](../internal/features/common/security_attempt.go) | Independent transaction, cleanup-before-audit, rate rejection tanpa DB audit, duplicate Login/reuse prevention. Generic attempt memakai timeout 2 detik; path Login perlu direkonsiliasi tersendiri. |
| Audit failure counter | [login_producer.go](../internal/configs/login_producer.go), [foundation.go](../internal/configs/foundation.go) | `AuditFailures()` memakai atomic counter untuk failed Login dan generic attempt. Belum membuktikan seluruh required-audit persistence failure terobservasi. |
| Pipeline/lifecycle | [server.go](../internal/configs/server.go), [foundation.go](../internal/configs/foundation.go), [config.go](../internal/configs/config.go) | Language → Error → Service → Attempt → Session → Rate. Cleanup worker dimiliki server; cancel/join sebelum pool ditutup. |
| Outbox | [domain/outbox.go](../internal/repositories/domain/outbox.go), [writer](../internal/repositories/writer/outbox.writer.go) | Pending record memakai `published_at` kosong. Belum ditemukan reader aggregate backlog/oldest age pada source yang diperiksa. |
| Database/logger | [postgres.go](../pkg/database/postgres.go), [logger](../pkg/logger/logger.go) | Reuse primary `*sql.DB` dan logger existing; statistik pool tersedia melalui `DB.Stats()`. Tidak membuat koneksi baru. |

Belum ditemukan package metrics/telemetry dedicated pada `internal/` atau `pkg/`.
Ini hasil inspeksi source, bukan bukti bahwa semua skenario runtime gagal.
Graph dipakai untuk routing; hub domain auth/outbox/user-role/snapshot/transaction
diverifikasi lewat source dan tidak diduplikasi oleh collector.

## 3. Keputusan desain untuk implementasi

### 3.1 Metrics lokal dan ownership

Gunakan satu instance metrics process-local yang dibuat pada composition root,
diinjeksi ke boundary yang memakainya. Counter/histogram disimpan dengan ukuran
tetap dan concurrency-safe; tidak menyimpan request, credential, atau event
per pengguna. Package kecil `internal/telemetry` boleh dibuat karena memiliki
consumer nyata: RPC observation, audit failure, dan runtime sampler.

Pembacaan berupa typed Go snapshot dan ringkasan periodik melalui logger
existing. Tidak menambah listener, endpoint HTTP, gRPC reflection exposure,
atau public RPC untuk metrics. Usulan cadence tetap: sekali per menit, tanpa
log per request. Snapshot log harus punya jumlah field/ukuran terukur dan
urutan stabil; pisahkan menjadi jumlah record tetap bila diperlukan.

Tidak menambah runtime configuration dahulu. Interval/timeout lokal ditulis
sebagai named constant dan sumber waktu/tick diinject untuk test. Jika saat
implementasi terbukti perlu konfigurasi, gunakan loader serta contoh YAML
existing, validasi startup, dan dokumentasikan kebutuhan sebelum memperluas scope.

### 3.2 Schema metrics minimum

Nama di bawah adalah rancangan internal; final naming dicatat di evidence.
Counter kumulatif reset saat process restart. Gauge adalah snapshot, bukan
counter yang dijumlahkan ulang setiap tick.

| Metric | Bentuk dan definisi | Sumber/aturan |
| --- | --- | --- |
| RPC completed/error | Counter per method dan final gRPC code | Satu completion per invocation, termasuk service/session/rate rejection. Error code dibaca setelah encoder. |
| RPC duration | Count/sum dan histogram bucket tetap | Durasi seluruh unary pipeline termasuk independent attempt; clock terkontrol di test. Usulan batas: 1/5/10/50/100/500/1000/5000 ms dan overflow. Tidak mengklaim p95 dari rata-rata. |
| Auth outcome | Counter failure/deny/dependency | Klasifikasi catalog/code allowlist; success `EvaluateAccess allowed=false` masuk deny meskipun gRPC OK. Jangan mengubah dependency failure menjadi permission denial. |
| Rate rejection | Counter quota/capacity, optional fixed admission group | Reuse keputusan/counter limiter. Satu admission ditolak dihitung sekali walau memeriksa beberapa bucket. Refresh boundary dan generic boundary tidak menggandakan hitungan. |
| Limiter entries | Gauge process-local | Reuse `Counts().Entries`, tidak mengekspos key atau isi bucket. |
| Audit persistence failure | Counter per fixed origin | Cakup independent attempt dan required-audit write/finalization failure yang terbukti. Definisikan scope finalization; jangan menyebut setiap commit failure pasti audit row gagal. Hindari double counting callback/outer interceptor. |
| Outbox pending | Gauge jumlah record `published_at IS NULL` | Aggregate ORM di primary, termasuk pending yang retry time-nya belum jatuh tempo. Tidak membaca payload atau menandai published. |
| Outbox oldest age | Gauge `oldest_age_ns` sejak `MIN(created_at)` pending; bagi 1e9 untuk detik | Satu aggregate snapshot dengan count; nol saat kosong, minimum nol untuk clock skew. Waktu observasi UTC diinject. |
| DB pool usage | Gauge open/in-use/idle/max; cumulative wait count/duration | Reuse `sql.DB.Stats()` primary; tanpa DSN, host, database name, atau arbitrary source label. |
| Collection/export health | Counter failure dan gauge freshness | Bedakan belum tersedia, stale, dan pending=0. Kegagalan DB/log tidak mencetak raw error atau mengubah outcome bisnis. |

Method hanya berasal dari inventory registered 37 methods; arbitrary/unknown
method dipetakan ke satu `unknown`. Group dan reason memakai enum terbatas.
Status memakai himpunan gRPC code tetap. Tidak memakai user ID, email, IP,
session ID, caller ID arbitrer, token, HMAC/digest, correlation ID, request body,
permission dari input, atau `err.Error()` sebagai label/log metrics.

### 3.3 Posisi instrumentation dan failure isolation

RPC observer menjadi wrapper luar pipeline existing agar service-auth failure
dan hasil sesudah error encoder ikut tercatat. Urutan relatif Language → Error
→ Service → Attempt → Session → Rate tetap. Observer tidak mengautentikasi,
mengubah response/error/details/locale, memanggil handler dua kali, atau
mencatat ulang domain audit. Tidak menambah recovery yang mengubah panic contract;
defer observation tidak boleh menghalangi cleanup/recovery existing.

Hot path hanya memperbarui counter/histogram memori; tidak melakukan I/O log/DB,
enqueue tanpa batas, atau meluncurkan goroutine per request. Snapshot disalin
di bawah lock singkat; formatting/logging di luar lock. Observation failure
tidak mengubah original denial/error atau membuat audit/event baru.

### 3.4 Runtime sampler dan persistence boundary

Satu lifecycle-owned worker mengambil limiter/pool snapshot dan aggregate
outbox setiap menit. Jangan membuat worker/registry per feature. Collector
tidak boleh menunda cleanup limiter existing: gunakan worker terpisah yang
dimiliki server bila I/O collector dapat memblokir cleanup. Tidak boleh ada
tick overlap, backlog queue, atau retry loop tanpa batas.

Query outbox memakai reader ORM dan contract kecil milik consumer, aggregate
count/min dalam satu statement/snapshot, hanya kolom yang diperlukan. Reuse
primary pool; query memakai context worker dengan usulan timeout 2 detik dan
debug SQL dimatikan. Jangan fallback ke raw SQL atau full-table payload scan.
Nilai `next_attempt_at` tidak menyaring total pending.

Saat query gagal, pertahankan last successful sample beserta freshness/status;
sebelum sample pertama status unavailable. Jangan menerbitkan zero palsu.
Log sink failure tidak memicu log rekursif. Pada shutdown/listener/startup
failure: cancel, stop ticker, join worker, baru tutup resource/pool. Synchronous log write boleh ditunggu saat shutdown sesuai keputusan user;
jangan memakai goroutine yang ditinggal untuk membuat timeout palsu.

## 4. Hardening yang direkonsiliasi

| Area | Invariant yang dipertahankan/dibuktikan |
| --- | --- |
| Quota | Angka delapan group sesuai spec bagian 8; layered rejection tidak consume bucket lain, tidak allocate key baru, refill/concurrency deterministic. |
| Capacity | Maximum 10.000 entries; existing bucket tetap berlaku ketika penuh; idle TTL 15 menit, cleanup 1 menit; tidak evict active exhausted bucket. |
| Trust | `x-client-ip` hanya dari verified rest-orc, tepat satu IP valid tanpa zone; malformed/multiple/missing/other caller memakai verified service fallback. Identitas account/session dari bukti existing, bukan input tak terverifikasi. |
| Transport | RESOURCE_EXHAUSTED dengan satu authoritative ErrorInfo, reason quota/capacity, positive RetryInfo dan locale tetap; tidak ada credential/account enumeration baru. |
| Security audit | Cleanup sebelum independent audit; original denial/error tetap walau recorder gagal/panic; Login/reuse tidak double audit; rate rejection hanya aggregate telemetry, bukan satu DB attempt per rejection. |
| Volume | Ukur path yang ditolak sebelum Rate, termasuk invalid session dari verified service. Timeout saja tidak membuktikan volume/concurrency terbatas. Tidak menambah unbounded queue/log/DB fan-out. |
| Mutation safety | Required success audit/outbox/result tetap atomik; no-op/replay semantics dan commit-on-denial refresh reuse tidak berubah oleh observation. |
| Redaksi | Sentinel password/hash/access-refresh-service token/DSN/private error tidak masuk snapshot, log, label, atau exported error. |

Service admission baru membatasi work menuju independent audit sebelum session
lookup: 600 request/menit burst 100 per verified service/process, shared cap
10.000/TTL 15 menit. Rejection menggunakan RATE_LIMITED/RESOURCE_EXHAUSTED dengan
RetryInfo tanpa DB audit. Admitted requests tetap menjalankan required audit;
tidak melakukan sampling/drop. Token service yang sudah dikonsumsi tidak
direfund pada downstream denial, dan delapan quota identity existing tetap.
Gate juga berlaku untuk Logout/revoke sesuai persetujuan user. Ini rate bound,
bukan jaminan batas total koneksi/goroutine atau distributed quota.

## 5. Urutan pekerjaan

Checklist berikut rencana implementasi, belum tracked task OpenSpec atau PASS.

### A. Baseline dan gap ledger

- [x] Baca AGENTS root/terdekat, capture source manifest aman, dirty-tree scope,
  pinned checksum, workspace mode, inventory 37 methods/sembilan services.
- [x] Petakan M-10/M-11 ke source, scenario existing, historical evidence dan
  assertion yang masih kurang; buat expected/actual ledger, bukan asumsi gap.
- [x] Inventarisasi seluruh callback audit, refresh admission, early denial,
  worker, logger sink dan pool ownership. Bekukan definisi counter agar tidak
  double count generic interceptor/feature/repository.
- [x] Finalisasi schema metrics, finite cardinality, ukuran output, cadence,
  timeout/sink behavior, dan verifikasi policy volume audit bagian 4.

Hasil: baseline, gap ledger, ownership map, decision register dan test matrix.

### B. Metrics core dan RPC observation

- [x] Buat satu store kecil beserta typed snapshot, histogram tetap, bounded
  method/group/reason mapping, dan dependency injection composition root.
- [x] Tambah outer observer tanpa mengubah urutan existing/error encoding;
  cover success, validation, service/session denial, allowed=false, dependency,
  rate rejection, cancellation/deadline dan panic behavior existing.
- [x] Integrasikan counts limiter dan audit failure existing; lengkapi hanya
  origin yang belum tercakup, dengan exact-once assertion per observation point.
- [x] Verifikasi arbitrary method/error/credential tidak menaikkan cardinality
  atau bocor ke snapshot; concurrent updates/snapshot bebas race.

### C. Runtime metrics dan lifecycle

- [x] Tambah aggregate reader outbox melalui ORM, mock/DryRun tests count/min,
  nil oldest/empty, published exclusion, future timestamp, failure dan context.
- [x] Reuse pool stats dan limiter counts; buat sampler satu siklus nonoverlap,
  freshness/error state dan bounded periodic summary melalui logger existing.
- [x] Wire worker dengan cancel/join di server dan cleanup failure paths;
  gunakan injected ticks/clock/sink untuk test tanpa real sleep atau service.
- [x] Buktikan collector/log failure tidak memblokir admission/limiter cleanup,
  tidak mengubah RPC outcome dan tidak meninggalkan goroutine/resource.

### D. Tutup gap hardening yang terbukti

- [x] Jalankan/extend existing ratelimit, interceptor dan registered suites
  untuk delapan group, numeric/refill/layered/cap/TTL/trust/RetryInfo assertions.
- [x] Uji attack-shaped input dengan banyak identitas/method/error arbitrer;
  assert batas memory/series/log volume, tidak hanya latency atau tanpa panic.
- [x] Buktikan independent audit cleanup/context/error/redaction dan exact
  Login/reuse/rate rejection counts. Pertahankan required audit saat overload;
  keputusan policy yang belum selesai menahan closure gate terkait.
- [x] Implementasikan minimum fix berdasarkan source/repro; tidak merapikan
  business code lain atau mengganti quota/framework/dependency.

### E. Final verification dan dokumentasi

- [x] Jalankan gate bagian 7 pada source deliverable yang sama, simpan receipts
  aman dan traceability requirement → scenario → result.
- [x] Dokumentasikan membaca snapshot/counter/reset/freshness dan expected
  pending backlog ketika relay belum aktif; hindari klaim delivery/p95 palsu.
- [x] Tutup seluruh gap mandatory dengan evidence; bedakan backend Phase 5
  completion dari actual consumer/deployment readiness Phase 6 DEP-01.

## 6. Acceptance ledger

Status awal seluruh gate adalah **NOT_STARTED**; hasil aktual dicatat pada
[acceptance ledger](../openspec/changes/archive/2026-10-20-auth-phase-5-hardening-metrics/evidence/progress.md).
Local ID tidak mengganti M/AC asal.

| ID | Kaitan | Syarat PASS |
| --- | --- | --- |
| P5-01 | M-10/M-11, spec bagian 8 | Baseline/method inventory, gap/decision ledger dan bounded schema metrics lengkap. |
| P5-02 | M-10 | Delapan group tetap sesuai spec; burst/refill/atomicity/cap/TTL/trust/RetryInfo dan concurrency assertions PASS. |
| P5-03 | Metrics minimum, M-01/02/05 regression | Exact RPC/auth/rate counts, encoded errors, allowed=false, histogram, unknown method, finite cardinality dan redaksi PASS. |
| P5-04 | M-11, spec volume | Audit persistence failure observable tanpa duplicate audit; independent cleanup/original error tetap; volume/resource bound dibuktikan dan tidak ada policy ambiguity/drop required audit. |
| P5-05 | AC-28 MVP subset, metrics minimum | Aggregate pending/oldest/pool metrics benar; stale/unavailable jelas; query/context/failure/empty dan dedicated PostgreSQL assertions PASS. Bukan broker proof. |
| P5-06 | M-07/10/11 regression | Worker/ticker/cancel/join/startup failure/log sink isolation, concurrent update/snapshot/cleanup dan credential redaction PASS. |
| P5-07 | EXT-01 regression | Final workspace/standalone gates, method/error compatibility, source digest, safe receipts dan docs PASS; DEP-01 tetap terpisah. |

## 7. Verification saat implementasi

Semua test di root `tests/`, runner tipis dan scenario `.go` mengikuti convention
existing. Reuse `tests/ratelimit`, `tests/interceptors`, `tests/configs`,
`tests/integration/auth_reads`, serta suites feature/repository yang terdampak;
buat suite `tests/telemetry` hanya ketika production consumer dibuat.

1. `gofmt` file Go yang berubah; targeted normal/race tests untuk tiap increment
   dan seluruh caller yang berubah. Clock/tick, observer, sink dan DB default
   dikontrol dengan fake/mock; assert value, context, call order/count, side effect.
2. Setelah targeted PASS: `go test ./...`, `go vet ./...`, `go test -race ./...`
   pada workspace dan ulangi dengan `GOWORK=off` pada published pins existing.
3. Dedicated opt-in PostgreSQL normal/race untuk aggregate outbox dan invariant
   audit/persistence yang berubah. Reuse `tests/integration/postgres` dengan
   tag `postgres`, guarded test DB/owned namespace dan existing config loader.
   Jalankan serial antar mode agar fixture tidak bertabrakan. Tambah interleaving
   dua physical connections/barrier bila perubahan menyentuh locking/mutation;
   mock aggregate saja tidak membuktikan database semantics.
4. Full opt-in PostgreSQL regression normal/race pada kedua mode sebelum final
   backend closure. Unavailable/skipped berarti BLOCKED pada gate DB. Tidak
   memakai DB aplikasi, Docker, migration baseline atau startup server asli.
5. `make build BIN_DIR=<temporary-directory>` di kedua mode, inventory 37
   methods/sembilan services, error/RetryInfo/locale compatibility, link/anchor
   check, `git diff --check`, diff/status/source manifest review.

Catat exact command, source digest, mode, module checksum, waktu UTC,
expected/actual, exit/pass/fail/skip dan safe environment identifier. Jangan
menyalin DSN/secret/runtime config. Jika source berubah sesudah gate, ulangi
gate terkait. Angka pass dari arsip tidak menggantikan verifikasi deliverable.

## 8. Risiko, keputusan, dan syarat selesai

| Risiko/keputusan | Penanganan |
| --- | --- |
| Double counting limiter/audit | Tentukan satu owner per occurrence; counter completion berbeda dari counter persistence failure. Uji exact counts. |
| Collector menambah beban pool/outbox | Aggregate sekali per menit dengan timeout, tidak per request; ukur query pada test DB. Index/migration baru hanya bila gap terbukti dan scope disetujui. |
| Sink lokal memblokir | Gunakan bounded export dengan synchronous sink existing dan accepted shutdown wait; jangan mengandalkan goroutine timeout yang bocor. |
| Durable failed-attempt volume | Selesaikan decision register bagian 4 tanpa silently drop audit atau mengubah error/limiter policy. |
| Metrics salah dianggap authoritative | Nilai lokal reset saat restart; freshness eksplisit, tidak digunakan untuk authorization atau keputusan replay. |
| Source/docs historis berbeda | Source current, archived receipts dan spec diperiksa bersama; baseline HS256/Register aktif di dokumen lama tidak mengembalikan behavior legacy. |

Dokumen plan selesai setelah scope/source/link konsisten. Implementasi Phase 5
baru selesai ketika P5-01..07 PASS dengan evidence deliverable, semua mandatory
gap/decision terselesaikan, serta compatibility/regression terbukti. Plan sendiri tidak memberi status PASS implementasi; hasil command dan test
aktual pada evidence menjadi dasar closure.

Implementasi scoped change `auth-phase-5-hardening-metrics` mengikuti urutan
metrics core → runtime sampler → hardening gap closure → final gates.
Schema dan batas runtime dijelaskan di [runtime metrics](phase-5-runtime-metrics.md).
Phase 6 DEP-01 mengikuti [readiness checklist](foundation-activation-readiness.md#phase-6-deployment-checklist-and-rollback)
dan tetap belum diluluskan oleh pekerjaan Phase 5.

## 9. Hasil implementasi20 Oktober2026

P5-01..07 PASS pada source deliverable yang sama. Full default3579 dan
PostgreSQL589 test/subtest pass events per run, normal/race di workspace dan
GOWORK=off, tanpa failure/skip; vet/build kedua mode PASS. Seluruh20 descriptor
tetap identik,37 RPC/sembilan service. Lihat [acceptance dan receipts](../openspec/changes/archive/2026-10-20-auth-phase-5-hardening-metrics/evidence/progress.md).
Phase6 DEP-01/consumer/deployment/broker tetap terpisah.
