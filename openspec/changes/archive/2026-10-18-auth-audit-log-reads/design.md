# Design

## Context

Motivasi dan scope ada pada [proposal](proposal.md). Baseline 18 Oktober 2026 adalah working tree yang berisi perubahan user; HEAD/graph bukan gambaran lengkap source terkini. Graph `built_at_commit=2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec` dipakai untuk routing: structural hubs domain auth/outbox/user_role/transaction/snapshot, serta jalur feature → verified audit actor → AuditWriter. Link calls EXTRACTED diverifikasi terhadap source; inference tidak dipakai sebagai bukti behavior.

| Source yang diperiksa | Fakta existing / konsekuensi |
| --- | --- |
| `internal/repositories/domain/audit.go`, `writer/audit.writer.go`, `pkg/types/audit.go` | AuditRecord memakai ID int64, nullable actor user/service, metadata dan JSON safe snapshots; writer menerima tx feature, tanpa read API |
| `internal/repositories/domain/snapshot.go` | SafeSnapshot allowlist dan strict JSON decoder tersedia; response tidak boleh menyalin JSON/map arbitrer |
| `internal/features/catalog/impl.go`, `read.go`, `user/read_snapshot.go` | Feature rechecks current identity/exact permission sebelum membaca data pada satu snapshot; injected binder/clock/provider bisa dipakai sebagai pola |
| `pkg/database/transaction.go`, `features/common` | BeginSnapshot membuka primary REPEATABLE READ READ ONLY dengan leased cleanup; feature/helper memiliki Begin/Commit/Rollback call sites |
| `domain/catalog.go`, `catalog_cursor.go`, `reader/catalog.go`, `handler/catalog.go` | Catalog dynamic sort/projection berbeda dari pilihan user; cursor INTEGER assumptions tidak cocok untuk audit BIGINT |
| `internal/configs/foundation.go`, session/rate/security-attempt interceptors | Default 36 methods/eight services; exact switches perlu menambahkan audit; successful ordinary reads tidak diaudit; failure recorder independen dan rate rejection tidak menulis DB |
| `docs/sql/user-management/01_schema_backfill.sql`, opt-in PostgreSQL fixture | audit_logs BIGINT identity, timestamps, JSONB dan index time/actor/target tersedia; keberadaan file tidak membuktikan live schema |
| `api-contracts/proto/paginator/v1/paginator.payload.proto`, `proto/authorization/v1/`, common dictionary | Shared PageQuery/PageMeta tersedia; `proto/audit/v1/` belum ada. Published baseline `v0.0.0-20261018052225-bfc60a40a858`; ORM `v1.8.0-support-subquery` |
| `tests/features/catalog`, `tests/handler/test_scenarios/catalog.go`, `tests/repositories/role/test_scenarios/catalog.go`, `tests/integration/postgres/test_scenarios/catalog_snapshot.go` | Existing scenario runners, strict call order/handle tests, SQL argument checks dan two-physical-backend barrier dapat diadaptasi tanpa framework baru |

## Goals / Non-Goals

**Goals:** implementasi kecil untuk administrative audit history; bounded exact query dan pagination; satu authority/data snapshot; safe typed output dan cleanup yang dapat dibuktikan. Keputusan user 18 Oktober 2026 mengunci query minimal, bukan dynamic catalog.

**Non-Goals:** tidak menambah generic query framework atau memaksa audit masuk registry catalog; tidak mengubah audit producers, schema, mutation/lock/idempotency semantics atau permission scope audit menjadi session administration. Batas bisnis lain mengikuti proposal.

## Decisions

### 1. Small audit feature with current administrative authority

Tambahkan feature `internal/features/audit/` dengan small service/dependencies, binder state/permission/audit readers, injected clock dan ReadSnapshotProvider. Handler mengadaptasi protobuf ke domain query dan hasil domain ke typed transport. Reader `internal/repositories/reader/audit.go` memakai handle yang diberikan feature. Reuse shared constructor/pipeline di foundation, tanpa connection per request.

