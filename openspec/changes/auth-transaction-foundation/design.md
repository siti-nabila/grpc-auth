# Design

## Context

Motivasi dan batas increment ada pada [proposal](proposal.md). Ini baseline source inspection 9 Oktober 2026, bukan hasil test atau inspeksi database live. Graph AST dan HEAD grpc-auth sama-sama `46dc4debb87281d8357a1c671365407ac8c374ca`; graph menunjukkan transaction helper dipakai Register dan UpdateProfile, lalu source dibaca untuk memvalidasi path tersebut.

| Source | Fakta relevan |
| --- | --- |
| [auth.feat.go](../../../internal/features/auth_feature/auth.feat.go) | Register cleanup memanggil finalizer sebelum recover; profile/user-role write memakai errgroup dan shared err. Duplicate/default-role reads memakai reader pool |
| [profile.feat.go](../../../internal/features/profile/profile.feat.go) | Profil dibaca sebelum Begin; return writer.Update langsung tidak mengisi err yang dibaca defer; unnamed return tidak menerima commit error |
| [common/impl.go](../../../internal/features/common/impl.go) | Helper commit saat err nil, rollback error dibuang; RPC error saja tidak cukup untuk memilih outcome security-revocation |
| [auth writer](../../../internal/repositories/writer/auth.writer.go), [profile writer](../../../internal/repositories/writer/profile.writer.go), [user-role writer](../../../internal/repositories/writer/user_role.writer.go) | Begin memakai Db.Begin tanpa request context; adapter disimpan pada instance, UseTransaction membagi handle write |
| [auth reader](../../../internal/repositories/reader/auth.reader.go), [profile reader](../../../internal/repositories/reader/profile.reader.go), [role reader](../../../internal/repositories/reader/role.reader.go) | Query adapter dibangun dari pool; belum ada bound transaction reader |
| [profile handler](../../../internal/handler/profile.handler.go) | Request domain dikirim by value; response memakai request.Id yang tidak berubah dan UserId=0 |
| ORM module v1.7.0 | SqlTransactionAdapter.UseModel/Create/Update memakai sql.Tx yang sama; TryLock memakai QueryRowContext; PostgreSQL pg_try_advisory_xact_lock; ReleaseLockQuery needed=false |
| [schema artifact](../../../docs/sql/user-management/01_schema_backfill.sql) | Audit/outbox/idempotency tables, auth.version/auth_version; sessions tanpa version. User melaporkan migration/seed, belum diverifikasi live |
| api-contracts sibling | Busy/version/idempotency catalog belum ada; common ServiceUnavailable dipetakan UNIMPLEMENTED. Workspace checkout berbeda dari published go.mod version |
| [testutils](../../../tests/shared/testutils/scenario.go) | Scenario{Name,Run} dan Run(t,scenarios); root tests saat ini punya handler/interceptor/reader runners, belum punya transaction foundation suite |

## Goals / Non-Goals

**Goals:** perbaiki transaksi existing yang benar-benar dipakai, implementasikan repository persistence fungsional dengan contract/error paths yang diuji, dan buktikan lifecycle/atomicity lintas koneksi PostgreSQL. Tidak membuat abstraction kosong hanya untuk menyiapkan tree.

**Non-Goals:** tidak membuka RPC atau security flow baru; tidak mengganti stack atau public protobuf. Session/auth enforcement dan production command wiring belum berada dalam increment ini. Referensi target di bawah membatasi desain fondasi agar kompatibel dengan change berikutnya; tidak berarti target itu diimplementasikan sekarang.

## Decisions

### 1. Feature memiliki transaction outcome secara eksplisit

Pemanggilan `Begin`, `Commit`, `Rollback`, dan `TryLock` berada di `internal/features/`, termasuk helper `internal/features/common` bila diperlukan. Feature menentukan boundary, canonical keys/order, dan outcome. Feature memanggil `Begin(requestCtx)` melalui transaction provider/factory kecil yang diinjeksi; implementasi teknis provider di infrastructure menggunakan `Db.BeginTx(requestCtx)` dan adapter ORM existing. Bukan writer repository yang menjadi entry point Begin; baseline writer Begin akan diganti pada path mutasi terkait. Tidak menambah framework transaksi, global mutable provider, atau SQL locking baru.

