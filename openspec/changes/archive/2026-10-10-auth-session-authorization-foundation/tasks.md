# Tasks

Progres dan actual evidence dicatat di [evidence](evidence.md). Scope hanya grpc-auth. Task tetap unchecked sampai prerequisite dan evidence sesuai approved source/deployment separation tersedia, tanpa dummy contract, fallback atau edit sibling. Gate eksternal berarti memverifikasi deliverable yang diselesaikan lewat pekerjaan terpisah, bukan mengimplementasikannya di sini. Rujukan status/AC: [design ledger SAF-01..11](design.md#acceptance-dan-evidence-ledger).

User menunda pekerjaan rest-orc pada 10 Oktober 2026. Pemeriksaan internal grpc-auth terbaru tercatat di [readiness](activation-readiness.json), [local caller proof](grpc-auth-caller-verification.json) dan [schema/admin preflight](grpc-auth-readiness-preflight.json). User kemudian menyetujui source wiring/test grpc-auth sekarang; EXT-02 tetap mandatory deployment prerequisite yang belum PASS, dilacak pada Phase 6 DEP-01 spec MVP.

Keputusan source/deployment separation berlaku sesuai design section 2. Source wiring/task 7.2–7.4 dan backend acceptance 8.4 dapat diselesaikan setelah backend evidence PASS; mandatory deployment gate Phase 6 DEP-01 berada di luar checklist increment ini. Tidak ada partial method switch atau runtime bypass.

## 1. Preflight dan dependency ledger

- [x] 1.1 Cocokkan baseline source dengan commit design, six full methods, generated messages/error catalog dan pin published api-contracts; deliver gap manifest aman untuk EXT-01 tanpa mengubah sibling/generated/dependency.
- [x] 1.2 Catat actual schema auth/session/RBAC/audit/outbox fields, types, indexes/uniqueness/ownership constraints dan ACTIVE admin melalui read-only preflight yang diotorisasi saat implementation; verify report membedakan observed dari belum tersedia dan tidak menjalankan migration/seed/reset. Schema gap memblokir consumer terkait sampai scoped migration disetujui terpisah.
- [x] 1.3 Catat prerequisite change/deliverable producer Login/EdDSA/basic limiter, EXT-01, EXT-02 serta owner dan readiness evidence di ledger; verify activation tetap BLOCKED tanpa producer dan full Phase 2 bukan prerequisite producer.
- [x] 1.4 EXT-01: verifikasi target safe additive fields/error catalog/mapping/locale telah published dan dependency update bila diperlukan telah diotorisasi terpisah; catat versi serta workspace/GOWORK=off build evidence tanpa local proto/catalog fallback.

## 2. Service caller trust boundary

- [x] 2.1 Tambahkan typed env.yaml TLS/caller digest/expiry/current-previous overlap/method/namespace config dan startup validation; root tests/configs menguji missing/invalid config, no secret default, explicit previous expiry dan error cleanup.
- [x] 2.2 Implement caller verifier dan immutable verified service context, constant-time digest comparison, separate x-service-token metadata dan exact method/namespace policy; root tests/interceptors/sessions menguji valid/missing/expired/ambiguous credential, rotation boundary, caller spoof dan denied method/namespace dengan controlled clock.
- [x] 2.3 Hubungkan TLS/caller verifier ke production server construction dengan locale/error/context boundary existing; root configuration/interceptor integration memakai in-memory transport menguji TLS CA/hostname rejection, semua six methods service-authenticated, TesRPC tetap ada dan startup failure tidak meninggalkan resource.
- [x] 2.4 Dokumentasikan config shape tanpa credential asli, rotation/not_after dan error contract dalam docs grpc-auth; verify contoh sesuai loader, local links valid dan SAF-01 evidence menunjukkan scope component, bukan external caller readiness.

## 3. Session persistence dan credential verifier

- [x] 3.1 Perluas domain auth/session dengan authoritative status/auth_version/must_change_password/entity version dan session UUID/ownership/absolute timestamps; implement bound reader/writer yang memakai injected handle/context. Root repository tests memverifikasi field mapping, missing row/dependency/cancellation dan tidak ada repository Begin/Commit/TryLock atau session version column baru.
- [x] 3.2 Implement EdDSA verifier contract serta typed verified claims/principal context; root jwt/sessions tests mencakup wrong algorithm/signature/issuer, missing/malformed sub/sid/jti/iat/exp/auth_version, legacy user_id mismatch, audience ignored, 15m/7d/skew30s boundary dan safe missing principal error.
- [x] 3.3 Implement authoritative user/session validation dengan primary reader; root feature tests mencakup ACTIVE/owner/version/expiry, PENDING/SUSPENDED/DEACTIVATED, revoked/missing session, reactivation tanpa resurrection, unavailable versus denial dan forwarded request context.
- [x] 3.4 Dokumentasikan contract session/verifier yang dikonsumsi real Login prerequisite, tanpa mengimplementasikan issuer/refresh lifecycle di task ini; verify producer interface cukup untuk session/refresh/audit satu tx dan SAF-02 evidence tidak mengklaim real Login/activation PASS.

## 4. Primary authorization snapshot dan existing read consumers

- [x] 4.1 Implement RBAC snapshot reader/resolver active assignment/role/permission + business application/access, preserving source roles; root repository/feature tests menguji union, partial revoke, inactive gates, no role_code bypass, no JWT/cache grant dan self-service tanpa admin/app grant.
- [x] 4.2 Implement same-statement authorization+response snapshot untuk Me/ListUsers termasuk actor state saat page kosong; verify generated SQL dan root tests tidak memisahkan auth/data snapshots atau memakai pool fallback. Bila ORM tidak mampu, dokumentasikan bukti dan gunakan feature-owned read-only RepeatableRead contract kecil sesuai design, tanpa mengganti isolation mutation global.
- [x] 4.3 Hubungkan existing Me/ListUsers consumer ke verified service/claims dan authoritative snapshot; root feature/handler tests menguji iam.user.read, restricted Me minimum, restricted ListUsers deny, cross-user input, empty-page denial, dependency/locale mapping serta preserved filters/auth_id/last_id/numeric role_codes/TesRPC contracts.
- [x] 4.4 Hapus shared-err concurrent Me read path hanya dalam consumer yang diganti; root regression scenario dan race test membuktikan response snapshot konsisten tanpa data race, lalu catat SAF-03/04 component evidence serta management RPC yang tetap follow-up.
- [x] 4.5 Dokumentasikan exact six-method matrix, restricted full allowlist dan primary snapshot semantics di docs; verify matrix cocok dengan registered methods dan tidak mendaftarkan unavailable management/lifecycle RPC.

## 5. Mutation recheck dan production persistence consumers

- [x] 5.1 Perluas feature UpdateProfile memakai Begin(requestCtx) → ordered actor user/session TryLock → current actor/session/owner/restricted recheck → bound read/write → Commit; root mocks menguji handle identity/order/count, no fallback pool, RESOURCE_BUSY versus dependency, cancellation/panic/commit cleanup dan stale actor denial.
- [x] 5.2 Implement Register PENDING/token field 1 kosong/no session/no first-admin dengan verified service actor, USER_REGISTERED audit dan pending iam.user.created.v1 dalam tx existing; root scenarios menguji default nonadmin member, duplicate/privilege attempt, safe payload dan setiap mutation/audit/outbox/begin/commit failure. Additive response mapping memakai EXT-01 published; source wiring memerlukan producer/local backend readiness dan deployment memerlukan Phase 6 DEP-01.
- [x] 5.3 Hubungkan effective UpdateProfile mutation, user version +1, USER_PROFILE_UPDATED audit dan pending iam.user.profile_updated.v1 pada tx yang sama; root scenarios memverifikasi safe self fields, legacy full replacement, auth_version/session unchanged, event aggregate version dan rollback setiap required write.
- [x] 5.4 Implement identical profile replacement changed=false/NO_OP audit tanpa event/version increment; root scenarios memverifikasi one-audit counts, tidak menambah general idempotency Register/Login/profile dan tidak menyimpan credential untuk replay/response-loss.
- [x] 5.5 Implement independent safe failed-attempt audit setelah rollback/denial pada scoped authentication/commands; root failure-injection tests membuktikan unknown actor tidak dipalsukan, audit failure tetap deny/dependency, credential redacted dan telemetry bounded.
- [x] 5.6 Dokumentasikan command mutation/audit/outbox/no-op/response-loss matrix dan consumer handoff management idempotency; verify action/event/envelope cocok catalog dan AC-24..26 belum diklaim PASS tanpa production management consumer.

## 6. Opt-in PostgreSQL proof untuk foundation consumers

- [x] 6.1 Tambahkan scenarios di tests/integration/postgres untuk session/RBAC primary reads dan same-snapshot Me/ListUsers termasuk page kosong; verify dedicated opt-in/tag/DSN guard serta default suite tetap tanpa real DB. Jalankan test database yang diotorisasi dan catat physical backend IDs/barriers serta post-commit role/app change terlihat tanpa session/auth_version change; unavailable → BLOCKED.
- [x] 6.2 Uji actor status/session revocation committed sebelum locked UpdateProfile recheck, lock contention/retention, cancellation/rollback/panic/pool reuse memakai dua physical connections dan controlled barriers; verify tidak ada unauthorized mutation atau success audit/event, bukan ordering berbasis sleep.
- [x] 6.3 Uji actual Register/Profile production command transactions, tentative mutation + required audit/outbox failure, commit outcome, version/no-op dan pending row counts; verify atomik tanpa broker dan tanpa baseline DB/migration/Docker. Catat SAF-05/06 evidence atau BLOCKED, bukan PASS karena test di-skip.
- [x] 6.4 Dokumentasikan command/tag/explicit opt-in database requirements dan safe report pada evidence change; verify report mencantumkan commit/module/time/environment/expected-actual tanpa secret dan tidak memakai historical foundation evidence sebagai hasil baru.

## 7. Producer integration dan activation readiness

- [x] 7.1 SAF-08: terima evidence increment prerequisite real Login membuat session/refresh/SESSION_CREATED audit atomik dan issuer EdDSA dengan required claims/TTL; jalankan integration consumer terhadap production producer yang sama, verify credential hanya keluar setelah commit serta failure generic/rollback/redaction. Tidak mengimplementasikan producer atau synthetic penggantinya lewat task ini.
- [x] 7.2 SAF-08 backend limiter/caller readiness: verify production quotas/buckets/TTL/cleanup/cap/fallback and RetryInfo encoding using controlled-clock/layered/concurrency tests plus actual local caller verification. Record EXT-02 IP sanitization/client retry evidence as mandatory deployment gate Phase 6 DEP-01; do not claim external readiness from config/tests.
- [x] 7.3 SAF-09 local backend readiness: verify actual local key sign/verify/HMAC, CA/hostname/TLS pair, caller allowlists/expiry and rotation behavior tests, fresh read-only schema/minimal ACTIVE admin; deliver cutover/rollback checklist. Actual deployment/consumer PENDING/login ulang/rotation readiness remains mandatory Phase 6 DEP-01.
- [x] 7.4 SAF-10 after backend 7.1..7.3: wire the complete default JWT/session/policy pipeline and real Login/PENDING Register/read/profile consumers for every existing registered RPC together. Root integration through real Login proves Me/Profile/ListUsers success/deny, restricted behavior, caller/TLS checks, localized dependency errors, HS256/sid-less rejection, no bypass, startup cleanup and limiter cleanup worker cancellation/join. No deployment.
- [x] 7.5 Catat component-ready versus activation-ready dan remaining Phase 1/2/3 AC di evidence/design; verify tidak mencentang mandatory readiness task berdasarkan fixture atau menutup keseluruhan Phase 1 karena increment transaction sudah archived.

## 8. Final integration verification dan review

- [x] 8.1 Setelah targeted tests tiap group PASS, jalankan gofmt perubahan Go, rtk go test ./..., rtk go vet ./..., relevant race suites dan rtk proxy make build; verify hasil/limitations tercatat dan shared-state/server wiring regressions tidak tersisa.
- [x] 8.2 Published dependency verification: ulang integration/build workspace dan GOWORK=off serta required opt-in PostgreSQL suites pada deliverable versions; verify contract/TLS/session/authorization/persistence integration evidence nyata, tidak memperluas scope ke full refresh/management follow-up.
- [x] 8.3 Jalankan openspec validate auth-session-authorization-foundation --strict dan specs validation, link/AC/matrix review, git diff --check/status; verify touched files sesuai authorized implementation scope, tidak ada sibling/generated/secret artifact atau unapproved dependency/migration change.
- [x] 8.4 Finalize backend evidence with actual source/module identity, commands/time/safe environment and registered-RPC expected/actual results after required tests PASS. Refer to the unchecked Phase 6 DEP-01 in the global MVP spec, preserve follow-up owners/status and production deployment BLOCKED; do not claim Phase 1/3 production completion.

## Handoff deployment ke phase terakhir

Atas instruksi user 10 Oktober 2026, task 9.1 dipindahkan seluruhnya ke
[Phase 6 DEP-01](../../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).
Gate tetap mandatory dan belum PASS; tidak dicentang sebagai selesai. Foundation
hanya melacak implementation/backend verification: **36/36 task selesai**.
Penyelesaian ini bukan production/deployment readiness atau seluruh Phase 1/3.
