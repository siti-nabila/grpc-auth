# Design

## Context

Lihat [proposal](proposal.md#why) untuk masalah dan scope. Rujukan awal adalah
[plan Phase 5](../../../../docs/phase-5-hardening-metrics-plan.md), dengan dua
keputusan user 20 Oktober 2026: quota service 600/menit burst 100 untuk seluruh
37 RPC, serta log periodik existing yang boleh menahan shutdown selama write.
Keputusan ini menggantikan bagian plan awal yang melarang quota baru dan
mewajibkan sink cancellable; bukan open question implementasi.

Source sudah dibaca: `internal/ratelimit/store.go`, interceptor
rate/refresh/service/session/security-attempt/error, composition
`configs/{foundation,server,login_producer,lifecycle,config}.go`, audit bind/create
pada features, domain/writer outbox, reader aggregate existing, transaction
provider/owner, pool/logger dan corresponding root tests. Constructor standalone
Login dan foundation berbagi limiter; wiring baru harus memakai satu instance.

Baseline HEAD dan graph `2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec`; working
tree lebih baru. Graph menunjukkan dependency configs → interceptor →
transaction infrastructure, lalu diverifikasi pada source. Hub domain
auth/outbox/user-role/snapshot/transaction dipertahankan. Historical config
OpenSpec masih menyebut enam RPC/ORM lama; source/go.mod dan evidence terbaru
menunjukkan 37 methods/sembilan services, contract
`v0.0.0-20261018073250-b4ad807ad5fa` dan ORM `v1.8.0-support-subquery`.

Temuan konkret: `Store.Counts()` sudah bounded, `AuditFailures()` hanya dua
attempt paths; generic attempt mempunyai timeout 2 detik sedangkan Login
recorder memakai request context. Session interceptor berada sebelum Rate,
sehingga valid service dengan invalid user session dapat mencapai independent
audit tanpa identity quota. Logger existing menggunakan synchronous logrus
hooks ke stdout dan file `io.MultiWriter`; tidak menyediakan cancellation.
Belum ada aggregate outbox reader atau dedicated metrics package.

## Goals / Non-Goals

**Goals:** satu owner metrics di composition root, fixed-size snapshot,
independent cleanup/sampling workers, bounded admission sebelum DB audit,
pengukuran exact-once serta current-authority/persistence regression yang jelas.

**Non-Goals:** metrics tidak menjadi authorization cache, event relay, mutable
global registry, generic command framework, atau alasan mengganti transaction
ownership/config loader. Tidak ada RPC, schema, dependency, migration, extra
listener atau feature scaffold baru. Tidak menjanjikan cancellation sink/file
write atau total in-flight bound dari token bucket.

## Decisions

### 1. Service gate terpisah sebelum audit dan session

Pipeline final:

```text
Metrics → Language → Error → Service → ServiceAdmission → Attempt → Session
        → existing identity Rate/RefreshAdmission → handler/feature
```

`ServiceAdmission` membaca verified `serviceauth.Caller` sesudah TLS/token/
method allowlist, memakai group baru `service-ingress` dan key service ID dalam
store existing. Numeric policy: refill 600/menit, burst 100; satu token per
100 ms. Reuse atomic integer-credit arithmetic, cap 10.000, TTL 15 menit,
cleanup 1 menit. Current/previous service credentials berbagi key yang sama.
Unverified service tidak allocate state; service ID tetap tidak diekspor.

Stage ini berdiri sendiri. Token service yang sudah dikonsumsi tidak di-refund
saat downstream menolak/cancel. Existing Login IP+account dan refresh
IP+proven-session tetap atomic dalam stage masing-masing. Tidak memakai
reserve/cancel, mencoba dua admission service atau membangun store kedua.
Kebijakan ini membatasi work yang masuk menjadi burst 100 plus refill 10/detik
per configured caller/process, termasuk failed-attempt transactions; tidak
menjamin batas koneksi/goroutine network atau quota global multi-replica.

Early rejection reuse `rateError`/catalog RATE_LIMITED dengan
RESOURCE_EXHAUSTED, satu ErrorInfo, `rate_limit_reason=quota|capacity` dan
positive RetryInfo. Refill wait dibulatkan ke detik, sehingga next token 100 ms
dapat menghasilkan guidance 1 detik; cap memakai 1 menit. Tidak menambah
metadata contract baru. Letak sebelum Attempt mencegah DB audit per rejection.
Admitted denials tetap mengikuti required audit existing, tanpa sampling/drop.

Alternatif: sampling audit setelah session denial dapat menghilangkan required
attempt; global request queue menambah lifecycle/overload semantics. Keduanya
tidak dipilih. Quota baru adalah keputusan user eksplisit, bukan inferensi dari
angka quota per-user existing.

### 2. Inventory dan authority tetap untuk semua full methods

Setiap prefix pada tabel diikuti `/` dan satu nama method menghasilkan exact
full method. Seluruh 37 berpartisipasi pada service gate, termasuk revoke/
Logout/TesRPC; generated descriptor existing menjadi inventory regression.

| Full service prefix | Methods | Authority sesudah admission |
| --- | --- | --- |
| `/user.v1.UserService` | Register, Login, TesRPC, Me, ListUsers, GetUser, CreateUser, UpdateUser, ChangeUserStatus, ListUserRoles, GrantUserRoles, RevokeUserRoles, ListUserApplicationAccess, GetMyEffectivePermissions, GetUserEffectivePermissions | Tiga pertama exempt user JWT; Me self/restricted minimum; user/catalog management exact current permission existing; effective permission self/admin tetap. |
| `/profile.v1.ProfileService` | UpdateProfile | Active unrestricted self, presence/mask legacy dan locked recheck. |
| `/auth.v1.AuthService` | RefreshSession, Logout, ChangePassword | Refresh credential proof tanpa access JWT; Logout satu access/refresh mode; ChangePassword verified self dan restricted allowed. |
| `/session.v1.SessionService` | ListMySessions, RevokeMySession, RevokeAllMySessions, ListUserSessions, RevokeUserSession, RevokeAllUserSessions | Self ownership atau exact admin session permission existing; restricted di luar allowlist tetap ditolak. |
| `/role.v1.RoleService` | ListRoles, GetRole, CreateRole, UpdateRole, ChangeRoleStatus, ReplaceRolePermissions | Exact role read/create/update/permission.update existing, system/last-admin guards tetap. |
| `/permission.v1.PermissionService` | ListPermissions | iam.permission.read. |
| `/application.v1.ApplicationService` | ListApplications, GrantApplicationAccess, RevokeApplicationAccess | Catalog permission.read; grant/revoke memerlukan AND user.application dan user.role permissions existing. |
| `/authorization.v1.AuthorizationService` | EvaluateAccess | Verified unrestricted self dan namespace allowlist, satu authoritative primary snapshot. |
| `/audit.v1.AuditService` | ListAuditLogs | iam.audit.read terkini dan safe primary snapshot. |

TLS/service/full-method/namespace dan user permission tidak diganti oleh
admission. Exact permission/ownership matrix mengikuti
[spec MVP](../../../../docs/grpc-auth-mvp-spec.md#42-matriks-full-rpc) dan source
existing; descriptor/permission inventory dibandingkan saat verification.
Unknown route tetap UNIMPLEMENTED di gRPC; tidak memasang unknown handler demi
mendapat metric. TLS handshake error tidak melewati unary observer.

### 3. Metrics store kecil dengan cardinality finite

`internal/telemetry` memiliki consumer nyata dan satu instance di foundation.
Store memakai enum/index finite atau fixed arrays dengan mutex/atomics; method
index 37 + unknown, gRPC codes 17, auth category none/failure/deny/dependency dan
fixed audit/collection origins. Tidak membuat map berkey input arbitrer.
Copy snapshot di bawah lock singkat, formatting/logging di luar lock. Counter
overflow ditangani saturating, bukan wrap menjadi kecil/negatif.

RPC observer paling luar mencatat final encoded status dan elapsed time.
Histogram cumulative upper bounds 1/5/10/50/100/500/1000/5000 ms + overflow;
count/sum memakai numeric bounded representation. Inject clock untuk test;
negative elapsed akibat fake/clock skew dijadikan nol. Jangan mengklaim p95
dari sum/count. Per completion satu auth category sesuai catalog allowlist;
valid `EvaluateAccess allowed=false` masuk deny walau code OK. Tidak mengubah
permission denial/dependency error atau menganggap semua business conflict
sebagai auth failure.

Snapshot memuat fixed numeric fields, bukan raw error/request/credential,
correlation atau arbitrary caller/user labels. Rate counter dibedakan stage
service/identity dan reason quota/capacity; group detail tambahan tidak wajib.
Limiter entries dibaca dari store, bukan gauge kedua yang dapat drift.

Alternatif global exporter/SDK monitoring menambah dependency/infrastructure;
per-request log menambah I/O dan volume. Typed local snapshot + periodic existing
logger memenuhi kebutuhan lokal tanpa keduanya.

### 4. Ownership audit failure dan finalization uncertainty

Reuse callback Login dan generic attempt dengan origin berbeda. Preserve
`AuditFailures()` compatibility jika masih digunakan. Satu failed independent
attempt dihitung pada outer owning callback, tanpa decorator kedua untuk
Create yang sama. Begin/Create/commit failure/panic pada attempt mempertahankan
safe original outcome dan dihitung sekali sebagai failed attempt persistence;
metric tersebut tidak menjanjikan row pasti hilang pada commit uncertainty.

Required business audit diobservasi pada `domain.AuditWriter.Create` boundary
melalui decorator ringan di `features/common`, diinjeksi lewat Bind closure
composition root. Reuse domain contract; jangan membaca payload untuk label.
Origin `required-write` berarti operasi Create mengembalikan error, termasuk
safe record validation failure; bukan bukti query pasti dieksekusi. Observer
hanya menambah counter dan mengembalikan original error/cause.

Transaction finalization dipisahkan sebagai `finalization-unknown` pada
feature/common owner yang mengetahui commit gagal, bukan inferensi bahwa audit
hilang dari RPC UNAVAILABLE. Tidak melakukan retry/menganggap rollback pasti
pada lost acknowledgement. Finalization operation counter dan Create counter
punya denominator berbeda; satu failure yang sama tidak dicatat dua kali di
origin yang sama. Pertahankan fungsi owner sederhana dan callback optional
internal tanpa boolean mode/generic framework; provider/handle tidak diganti.

Bind consumers yang sudah dipetakan: pending Register, Login success, refresh
normal/reuse, Logout, ChangePassword, profile, user management, assignments,
role management, application access, self/global/admin session. Independent
attempt helper tetap owner transaksi terpisah, tidak dibungkus sebagai required
business write. Tambah coverage semua consumer yang wiring-nya berubah.

Alternatif menghitung semua UNAVAILABLE sebagai audit failure memberi false
positives; repository-global callback mengikat request ke mutable global.
Observasi di boundary owner mempertahankan identity dan ownership.

### 5. Aggregate reader dan runtime samples

Tambahkan reader ORM untuk aggregate count dan minimum created_at pending
`outbox_events`, memakai satu statement `published_at IS NULL`. Reuse contoh
aggregate ORM di reader audit/catalog; hanya projection fixed literal,
tanpa raw SQL execution, payload read, limit yang membuat count parsial atau
filter `next_attempt_at`. Read-only sampler tidak membuka transaction mutation,
lock atau pool baru. Jika API ORM terbukti tidak mendukung aggregate ini,
hentikan dependent work sebagai EXT-03; jangan bypass ORM/update sibling.

Contract consumer kecil untuk pending count/oldest; concrete reader di
`internal/repositories/reader`, model/projection aman di domain. Binding
composition menggunakan primary `*sql.DB` dan ORM config debug=false.
Setiap read memakai worker context timeout 2 detik. Satu aggregate statement
memberi consistent count/min tanpa memindahkan BeginSnapshot ke repository.
Pool stats berasal dari `DB.Stats()`, menyimpan open/in-use/idle/max dan
cumulative WaitCount/WaitDuration tanpa DSN/pool-name label.

Sampler satu menit, single worker nonoverlap, tanpa catch-up queue; clock/tick
injected. Awal unavailable; success menyimpan timestamp/count/oldest, empty
count/age=0, future oldest age=0; failure menyimpan last good sample dengan
stale status. Snapshot menghitung age/freshness dari controlled now, bukan
mengubah gauge gagal menjadi healthy zero. Local status tidak mengubah RPC
dependency fail-closed atau authorization snapshot.

### 6. Existing logger sink dan dua worker yang dimiliki server

Pisahkan telemetry sampler/logger worker dari existing limiter cleanup worker.
Keduanya dimulai hanya sesudah construction berhasil; keduanya dimiliki
`GRPCServerClient` dan di-cancel/join pada Stop/GracefulStop/listener failure.
Runtime `configs.run` menutup pool sesudah server workers selesai. Constructor
failure tidak memulai background worker; tidak ada worker per feature/request.

Setiap telemetry tick mengekspor satu event log berisi snapshot safe dengan
urutan stabil. Payload telemetry JSON mempunyai hard maximum 64 KiB;
worst-case fixed schema/max numeric value diuji. Logger hooks boleh merender
event yang sama ke fixed destinations existing; tidak menambah sink. Jika
serialization gagal/ukuran tidak memenuhi bound, increment bounded export
failure, jangan log payload parsial/error rekursif. Semua required metrics
tetap tersedia pada typed snapshot. Adapter telemetry memakai formatter dan
sink/hook existing tetapi mengembalikan error hook/write ke sampler; jangan
mengandalkan `.Info()` yang tidak mengembalikan hasil persistence ke caller.
Dispatch telemetry diserialisasi, tidak memutasi global hooks atau mencetak
raw hook error ke stderr. Hook/write failure menambah export failure sekali,
tanpa retry/log rekursif atau klaim event sudah tersimpan di semua destination.

Synchronous write tidak bisa dicancel: keputusan user membolehkan worker dan
shutdown menunggu write selesai. Saat cancel, tidak memulai sample/write baru,
query aktif dibatalkan, write yang sudah dimulai diselesaikan lalu worker join.
Tidak membuat detached goroutine sebagai timeout palsu. Slow logger tidak
mengunci metrics store, menahan limiter cleanup atau memblokir request path.
Test memakai sink yang diblokir/release oleh channel untuk membuktikan urutan,
bukan real file delay atau janji shutdown dengan timeout mutlak.

### 7. Persistence, token dan mutation contracts tetap

Seluruh call site Begin/Commit/Rollback/TryLock tetap di features/common atau
feature, menggunakan request context dan injected provider. ORM PostgreSQL
TryLock existing retained sampai finalization; protocol idem → last-admin/
policy → account/role-code → user → role → application → session canonical
tetap. Setelah locks, seluruh read/write memakai handle yang sama; jangan
menambah repository lifecycle atau fallback pool dalam command.

[Matrix 21 mutation methods](../2026-10-18-auth-mutation-audit-reconciliation/design.md#mutation-outcome-matrix)
dan event envelope mapping dipertahankan. Untuk request admitted:

| Outcome | Efek yang tetap | Observasi baru |
| --- | --- | --- |
| Effective command | Mutation + required audit/outbox + applicable safe result atomik, entity version naik sekali | Satu RPC completion, audit error hanya pada actual failure boundary. |
| New keyed no-op / replay | No-op satu NO_OP audit tanpa event/version; replay tidak menambah audit/event/version | Tiap invocation mempunyai completion sendiri, tanpa audit baru demi metrics. |
| Failure/panic/cancel precommit | Rollback/cleanup dan current authority/expected-version/error existing | Bounded safe failure category, tanpa retry command. |
| Commit acknowledgement uncertain | Tidak mengklaim pasti rollback/commit atau mengembalikan credential | finalization-unknown; management retry tetap scoped safe replay 24 jam, refresh response-loss tetap login ulang. |
| Refresh consumed reuse | Required revoke/audit/event committed dahulu, lalu UNAUTHENTICATED | Denial completion tanpa rollback security state/duplicate attempt. |
| Service admission rejects | Tidak masuk Session/Attempt/feature, tanpa DB audit/mutation | Satu service rejection dan satu RESOURCE_EXHAUSTED completion. |

Key actor/method/canonical target/key dan keyed credential fingerprint tetap;
current permission sebelum replay, replay sebelum target stale-version check,
same-key/different-payload conflict dan different-target scope tidak berubah.
Role/permission/app commands tidak menaikkan auth_version/revoke sessions;
password/global revoke/suspend/deactivate mempertahankan invalidation existing.
EdDSA-only issuer ApplicationName, audience ignored, access 15 menit, absolute
session 7 hari/skew 30 detik dan restricted Me/ChangePassword/Logout tetap.
Register tetap PENDING/token-empty, credentials hanya committed Login/Refresh.

## Acceptance Ledger

Status awal implementasi **NOT_STARTED**; hasil aktual tersedia di
[evidence/progress.md](evidence/progress.md). Owner seluruh P5: grpc-auth;
prerequisite bersama: secure pipeline dan pinned published dependencies.
Status PASS memerlukan source identity dan actual results, bukan dokumen ini.

| ID | Scope dan kaitan asal | Test/evidence mandatory |
| --- | --- | --- |
| P5-01 | MVP_REQUIRED; M-10/11 dan metrics minimum | Baseline/source digest, 37/nine inventory, bounded schema/decision/gap ledger dan P5 requirement-to-test matrix. |
| P5-02 | MVP_REQUIRED; M-10, new approved service policy | Numeric/concurrent service 600/100; all37 coverage, rotation/method identity, cap/refill, no rejected DB work; delapan identity quota unchanged, atomic layered tests. |
| P5-03 | MVP_REQUIRED; metrics minimum, M-01/02/05 regression | Completion/auth classification/histogram/unknown routing/redaction/cardinality/max size and race snapshots via unit + registered tests. |
| P5-04 | MVP_REQUIRED; M-11, AC-24/28 regression | Early admission bounds attempted DB work, no sampled admitted audit, exact independent/write/finalization failure, no-op/replay, refresh exceptions, safe original error and dedicated PostgreSQL assertions. |
| P5-05 | MVP_REQUIRED; AC-28 pending subset | ORM aggregate single snapshot, future/empty/published/retry semantics, stats/freshness/cancel/error, dedicated PostgreSQL normal/race. Tidak membuktikan broker delivery. |
| P5-06 | MVP_REQUIRED; M-07/10/11 | Startup/listener/shutdown/slow-sink/channel release, worker join before pool close, cleanup progress, no overlap/queue/request I/O, registered/PG regression. |
| P5-07 | MVP_REQUIRED; EXT-01 regression | Full normal/vet/race/build + PG normal/race, workspace dan GOWORK=off, descriptor/errors/version receipts dan docs/diff/scope validation. |

## Risks / Trade-offs

- [Service-wide quota dapat menunda Logout/revoke saat caller flood] → perilaku
  ini disetujui user; dokumentasikan retry, tanpa bypass tersembunyi atau
  mengaku request revoked sebelum feature berjalan.
- [Downstream rejection tetap memakai service token] → stage boundary explicit
  dan diuji; refund tidak dipakai untuk membuka jalur spam audit.
- [State cap bersama dapat capacity-reject service baru] → reuse bounded store,
  guidance existing; configured service IDs saja dapat allocate group ini.
- [Sink file/terminal blocking menahan shutdown] → accepted limitation;
  sampler terpisah, tidak ada goroutine yang diabaikan atau janji cancellation.
- [Audit instrumentation menambah banyak bind consumers] → decorator kecil,
  callback typed dan exact failure/order tests; tidak membuat middleware DB
  global atau framework transaksi baru.
- [Backlog aggregate dapat mahal dan pool sample bersifat lokal] → read satu
  menit timeout 2 detik, ukur pada test DB, stale eksplisit; index/migration
  hanya lewat scope terpisah bila gap nyata.
- [No absolute bound on network concurrency] → service token bucket membatasi
  admission work, bukan memory/concurrency global; jangan mengklaim DDoS atau
  multi-replica protection dari hasil lokal.

## Migration Plan

Tidak ada migration database/protobuf atau dependency publication. Implementasi
bertahap: store/observer → service admission → audit metrics → aggregate/sampler
→ lifecycle/logging → full verification. Update plan/operational docs agar
600/100, nonrefund dan accepted sink wait menggantikan assumptions awal.

Default tests memakai fake/mock/in-memory registered paths. Dedicated PG
normal/race memakai existing opt-in guarded test DB/owned namespace untuk
aggregate dan exact admitted/rejected audit counts; saat locking berubah,
gunakan dua physical connections/barriers. Unknown commit outcome, scoped
replay, last-admin/current authority tetap regression. Full default/vet/race/
build dan full PG normal/race di workspace dan GOWORK=off memakai pin yang sama;
source berubah setelah receipt mengulang affected gate. Unavailable/skipped
PG berarti BLOCKED, tanpa Docker/DB aplikasi/server startup.

Actual deployment/cutover/consumer readiness tetap Phase 6 DEP-01, tidak
dijalankan oleh change ini. Rollback plan harus mempertahankan TLS/service,
EdDSA/session/permission dan persistence protections; tidak mengembalikan
revoked sessions, HS256, sid-less credentials atau auth_version lama.
Penghapusan service gate dari versi rollback merupakan policy change dan
memerlukan keputusan tersendiri, bukan fallback otomatis saat metrics gagal.