Feature membagikan handle yang sama ke dependency terkait, memanggil TryLock, menjalankan state checks/write melalui repository, lalu memanggil Commit secara eksplisit sebelum response. Repository hanya read/write dengan handle yang diberikan; tidak memanggil Begin/Commit/Rollback/TryLock secara mandiri. Implementasi metode pada adapter tetap di ORM; aturan lokasi berlaku pada caller/orchestration aplikasi, bukan internal driver atau harness pembuktian ORM. Deferred cleanup hanya menangani transaksi yang belum selesai. Recover menetapkan failure dan rollback sebelum error generic dikembalikan; tidak ada commit sebelum recover. Success result dibuat hanya setelah commit sukses.

Operation error dipertahankan bila rollback juga gagal; cleanup error dicatat terpisah dan redacted. Commit error tidak dianggap pasti rollback bila outcome unknown; jangan mengembalikan credential/success atau retry mutation tanpa mekanisme yang disepakati. Handle transaksi didetach setelah terminal outcome, sehingga dependency tidak mengembalikan closed transaction pada request berikutnya.

Tidak memakai boolean flag generic commit-on-error. Future refresh reuse owner dapat commit security writes secara eksplisit dan kemudian mengembalikan denial; deferred cleanup melihat transaksi sudah selesai. Test consumer fixture membuktikan lifecycle ini, bukan real RefreshSession.

Alternatif: mempertahankan finalizer yang memilih commit dari err pointer saja. Ditolak karena unnamed return, panic ordering, dan security-denial outcome tidak dapat dijamin dengan pola itu.

### 2. Bound readers dan writers memakai adapter yang sama

Reader/writer transaction path memakai adapter SqlTransactionAdapter yang sama; UseModel pada adapter menyediakan query bound ke sql.Tx. Reader pool tetap untuk read-only flow existing. Tambahkan contract/injection hanya pada auth/profile/default-role reads yang perlu check di bawah lock dan pada repository persistence baru; tidak refactor seluruh readers atau domain sekaligus.

Constructor feature/repository memiliki seam injeksi kecil untuk dependency yang digunakan, pool, clock/ID source dan token generation bila relevan. Runtime constructor existing tetap merangkai production dependencies; unit tests tidak memanggil startup configs.InitAllConfigs atau global DB fallback. Manual fake/mock driver/ports cukup; dependency test baru hanya jika pola existing tidak memadai dan scope-nya disetujui.

Alternatif: membiarkan reader memakai pool sesudah writer memperoleh lock. Ditolak karena check dapat membaca snapshot/koneksi yang tidak sesuai dengan write dan mutation tidak benar-benar satu transaction boundary.

### 3. Register writes berurutan dan canonical account check di dalam transaksi

Normalisasi trim + lowercase dilakukan sebelum validation untuk Register/Login, sesuai keputusan bisnis existing MVP. Batas email/password 6–50 tetap. Preflight memastikan seluruh email canonical, unique, dan tidak collision; bila data tidak canonical, rollout berhenti tanpa auto-cleanup. File 00_preflight hanya memeriksa duplicate literal; tambahkan pemeriksaan count canonical collision pada checklist/preflight read-only yang relevan, tanpa menampilkan email/password.

Hash Argon2id disiapkan sebelum lock/tx agar critical section pendek; parameter/hash format unchanged dan encoded hash harus muat VARCHAR(150). Sesudah Begin dan lock, duplicate dan default member dibaca ulang melalui adapter. Tulis auth → profile → user_role berurutan; jangan menjalankan errgroup pada satu mutable tx adapter. Generate token existing sebelum commit agar kegagalannya tetap rollback; token hanya keluar setelah commit.

Default member tetap non-admin dan tidak ada user pertama auto-admin. Register status/credential cutover belum berubah; presence tabel PENDING tidak membuktikan login/status enforcement. Jangan menyebut increment ini secure Register MVP.