Exact method adalah `/audit.v1.AuditService/ListAuditLogs`. TLS dan verified service credential harus diizinkan exact method serta namespace `iam`; ACTIVE unrestricted user/session memerlukan `iam.audit.read`. Tambahkan permission constant; resolver active-role union dan soft-delete/application gates tetap authoritative. Numeric admin/auditor role name, browser identity dan JWT role snapshot tidak memberi bypass. IAM tidak membutuhkan user application-access grant.

Actor filter berarti historical record selector, bukan caller identity. Pemegang permission boleh membaca history user lain serta verified-service attempts; tidak memerlukan `iam.user.read` atau `iam.session.read` dan tidak memperoleh live session detail. Alternative self-only/filter-as-owner ditolak karena kontrak administrative audit. Application namespace caller mengamankan RPC IAM, bukan menyaring audit berdasarkan namespace target RPC bisnis yang tercatat.

### 2. Freeze an additive typed contract before backend wiring

Rencana source ownership: `api-contracts/proto/audit/v1/audit.payload.proto` dan `audit.service.proto`, generated `pb/audit/v1/`, contract tests/docs. Existing descriptors/tags/types tidak berubah. Capture prior 36 methods/eight services dan semua prior descriptors; target tepat 37/sembilan.

| Message | Tag / type / meaning |
| --- | --- |
| ListAuditLogsRequest | query=1 PageQuery; filter=2 AuditLogFilter; nil whole request invalid, nil query/filter memakai defaults |
| AuditLogFilter | actor_user_id=1 optional uint64; actor_service=2 string; action=3 string; target_type=4 string; target_id=5 string; occurred_from=6 Timestamp; occurred_before=7 Timestamp |
| AuditLogItem | id=1 uint64; actor_user_id=2 optional uint64; actor_service=3 optional string; action=4 string; target_type=5 string; target_id=6 string; safe_before=7 AuditSnapshot; safe_after=8 AuditSnapshot; reason=9 optional string; request_id=10 optional string; trace_id=11 optional string; occurred_at=12 Timestamp |
| AuditSnapshot | version=1 int64; status=2 string; name=3 optional string; description=4 optional string; address=5 optional string; phone=6 optional string; role_ids=7 repeated uint64; permission_codes=8 repeated string; application_id=9 uint64; session_id=10 string; reason_code=11 string; no_op=12 bool |
| ListAuditLogsResponse | items=1 repeated AuditLogItem; meta=2 PageMeta; next_cursor=3 string |

Audit IDs 1..MaxInt64 merepresentasikan BIGINT, tidak dibatasi MaxInt32 atau diubah menjadi float. Actor user ID present harus 1..MaxInt32; present zero invalid. Optional/null actor dan metadata serta nil versus present snapshot dipertahankan. Session ID adalah identifier historis yang telah diizinkan safe snapshot, bukan credential atau izin membaca session admin API.

Reuse common bad-request/forbidden/dependency/cancellation/internal catalogs, auth invalid-session dan locale mapping. Tidak ada business error baru yang dibutuhkan; bila implementation menemukan gap yang nyata, catat dan selesaikan di dictionary feature audit milik api-contracts, bukan string ad-hoc atau remap global. Sibling edits memerlukan otorisasi scope baru; publication Git memerlukan otorisasi yang berlaku. Proposal ini tidak melakukan keduanya.

### 3. Exact optional filters and fixed latest-first pagination

String filter di-trim; kosong berarti absent, case dipertahankan dan matching exact case-sensitive. Batas UTF-8/rune storage: actor_service/action/target_id 100, target_type 50; invalid UTF-8/control characters ditolak. Unknown well-formed action/type/service/target menghasilkan empty result, bukan NOT_FOUND. target_id memerlukan target_type agar selector tidak ambigu; target_type sendiri boleh. Tidak memvalidasi target melalui live tables atau menganggap target ID selalu numeric: target `rpc` adalah full method, target `session` UUID dan user/role textual ID.