Alternatif: mempertahankan parallel writes untuk mengurangi latency. Ditolak karena tidak ada kebutuhan throughput terukur yang membenarkan shared err dan adapter concurrent access pada tiga write ini.

### 4. Profile owner dan response berasal dari hasil transaksi

Ambil principal dari request context, Begin, peroleh user lock, lalu baca profil owner melalui bound reader. Tiga scalar fields tetap full replacement; tidak menambah arbitrary map, email/password/status field atau field_mask. Feature mengembalikan domain profile hasil committed agar handler memakai ID/user_id aktual, bukan salinan request yang tidak berubah. Nilai baru dibaca/ditentukan di dalam tx; response tidak memakai pool read sesudah commit yang dapat melihat mutation request lain.

Exact methods dan boundary:

| Full RPC | Authentication sekarang | Guard fondasi | Behavior/error berubah |
| --- | --- | --- | --- |
| /user.v1.UserService/Register | Public user-JWT exemption existing | policy/default-member → canonical account | Sequential atomic writes, duplicate after lock, busy ABORTED; existing token response sebelum cutover |
| /user.v1.UserService/Login | Email/password existing | Tidak menambah write lock | Canonical email lookup saja; JWT/error password existing tetap |
| /profile.v1.ProfileService/UpdateProfile | JWT principal existing | user/{principal ID} | Bound owner read, rollback/error/commit benar, response owner/ID benar; busy ABORTED |