Semua filter yang ada di-AND; actor_user_id dan actor_service juga AND bila keduanya dikirim karena schema historical actor tidak menetapkan XOR. Tidak ada implicit OR, browser-controlled actor identity atau live account-status filtering. Kedua time bounds opsional; valid protobuf timestamps dikonversi UTC dan memakai `occurred_at >= from AND occurred_at < before` dengan perbandingan setara pada presisi PostgreSQL; from harus lebih kecil dari before bila keduanya ada. Tidak ada default rolling window atau max range baru.

PageQuery dipakai hanya untuk page/limit/last_id: default page1/limit20, limit maksimal100, negative dan arithmetic overflow invalid. Semua nonempty fields/sort atau present search ditolak INVALID_ARGUMENT, termasuk request sort yang meniru default. Ini menjaga pilihan user terlihat pada contract; ignore parameter diam-diam ditolak sebagai alternatif.

Order selalu occurred_at DESC lalu id DESC. last_id menerima hanya opaque versioned base64url cursor, bukan numeric legacy cursor. Payload canonical memuat method/version, hash normalized filters dan tuple timestamp UTC pada presisi tersimpan + positive int64 ID. Maksimal4096 byte; malformed/type overflow/unknown fields/trailing data/query mismatch/cross-method invalid. Hash bukan signature atau authorization; cursor selalu diperlakukan sebagai input tak terpercaya dan authority diperiksa setiap call.

Predicate next page lexicographic `(occurred_at < t) OR (occurred_at = t AND id < i)`, parameterized. Cursor mengambil limit+1, tanpa offset; tanpa cursor page>1 memakai deterministic offset dengan overflow guard. Cursor mengalahkan offset; page tetap metadata requested/default, tidak dianggap token authorization. Hash mengikat semua normalized filters dan fixed ordering, bukan page/limit; klien dapat mengganti page size atau metadata page tanpa mengubah filter. total menghitung seluruh matching records sebelum cursor/offset/limit pada snapshot yang sama; total_pages=ceil(total/limit), empty total/pages0; has_next dari limit+1; has_prev dari cursor/page>1. next_cursor hanya bila has_next. Meta int32 overflow tidak boleh wrap; suppressed response dan safe dependency error bila count tidak dapat direpresentasikan.

Alternative reuse CatalogCursor mentah ditolak: numeric ID compatibility dan INTEGER bounds di sana tidak cocok. Reuse helper netral hanya bila benar-benar mengurangi duplication tanpa mengubah RPC lama. Pagination tidak mempertahankan database snapshot lintas request; audit baru dengan timestamp lama dapat masuk halaman berikutnya. Ini listing, bukan frozen export.

### 4. One feature-owned primary read snapshot with safe finalization

Alur: verified service/session/admission → handler normalization → feature checks → BeginSnapshot(requestCtx) → bind same tx → reauthenticate account/session/restriction dan current exact permission → count matching audit → read page → strict safe mapping → Commit → response. Tidak ada fallback replica/pool read atau external network di snapshot. Audit history tidak memerlukan actor/target live join; inactive historical users tetap terbaca oleh auditor saat ini.

Feature/features-common adalah satu-satunya owner Begin/Commit/Rollback. Provider existing menyediakan REPEATABLE READ READ ONLY dan leased resource cleanup. Tidak ada TryLock, last-admin/policy guard, expected entity version, persistent fingerprint atau no-op command pada read ini. Protocol mutation existing idem → last-admin/policy → account/role-code → user → role → application → session tetap berlaku di command owners, tanpa perubahan. PostgreSQL advisory-xact-lock pada mutation tetap dilepas finalization; read baru tidak memperolehnya.

Snapshot yang ditetapkan setelah committed revocation harus menolak actor yang kini invalid/tanpa permission. Snapshot in-flight boleh menyelesaikan state lama yang konsisten; tidak menjanjikan cancellation karena revocation setelahnya. Privilege mutation tidak mencabut sesi atau menaikkan auth_version; password/global revoke/suspend/deactivate tetap invalidasi global, reactivate tidak memulihkan sesi. Failed begin/state/permission/count/page/decode/finalization → UNAVAILABLE dengan cause preserved dan tanpa partial page; cancellation/deadline → matching gRPC code, panic → redacted INTERNAL; cleanup selalu selesai sebelum independent attempt. Auth invalid → UNAUTHENTICATED, restricted/missing authority → PERMISSION_DENIED, malformed input → INVALID_ARGUMENT. Authorized empty → success total0, bukan substitute error.

### 5. Typed safe history and existing side-effect policy

Read hanya column metadata yang dibutuhkan dan JSON safe_before/safe_after. Decode JSON dengan strict SafeSnapshot allowlist/validation; lalu map eksplisit ke AuditSnapshot. Unknown keys, malformed JSON atau invalid stored fields menghasilkan safe dependency error untuk seluruh halaman, bukan raw JSON, silent dropping row, truncated credential atau partial success. SQL NULL snapshot berbeda dari valid empty object; JSON literal null/nonobject invalid untuk snapshot non-NULL. Tidak ada password/token/hash/header, arbitrary payload, internal error/stack atau enrichment credential. Text reason/correlation mengikuti bounded stored-safe contract; read ini tidak mengklaim memperbaiki data legacy yang melanggar write contract.

| Outcome | Business/version/session writes | Audit | Outbox / idempotency |
| --- | --- | --- | --- |
| Successful page/empty page | 0 | 0 success/read audit | 0 / 0 |
| Error with verified service | 0 | Existing one SECURITY_ATTEMPT_FAILED safe service attempt sesudah read cleanup; target RPC, safe catalog reason; tanpa payload/filter/result | 0 / 0 |
| Quota/capacity rejection | 0 | Bounded limiter telemetry; 0 per-rejection DB attempt | 0 / 0 |
| Attempt persistence failure/panic | 0 | Original error tetap; bounded failure observation existing | 0 / 0 |
| Response loss/retry/same incidental idempotency-key | 0 | Successful calls tetap 0 | Fresh authorized read, no saved replay |

Tidak ada audit-of-success loop. Failure audit terpisah boleh terlihat pada read berikutnya; tidak masuk successful read snapshot yang sudah ditutup. Preserve independent recorder timeout/cancellation cleanup existing tanpa double audit atau recursive ListAuditLogs call.

### 6. Admission and compatibility

Tambahkan exact method ke session IAM/recognized protected methods dan rate switch. Shared protected-read 120/minute burst20, maximum10000 entries, idle15m/cleanup1m, RetryInfo dan localized RESOURCE_EXHAUSTED tetap. Invalid input setelah verified identity ikut quota; unverified identity tidak mengalokasikan bucket. Tidak membuat bucket berdasarkan arbitrary actor/filter/cursor. Example allowlist exact37 methods; runtime caller secrets/policy tidak disentuh.

Prior36 methods, TesRPC, numeric ListUsers cursor/dynamic catalog contracts, PENDING/token-empty Register, generic Login denial dan credential-only Login/Refresh tetap. EdDSA-only, issuer ApplicationName, audience ignored, access15m/session7d absolute/skew30s, strict refresh rotation/reuse commit-on-denial dan restricted allowlist Me/ChangePassword/Logout tetap. Auditor tidak otomatis memperoleh session-admin permission.

## Risks / Trade-offs

- [History query besar atau full count mahal] → indexed default time/ID order, bounded page, parameterized filters dan caller deadline/shared quota; no new SLA/index migration tanpa evidence dan scope baru.
- [Audit ID dipotong helper INTEGER] → dedicated BIGINT validation, cursor dan tests dengan ID di atas MaxInt32 sampai MaxInt64.
- [Dirty working tree dan graph stale] → capture safe source/dependency baseline saat apply, preserve unrelated hunks dan verified fixture rename, catat deliverable digests serta actual commands.
- [Legacy JSON di luar contract] → fail closed whole page dan redacted error; remediation data terpisah, jangan mengubah/menyembunyikan history otomatis.
- [Cursor tidak menyediakan frozen export] → document per-call snapshot dan concurrent insertion semantics; no persistent cursor/session cache.
- [Source contract belum published] → block backend wiring sampai scope/published pin valid, verify GOWORK=off; local workspace build tidak cukup.

## Migration Plan