Target service-token opaque per caller melalui TLS, restricted allowlist Me/ChangePassword/Logout, persistent session checks serta permission snapshot/recheck tetap [MVP section 3–4](../../../docs/grpc-auth-mvp-spec.md#3-identitas-caller-jwt-dan-authorization). Tidak mempercayai forwarded caller ID atau user_id browser dalam fondasi. Tidak menambah service authentication parsial yang terlihat lengkap.

Alternatif: handler tetap merakit response dari input by value. Ditolak karena owner dan profile ID akan tetap nol walau transaction benar.

### 5. TryLock reuse dan satu urutan key

Pertahankan ORM v1.7.0. PostgreSQL TryLock memakai pg_try_advisory_xact_lock; Commit/Rollback memanggil releaseLock, tetapi PostgreSQL needed=false sehingga tidak ada manual unlock sebelum outcome transaction. Hash/normalisasi key mengikuti ORM; buat key canonical sebelum menyerahkannya, tidak menambah SQL advisory-lock implementation di service.

Feature (atau helper bersama di features) membentuk canonical keys dan memanggil TryLock ORM pada handle request; repository tidak memilih scope atau memperoleh lock. Urutan bersama: `auth:idem:{digest(scope,key)}` → `auth:guard:last-admin:global` bila relevan → `auth:guard:policy:global` → account/role-code digest → user ID ascending → role ID ascending → application ID ascending → session UUID canonical ascending. Duplikasi key dihilangkan. Register hanya policy/default-member dan account; profile hanya user; fixture management memakai subset relevan. Last-admin/session/policy business enforcement tetap change consumer berikutnya.

TryLock acquired=false ditutup rollback dan dipetakan RESOURCE_BUSY/ABORTED; tidak menunggu/mengulang write otomatis. DB error dipertahankan sesuai cause. State discovery dibaca ulang sesudah lock; bila diperlukan predecessor lock baru, rollback/restart terbatas dengan request context yang sama. Bound reads memakai isolation READ COMMITTED untuk recheck sesudah acquisition, tidak membuat stale pre-lock snapshot baru sebagai dasar mutation. Multi-query authorization read snapshot milik future authz change, tidak disamakan dengan mutable tx check ini.

Advisory lock bersifat cooperative; unique/FK/constraints tetap wajib. Bootstrap/migration yang kelak mengubah invariant harus ikut protocol atau write quiescence eksplisit. Tidak ada external network call saat lock aktif.

Alternatif: row locks/advisory SQL baru di feature. Ditolak karena ORM existing sudah menyediakan transaction-scoped capability dan user memilih TryLock; conditional ORM repair hanya jika suite membuktikan gap.

### 6. Persistence repository mempunyai satu transaction contract

Tambahkan domain models/contracts dan reader/writer untuk audit, outbox dan idempotency hanya dengan operasi yang dipakai suite/consumer berikutnya. Reader/writer menerima transaction adapter dari feature dan hanya menjalankan query/read/write; tidak memulai transaksi, memanggil Commit/Rollback, atau mengambil TryLock sendiri. Idempotency lookup sesudah lock menggunakan adapter sama; repository yang wajib transaction tidak boleh fallback pool jika handle tidak tersedia.

| Data | Mapping / batas |
| --- | --- |
| Audit | actor_user_id terverifikasi atau actor_service terverifikasi; action/target; safe_before/after JSONB typed allowlist; reason/request/trace bounded; occurred_at UTC. NO_OP/failure lewat action/safe snapshot, tanpa outcome column baru |
| Outbox | event_id UUID, envelope catalog sebagai payload JSONB, aggregate_version dari consumer; published_at NULL, attempts awal 0, delivery metadata sesuai schema. Tidak memanggil RabbitMQ |
| Idempotency | PK(actor_id,method,route_scope,key), request_hash BYTEA, safe_response JSONB, created/expires TIMESTAMPTZ; scope canonical user/role/app/session target, key max 200, window 24 jam |

Payload/result memakai allowlist domain DTO, bukan raw request/response generic. Audit/result yang membawa credential ditolak; sanitization bukan alasan untuk menerima secret lalu menyimpannya. Caller user/service harus sudah diverifikasi consumer; repository tidak melakukan permission lookup atau percaya metadata input sendiri. Default clock/UUID source production aman, tests memakai injected fixed clock/deterministic IDs.

Event catalog/envelope dan R/N matrix tetap [MVP section 6](../../../docs/grpc-auth-mvp-spec.md#6-kontrak-mutasi-audit-dan-transactional-outbox). User/role aggregate_version memakai entity version committed, bukan auth_version. Session terminal revision 2 hanya diterima dari future revoke consumer; rotation tidak memerlukan session version column. Global revoke/user status memakai compound event policy existing.

Alternatif: independent audit/outbox commit atau broker publish dalam tx. Ditolak karena rollback domain akan meninggalkan event/audit sukses palsu atau broker down menghalangi mutation.

### 7. Idempotency hanya persistence, consumer tetap owner authorization

Consumer future menormalisasi actor/full method/canonical target/key dan memverifikasi identity/current endpoint permission. Di bawah scope lock, lookup committed result → fingerprint compare/replay → expected entity version untuk request baru → mutation/audit/event/safe result → commit. Replay tidak menolak expected version payload yang sudah tertinggal akibat commit pertama. Different target/key menghasilkan scope berbeda.

Fingerprint mencakup target, normalized fields, presence/field_mask, expected user/role version dan sorted set, bukan request/trace/deadline. Untuk input password future CreateUser, gunakan HMAC/keyed digest dengan key version terpisah dari JWT/refresh key; simpan format versioned dalam BYTEA tanpa schema column baru. Keyring verifikasi mempertahankan key yang dibutuhkan record unexpired 24 jam. Inject policy/key melalui konfigurasi runtime existing saat consumer diaktifkan; jangan menambahkan default secret atau membaca env dari repository. Tests menggunakan key fixture khusus tanpa nilai produksi.

Repository returns found/new/replay/conflict outcomes secara terstruktur; transport catalog mapping dilakukan consumer/error boundary. Scope expired diganti atomik setelah lock; unexpired record tidak dievict. Safe no-op baru dapat menghasilkan satu NO_OP audit/result, tanpa entity increment/event. Replay tidak menambah apa pun. Tidak menambah key field pada Register/self profile atau menyimpan UserTokenResponse.

Alternatif: in-memory key cache atau digest password tanpa key. Ditolak karena restart/concurrency tidak memenuhi 24-hour persistence dan digest lemah memungkinkan dictionary attack.

### 8. Wiring command MVP berikutnya adalah prerequisite tersendiri

Fondasi persistence tidak otomatis dipasang ke existing Register yang belum memiliki verified caller service. Jangan mengisi actor_service dengan klaim browser, hardcoded caller, atau nama producer sebagai pengganti identity caller. Transaction hardening existing tidak menandai command MVP selesai.

Saat security/principal foundation tersedia, change Register PENDING dan profil MVP wajib menulis mutation + entity version + audit + required outbox secara atomik sejak pengaktifan pertama. Required audit/outbox tidak ditunda setelah command baru berjalan. Counter expected_user_version/expected_role_version dan auth_version memiliki makna [MVP section 5.2](../../../docs/grpc-auth-mvp-spec.md#52-counter); fundamental fixture tests tidak mengklaim public consumers menerapkannya.

Target keamanan tetap access 15 menit, session/refresh absolut 7 hari tanpa sliding, skew JWT 30 detik, issuer ApplicationName, audience ignored, EdDSA-only saat Phase 3. Strict refresh client single-flight, busy vs consumed reuse, commit revocation lalu denial dan response-loss login ulang tidak berubah. Limiter angka/cap/trusted-IP [MVP section 8](../../../docs/grpc-auth-mvp-spec.md#8-rate-limiter-local-mvp) diterapkan change hardening, tidak diberi default angka baru di sini.

Alternatif: menambah fake actor atau noop audit writer agar fondasi terlihat terpasang. Ditolak karena akan menyamarkan prerequisite dan melemahkan mandatory command gate.

### 9. Error contract menjadi external gate yang konkret

api-contracts perlu menyediakan logical RESOURCE_BUSY auth untuk Register dan user untuk profile, mapping ABORTED, localized messages, serta dependency/cancellation semantics yang benar bila registry existing belum mendukungnya. Idempotency/version outcomes mendapat mapping fitur pada future command consumer; shared reuse hanya bila semantik existing tepat. Tidak membuat alias/sentinel public business error catalog lokal atau return string ad-hoc.

Jangan memakai common.ErrServiceUnavailable untuk unavailable DB target karena mapping sekarang UNIMPLEMENTED; jangan mengubah mapping global itu diam-diam. Server/helper encoder perlu mendaftarkan mapping fitur baru setelah contract tersedia. Keep Language → Error → authentication ordering dan redaction.

Catalog/generator/publish di api-contracts merupakan pekerjaan external, bukan task edit sibling otomatis. Setelah version tersedia, service pin dependency yang disetujui dan verifikasi workspace serta GOWORK=off. Jika belum ada, busy/dependency RPC integration gate BLOCKED; independent core persistence work masih dapat dikerjakan. Proposal artifacts complete bukan external readiness.

### 10. PostgreSQL harness opt-in dengan namespace disposable

User telah menyetujui pengecualian real-DB test pada 9 Oktober 2026. Target layout `tests/integration/postgres/{runner_test.go,test_scenarios,mocks,fixtures}`; semua Go files pada suite beserta scenario/helper khusus DB diberi build tag postgres agar root default suite tidak mengimpor/membuka DB tidak sengaja. Runner memakai testutils.Run, scenario .go biasa.

Test configuration eksplisit: `GRPC_AUTH_POSTGRES_TEST_ENABLED=1`, `GRPC_AUTH_POSTGRES_TEST_DSN`, `GRPC_AUTH_POSTGRES_TEST_DATABASE`. Config harness tidak membaca aplikasi env.yaml. Sebelum DDL/write, periksa current_database sesuai expected test database dan explicit opt-in; salah/missing config menghasilkan failure redacted. Dedicated DB dibuat/disediakan user/CI; suite hanya create/drop schema fixture miliknya, tidak create/drop database, public schema baseline, atau Docker.

Set search_path untuk **setiap physical connection**, termasuk replacement/pool connection, melalui test DSN/connection initialization; SET sekali pada pool tidak cukup. Schema fixture dibuat dari DDL yang telah dicocokkan, mencakup auth/profile/role/user_role dan audit/outbox/idempotency constraints yang diuji; gunakan synthetic fixture accounts dan default member, bukan baseline user atau seed produksi. Fixture SQL ini test-only, bukan migration source yang dijalankan ulang.

Gunakan connection A/B berbeda dan assert pg_backend_pid; pool minimal dua connection, serta barrier/channel/controlled callback untuk interleaving. Test cleanup unregister/close semua resource dan drop hanya namespace suite yang telah diverifikasi. Panic/cancellation/pool tests memakai bounds/deadline yang terkontrol; tidak mengandalkan sleep sebagai proof. Mock/default tests menguji call/ordering/error; actual repository PostgreSQL tests menguji committed state/exclusion.

## Risks / Trade-offs

- [Schema live berbeda atau canonical email collision] → preflight metadata/count read-only pada environment yang diotorisasi sebelum rollout; tanpa auto migration/merge. Missing schema adalah blocker dengan perubahan terpisah yang konkret.
- [External catalog belum tersedia] → B-01 BLOCKED untuk RPC error integration, tanpa local catalog sementara atau remap legacy; core tests tidak dianggap membuktikan error boundary.
- [Source ORM benar tetapi lifecycle belum teruji] → B-03 conditional hanya bila real-DB suite gagal; jangan mengganti ORM. Commit/cancellation dapat mempunyai unknown outcome dan dilaporkan sesuai evidence.
- [Guard policy global meningkatkan contention] → Register memakai guard untuk default-member consistency; profile independen per user. Optimasi granularity memerlukan invariant proof pada change berikutnya.
- [Stores belum dipakai public command] → deliverable adalah repository fungsional + actual atomic DB tests; tidak mengklaim end-to-end MVP, dan task ledger mencatat follow-up wiring wajib.
- [Test fixture berbeda dari schema deployment] → catat DDL/commit/PostgreSQL version, constraints dan source preflight; jangan pakai minimal schema yang menghilangkan invariant yang diuji.
- [Metadata/log membawa credential] → typed safe DTO allowlist dan assertions log/error/audit/outbox/result; report hanya metadata/count, tanpa DSN/password/email/raw SQL args.

## Migration Plan

1. Pada apply, pastikan source/HEAD/spec masih sesuai baseline; rekam schema metadata, unique/FK/defaults/columns dan canonical-email counts di environment yang diotorisasi. Saat propose ini tidak ada koneksi DB atau migration.
2. Selesaikan B-01 contract external sebelum mengaktifkan lock busy path melalui RPC. Bootstrap PostgreSQL test khusus/fixture dan jalankan tests bertahap sebelum menandai criterion PASS.
3. Terapkan lifecycle/read-write ownership dan canonical locking existing; test profile response identity serta sequential register. Deploy foundation hanya pada lingkungan sebelum security cutover dengan compatibility/proof yang tercatat.
4. Implementasikan stores dan buktikan atomicity/idem replay melalui production repository consumer fixtures. Tidak memasang relay atau audit actor palsu ke Register existing.
5. Jalankan targeted/default/vet/race/build yang relevan serta PostgreSQL suite; catat actual evidence dan outstanding MVP follow-ups. Jangan mark Phase 1 secara keseluruhan atau auth security readiness selesai.

Tidak ada migration baru yang diasumsikan. Rollback foundation binary tidak boleh menghapus user/password/audit/outbox/idempotency data; event pending dipertahankan. Sesudah security cutover pada change lain, rollback tidak boleh menyalakan lagi HS256/session lama atau melepaskan guard mandatory. Jika preflight memerlukan schema/data repair, buat scope terpisah dan keputusan user; tidak otomatis mengulang file 01.

## Verification and traceability

Source AC/M-ID mengacu [spec MVP](../../../docs/grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence), dengan AC-* mempertahankan ID spec utama. Setiap F-* adalah criterion **fondasi** lokal, bukan pengganti seluruh source AC. Evidence actual belum tersedia; hanya inspeksi source yang selesai.

| ID | Source reference | Capability / expected proof | Scope, prerequisite | Test / evidence target | Status |
| --- | --- | --- | --- | --- | --- |
| F-01 | M-07; AC-02 target tetap outstanding | Register atomic writes, token-after-commit, write/commit/panic failure | MVP_REQUIRED grpc-auth; schema/core tx | feature mocks + PostgreSQL committed row counts, no credential on error | NOT_STARTED |
| F-02 | M-06, M-07 | UpdateProfile owner/ID/fields dan failure propagation | MVP_REQUIRED grpc-auth; principal existing, bound reader | handler/feature scenarios + DB owner/rollback | NOT_STARTED |
| F-03 | M-07, M-09; D-15 | feature call sites Begin/Commit/Rollback/TryLock, ctx/handle identity, terminal cleanup, no stale handle | MVP_REQUIRED grpc-auth; core tx | source boundary review + feature mock ordering/call counts + DB cancellation/pool | NOT_STARTED |
| F-04 | M-09; AC-35 command follow-up | same/different key exclusion, canonical ordering, lifecycle | MVP_REQUIRED grpc-auth; PostgreSQL test, ORM v1.7.0 | backend IDs/barriers + lock reacquire/visibility counts | NOT_STARTED |
| F-05 | M-08, EXT-01 | auth/user RESOURCE_BUSY ABORTED, correct dependency/locale/redaction | EXTERNAL_REPOSITORY api-contracts → local integration; B-01 | catalog version, mappings, workspace/GOWORK=off tests | BLOCKED |
| F-06 | M-09 | canonical email/unique defaults preflight tanpa data cleanup | MVP_REQUIRED grpc-auth; authorized environment/schema | metadata/count report + Register/Login normalization tests | NOT_STARTED |
| F-07 | M-07, M-11 | typed safe audit dan failure rollback | MVP_REQUIRED grpc-auth; fixture verified actor, schema | reader/writer/consumer + DB safe payload/count assertions | NOT_STARTED |
| F-08 | AC-28 persistence subset; M-07, M-11 | pending outbox envelope/catalog/atomicity | MVP_REQUIRED grpc-auth; stores/schema | contract serialization + DB published_at NULL | NOT_STARTED |
| F-09 | AC-24, AC-25 repository subset; M-08 | scoped replay/conflict/different target/parallel claiming | MVP_REQUIRED grpc-auth; idem lock/store | fixed payload/version + PostgreSQL exact effect/audit/event/result counts | NOT_STARTED |
| F-10 | M-08 | 24-hour safe result/expiry/keyed fingerprint/no-op | MVP_REQUIRED grpc-auth; fixed clock, test key version | deterministic boundary tests + locked expiry replacement DB | NOT_STARTED |
| F-11 | M-04 lifecycle subset; AC-17 outstanding | explicit commit then denial vs write failure rollback | MVP_REQUIRED grpc-auth; production tx/store consumer fixture | DB committed security fixture despite error, no real refresh claim | NOT_STARTED |
| F-12 | M-09; user test exception | opt-in isolated DB/default suite/reproducible evidence | MVP_REQUIRED grpc-auth; B-02 | default/tag selection, configuration failures, DB version/report | NOT_STARTED |

| Dependency | Owner / acceptance | Current state |
| --- | --- | --- |
| B-01 | api-contracts: catalogs/mapping/version; local dependency pin and encoding tests afterward | BLOCKED for RPC busy integration: catalogs absent in inspected source; no external code authorized by propose |
| B-02 | User/CI test environment: explicit dedicated PostgreSQL database/DSN/opt-in, schema fixture compatibility | Configuration not provided or checked; gate BLOCKED if absent at execution |
| B-03 | ORM owner: repair/release only for a proven lifecycle/capability gap | Conditional; no source-proven PostgreSQL early-unlock blocker |
| B-04 | Follow-up grpc-auth security/command changes: verified service/user principal, PENDING activation/session enforcement, atomic production audit/event/key wiring | MVP_REQUIRED outside increment; not downgraded to POST_MVP |

PASS report must include commit/module versions, command, PostgreSQL/environment version, execution time, expected/actual, and redacted evidence. Core stores/fake consumer tests cannot satisfy full AC-17/19/20/33–35, actor privilege-change or complete MVP command matrix. Those tests must use actual implemented command/security flows in their follow-up changes.

No unresolved business decision blocks authoring these artifacts. Database availability, live metadata and published contract version are execution prerequisites with explicit gates, not business defaults selected silently.