Tidak ada schema migration, seed, credential invalidation atau application DB access. Saat apply: scope contract authorization → additive source/generator/tests → separately authorized publication → pin published module → backend/read registration/example/docs → controlled dan opt-in PostgreSQL gates → source/evidence review. Sync/archive dilakukan workflow berikutnya. Deployment/caller activation, audit retention/archival dan consumer readiness tetap Phase6 DEP-01. Rollback deployment menghapus exact method dari caller policy dan memakai prior backend/compatible contract; tidak menghapus audit rows atau membuka legacy authentication fallback.

## Acceptance Ledger

Planning baseline: semua behavior baru NOT_STARTED; dependency contract ALR-EXT-01 BLOCKED sampai authorization/publication. Existing implementation/evidence increment lain hanya prerequisite, bukan PASS change ini. Setiap gate wajib actual source/contract commit atau digest, command, environment aman, waktu, expected/actual dan result. Default mocks bukan pengganti PostgreSQL.

| ID / mapped criteria | Scope / owner | Prerequisite | Required test / evidence | Status |
| --- | --- | --- | --- | --- |
| ALR-EXT-01 / EXT-01 | EXTERNAL_REPOSITORY / api-contracts | New sibling scope dan Git publication authorization | Wire/tag/presence/typed snapshot/37-method compatibility, generator round-trip, tests/vet, published module/checksums dan standalone resolution | BLOCKED |
| ALR-01 / AC-31, M-01/02/03/05 | MVP_REQUIRED / grpc-auth | Contract + auth/resolver foundation | Registered TLS/service exact method+IAM, active/session/restriction, auditor-only, spoof/cross-user filters, role numeric/name negative; current permission revoke | NOT_STARTED |
| ALR-02 / M-05/06 | MVP_REQUIRED / grpc-auth | Typed filter/domain/reader | Nil/default/presence, AND/unknown/no match, time [from,before), fixed order ties, offset/cursor/BIGINT/overflow and forbidden search/sort/projection; SQL args and meta | NOT_STARTED |
| ALR-03 / M-06/07 | MVP_REQUIRED / grpc-auth | SafeSnapshot/typed mapping | Nullable actors/snapshots/fields, historical inactive targets, allowlist/no credential, corrupt/unknown JSON whole-page error; no partial response | NOT_STARTED |
| ALR-04 / M-02/07/09 | MVP_REQUIRED / grpc-auth | Primary snapshot/cleanup + dedicated test DB | Strict Begin/read/commit/rollback handle/context and failures; two physical backend barrier authority/count/page consistency with concurrent insertion, permission/session/account revoke; next call new state, pool reuse normal/race | NOT_STARTED |
| ALR-05 / M-10/11 | MVP_REQUIRED / grpc-auth | Existing admission/attempt recorder | Shared quota invalid input, RetryInfo/locale, zero success writes, error one safe independent attempt after cleanup, audit failure/panic, zero rate DB attempt; exact SQL counters/DB rows | NOT_STARTED |
| ALR-06 / AC-31, M-05/06/07 | MVP_REQUIRED / grpc-auth | Published pinned contract + evaluator baseline | Exactly37/sembilan registration, prior36/descriptor/auth/credential/cursor regression, default/race/vet/build workspace+GOWORK=off, full opt-in PostgreSQL normal/race, final docs/source identity | NOT_STARTED |
| DEP-01 | EXTERNAL_REPOSITORY / deployment + consumers | Mandatory backend increments complete | Actual consuming-client/caller/key/schema/retention/cutover readiness | Separate Phase6, not claimed PASS |

Ledger mempertahankan makna [AC/M criteria](../../../../docs/grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence). Full Phase4 mutation matrix, Phase5 hardening, broker delivery/consumer dan CLM resource policy tetap terpisah.

## Implementation status

Backend COMPLETE on18 October2026; all22 tasks and required contract/default/race/vet/build/dedicated PostgreSQL normal/race gates pass in workspace and standalone modes. Planning baseline statuses above are historical. Actual ALR-01..06/ALR-EXT-01 results and immutable dependency/source identity are in [implementation evidence](evidence/progress.md). Phase6 DEP-01 remains separate; main-spec sync/archive has not been performed.
