# Apply evidence

## Archive — 10 Oktober 2026

Archived after all 36 tasks and planning artifacts completed. All 15 ADDED
requirements were synchronized into four main capabilities, preserving the
existing eight command-persistence requirements. JSON verification reports and
`.openspec.yaml` were retained unchanged; Markdown links now resolve from the
archive. This archive changes no application code and reruns no application/DB
tests. Mandatory Phase 6 DEP-01 remains unchecked / deployment BLOCKED; backend
increment completion does not claim whole-MVP completion.

## Status

Foundation implementation **PASS: 36/36 tasks**. User moved the former pending
9.1 deployment task to mandatory [Phase 6 DEP-01](../../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment),
the final phase. The gate remains unchecked / deployment BLOCKED, outside this
backend change. No readiness requirement was passed or dropped by this move.

Backend source/tests remain as verified in [server-verification.json](server-verification.json).
Historical reports and entries below retain their original 36/37 allocation and
commands/results. This docs-only revision changes ownership/timing of the
remaining gate, with no code edit, test rerun, deployment, archive or whole-MVP
completion claim. rest-orc remains deferred.

## Source dan contract — task 1.1

- HEAD grpc-auth dan graph built_at_commit sama: `b89f37270d019c8d0d7823a3ff5d3526874d2232`.
- `go.mod` module `github.com/siti-nabila/grpc-auth`; ORM v1.7.0; api-contracts `v0.0.0-20261010044649-bfbbeb282ec7`.
- `rtk proxy go list -m -json github.com/siti-nabila/api-contracts` resolve workspace sibling. `rtk proxy env GOWORK=off GOPROXY=off go list -m -json github.com/siti-nabila/api-contracts` resolve pinned cached module; ini module resolution, bukan standalone test/build target contract.
- 13 relevant generated/catalog/mapping files dibandingkan byte/hash workspace dengan pinned module: semuanya identik. Full six-method inventory, field tags dan SHA-256 ada di [contract-preflight.json](contract-preflight.json).
- Server tetap tanpa inbound TLS/service verifier, bearer HS256/UserId-only context, Login tanpa session/refresh persistence, model Go auth/role/assignment masih legacy. Source call sites dibaca, bukan disimpulkan dari graph inference.

| Gap | Current verified contract | Deliverable EXT-01 yang diperlukan |
| --- | --- | --- |
| CON-01 | UserTokenResponse hanya token string field 1 | Registration ID/status additive; token 1 tetap dan kosong untuk Register |
| CON-02 | Tidak ada refresh/session/expiry/restricted fields | Login token tetap access; additive refresh_token/session_id/access_expires_at/session_expires_at/must_change_password |
| CON-03 | UserData identity/profile/numeric role fields existing | Safe additive status/version/role code/app access; tanpa credential |
| CON-04 | ProfileRequest tiga scalar; ProfileResponse hanya profile | Field mask additive dan safe version/changed; full replacement bila omitted tetap |
| CON-05 | Auth password mismatch mapped INVALID_ARGUMENT; common generic authz/dependency mappings tersedia | Generic LOGIN_FAILED, applicable service/session errors dan limiter retry contract; jangan remap legacy global |
| CON-06 | Current pinned module cached, target belum ada di compared files | Generation/tests/published version di repository contract, lalu authorized pin update dan GOWORK=off test/build |

Field numbers/types baru dan catalog codes ditetapkan/dihasilkan di api-contracts; manifest ini tidak membuat protobuf/error registry lokal. Common ErrUnauthorized/ErrForbidden/ErrDependencyUnavailable mempunyai mapping yang bisa di-reuse untuk semantik tepat; tidak mengklaim seluruh security behavior membutuhkan catalog baru.

## Actual schema — task 1.2

[schema-preflight.json](schema-preflight.json) diambil **2026-10-10 06:41:26 UTC**, PostgreSQL **16.13**, database aplikasi lokal `user`, melalui satu transaction RepeatableRead **READ ONLY** yang diverifikasi `transaction_read_only=on` lalu rollback. Hanya metadata public schema dan aggregate count admin dibaca; tidak mengambil email/password/hash/token/profil user. Tidak ada migration, seed, reset atau fixture writes.

Execution: `rtk proxy env GOPROXY=off go run /private/tmp/grpc-auth-session-preflight.go`. Source temporary probe SHA-256 dicatat di report. Percobaan sandbox awal ditolak permission socket; eksekusi diulang dengan escalation yang disetujui untuk read-only preflight, lalu berhasil. Probe tidak menjalankan startup grpc-auth atau logger/database initialization package aplikasi.

| Expected / check | Actual |
| --- | --- |
| Required auth/session/RBAC/persistence model fields | 13 tables, 110 columns; seluruh required fields yang diperiksa tersedia |
| Ownership/uniqueness/status/expiry constraints | 67 constraints, seluruhnya validated; definitions ada di report |
| Existing/new indexes | 38 index definitions tersimpan untuk review, termasuk session user/status/expiry, refresh session/family/hash uniqueness dan RBAC pairs |
| auth password storage | VARCHAR(150), credential default disembunyikan dari report |
| User/role/application IDs, session IDs | INTEGER numeric IDs; sessions/refresh UUID; version/auth_version BIGINT |
| Session version column | Tidak ada; sesuai conceptual terminal revision contract |
| ACTIVE platform administrator | Count **1** dari auth ACTIVE/nondeleted + assignment ACTIVE + system role ACTIVE/numeric code 2 |
| Database mutations | Tidak ada; rollback transaction read-only |

Preflight bukan proof runtime authorization, cross-application business invariant, lock correctness, TTL enforcement atau command atomicity. FK sederhana tidak membuktikan role-permission dalam application yang sama. Actual schema tidak menjadi izin menjalankan fixture tests pada database `user`. Suite PostgreSQL wajib dedicated test DB; opt-in/DSN/database variables belum diset pada sesi ini. Tidak menjalankan suite nyata/default tests karena belum ada perubahan Go.

## Prerequisite ledger — task 1.3

Owner di bawah adalah repository/workstream pemilik deliverable, bukan klaim tugas sudah didelegasikan atau change baru sudah dibuat. Hanya `auth-session-authorization-foundation` aktif pada `openspec list` saat inspeksi.

| Gate | Owner / scope | Required deliverable dan evidence | Actual status |
| --- | --- | --- | --- |
| EXT-01 / SAF-07 | api-contracts, EXTERNAL_REPOSITORY | CON-01..06, safe additive generated fields, catalogs/mappings/locales, tests dan published compatible module; dependent standalone test/build | **PASS**: commit `42a9e6fa23ada9dd939369b9203e1282eacd0f9e` published, grpc-auth pin `v0.0.0-20261010071509-42a9e6fa23ad`; workspace/GOWORK=off test/vet/build dan official make build PASS |
| Producer / SAF-08 | grpc-auth, increment Phase 3 minimum terpisah | Real Login transaction session/refresh/SESSION_CREATED audit, EdDSA issuer/required claims/expiry, basic auth limiter; integration real producer | **BLOCKED**: Login existing token-only HS256, belum ada prerequisite change/deliverable evidence |
| Caller/client / SAF-09 | rest-orc/frontend/deployment, EXTERNAL_REPOSITORY | TLS CA/hostname/service credential, token-empty registration compatibility, trusted IP/cookies/CSRF/single-flight/login ulang evidence | **BLOCKED**: readiness evidence belum diberikan; repository tersebut tidak diubah atau diklaim teruji di sini |
| Schema/admin subset SAF-09 | grpc-auth/database owner, read-only preflight | Actual required schema dan minimal ACTIVE platform admin | **INSPECTED**: fields/constraints/indexes report dan count1; bukan PASS tests/runtime |
| Keys/cutover remainder SAF-09 | grpc-auth/deployment owner, Phase 3 | Actual EdDSA sign/verify/key distribution dan coordinated cutover/secure rollback report | **BLOCKED**: evidence belum tersedia; tidak membaca/mencetak key/secret deployment |
| Protected activation / SAF-10 | grpc-auth | SAF-01..09 mandatory evidence + actual registered RPC integration real Login | **BLOCKED**: producer/caller readiness belum tersedia; contract EXT-01 sudah PASS |
| Management idempotency/full lifecycle / SAF-11 | grpc-auth follow-up Phase 2/3/5 | Full management/refresh/password/revoke/limiter remaining AC evidence | **NOT_STARTED**: bukan implementation increment ini |

Urutan dependency tidak sirkular: independent foundation component → real Login/issuer prerequisite → coordinated guard activation. Producer dapat dibangun tanpa menunggu seluruh Phase 2. Tidak mengaktifkan dummy session/test issuer, HS256 fallback, dual algorithm atau runtime bypass agar gate tampak selesai. RabbitMQ tetap bukan prerequisite increment ini.

## Verification yang dijalankan

Source/AST routing dan direct source inspection; local/pinned module resolution; 13 contract file byte/hash comparisons; read-only schema/constraint/index/admin inspection; report JSON/required-field checks. Hasil ini menutup preflight saja.

Verification final sesi preflight: `rtk proxy openspec validate auth-session-authorization-foundation --strict` PASS; `rtk proxy openspec validate --specs --strict` 4/4 PASS (informational long requirement text existing); `rtk git diff --check` PASS. JSON reports, required-field coverage, local links/anchors/whitespace dan OpenSpec-only diff scope PASS. CLI apply progress terkonfirmasi 3/36 complete.

Tidak ada functional test, server, fixture DB suite, migration, publish, commit atau deployment yang dijalankan pada sesi preflight ini. Required implementation tests dan real producer integration tetap pending.

## Next blocker

Task 4.3 tetap unchecked. Read components sudah menerima verified service/claims, namun handler existing belum dialihkan sampai real Login session producer dan coordinated caller/client/key readiness tersedia. Legacy concurrent Me path belum diganti (task 4.4); race PASS pada suite yang dijalankan tidak mengklaim path tersebut sudah diperbaiki. Rekomendasi: tetapkan prerequisite real Login/EdDSA/basic limiter pada change terpisah, lalu selaraskan consumer wiring/activation task dengan readiness dependency. Task command persistence, dedicated PostgreSQL proof dan final integration readiness tetap pending. Tidak mengarchive change ini.

## Authorized contract prerequisite follow-up — 10 Oktober 2026

User mengotorisasi perbaikan blocker target field/error `api-contracts`. Ini pekerjaan prerequisite sibling terpisah, bukan perluasan implementasi runtime change grpc-auth. Rincian contract/publishing order ada di [auth-session-contract.md](/Users/bleki/Desktop/mini-projects/api-contracts/docs/auth-session-contract.md). [contract-implementation.json](contract-implementation.json) mencatat actual file SHA-256, timestamp, baseline commit serta publication/verification lanjutan; [contract-preflight.json](contract-preflight.json) tetap merupakan snapshot historis sebelum perubahan. Bagian ini mencatat tahap local-ready sebelum publikasi di bawah.

Deliverable lokal: additive registration/session/expiry/restricted fields; safe user/role/application metadata; profile field mask dan version/changed; service/session/generic-login/rate-limit catalog English/Indonesian serta gRPC mappings; standard RetryInfo decoding. Protobuf diregenerasi melalui `make proto`, tanpa mengedit generated file manual. Field/RPC descriptors baseline diambil dari pinned published module menggunakan `GOWORK=off`; compatibility suite membandingkan seluruh field lama dan six-method inventory. Numeric role codes, pagination dan legacy password mismatch mapping dipertahankan.

| Repository / mode | Command (prefix `rtk proxy`) | Result |
| --- | --- | --- |
| api-contracts generation | `make proto` | PASS |
| api-contracts targeted | `env GOPROXY=off go test ./tests/protobuf/... ./tests/grpcerror/... ./tests/dictionary/...` | PASS |
| api-contracts workspace | `env GOPROXY=off go test ./...` | PASS |
| api-contracts standalone | `env GOWORK=off GOPROXY=off go test ./...` | PASS |
| api-contracts standalone | `env GOWORK=off GOPROXY=off go vet ./...` | PASS |
| grpc-auth workspace | `env GOPROXY=off go test ./...` dan `env GOPROXY=off go build ./...` | PASS |
| rest-orc workspace | `env GOPROXY=off go test ./...` dan `env GOPROXY=off go build ./...` | PASS |

Go source telah diformat; api-contracts `git diff --check` PASS. Sandbox compiler-cache permission pada percobaan test awal diselesaikan melalui escalation yang disetujui; rerun PASS. Verifikasi memakai workspace/local cache tanpa service/network/database fixture atau startup. Consumer workspace PASS membuktikan source compatibility dengan target lokal, **bukan** penggunaan target published saat `GOWORK=off`.

Tidak ada commit/push/tag/release, go.mod/go.sum update atau deployment. Perubahan user yang sudah ada (`.DS_Store`, `.graphifyignore`, `graphify-out/`) dipertahankan. Agar EXT-01/task 1.4 PASS: publish revision contract yang diuji, otorisasi pin grpc-auth ke versi tersebut, lalu ulang test/vet/build workspace dan `GOWORK=off` terhadap versi published serta catat actual commit/version/results. Producer Login/EdDSA, readiness caller dan activation tetap gate terpisah.

## Authorized publication dan pin verification — 10 Oktober 2026

User menjawab `lakukan` pada permintaan commit/push contract dan update pin grpc-auth. Hanya 23 file contract yang direview/distage; SHA-256 semuanya cocok dengan manifest local-ready. Commit [42a9e6f](https://github.com/siti-nabila/api-contracts/commit/42a9e6fa23ada9dd939369b9203e1282eacd0f9e) dipush tanpa force ke existing branch `feat/user-err`; `git ls-remote origin refs/heads/feat/user-err` mengonfirmasi full hash yang sama. Perubahan user tetap di working tree api-contracts dan tidak masuk commit. Tidak membuat branch/tag/release/PR.

`rtk proxy env GOWORK=off go get github.com/siti-nabila/api-contracts@42a9e6fa23ada9dd939369b9203e1282eacd0f9e` mengambil published pseudo-version **v0.0.0-20261010071509-42a9e6fa23ad**; hanya pin contract go.mod dan dua checksum go.sum ditambahkan. Tidak memakai replace/local fallback. `GOWORK=off GOPROXY=off go list -m -json` dan `go mod download -json` mengonfirmasi version, module-cache directory, checksum serta remote origin hash. Seluruh **23 file** module hasil download cocok byte/hash dengan deliverable yang direview.

Module checksum: `h1:AHC3W1uS8foXjeGzXwARaZ+x/JIrAuJ1AhjikCHcXAo=`. GoMod checksum: `h1:Jb5bNxnX/aW6YWghT/pXorXlmIDRRxXltY8gL7Udzg4=`. Commit time **2026-10-10 07:15:09 UTC**; verification timestamp ada di manifest.

Verification grpc-auth berikut semuanya **PASS**, menggunakan prefix `rtk proxy`, tanpa server startup/database fixtures:

| Mode | Commands |
| --- | --- |
| Workspace | `env GOPROXY=off go test ./...`; `env GOPROXY=off go vet ./...`; `env GOPROXY=off go build ./...`; `env GOPROXY=off make build` |
| Published pin / standalone | `env GOWORK=off GOPROXY=off go test ./...`; `env GOWORK=off GOPROXY=off go vet ./...`; `env GOWORK=off GOPROXY=off go build ./...`; `env GOWORK=off GOPROXY=off make build` |

EXT-01/task 1.4 **PASS**, total **4/36**. Dependency/evidence changes grpc-auth tetap di working tree; commit/push pada sesi ini hanya api-contracts sesuai otorisasi. Real Login/session/issuer, runtime caller readiness, required opt-in PostgreSQL suites dan protected activation belum dibuktikan oleh verification contract ini.

Final review: strict validation change PASS; strict main specs validation 4/4 PASS; `git diff --check` PASS. `openspec list --json` mengonfirmasi progress 4/36. Consumer dependency diff hanya satu version replacement go.mod dan dua checksum additions go.sum; status api-contracts hanya menyisakan perubahan user yang sudah ada sebelum pekerjaan ini.

## Foundation components apply — 10 Oktober 2026

Sesi ini mengimplementasikan task **2.1–4.2** (10 task tambahan), tanpa perubahan dependency di luar pin yang sudah diotorisasi sebelumnya. [Manifest](foundation-component-verification.json) mencatat HEAD, module versions, safe environment, per-file hashes dan expected/actual verification. HEAD tetap baseline `b89f37270d019c8d0d7823a3ff5d3526874d2232`; implementasi belum dicommit sehingga hasil mengacu working tree hashes.

| Component | Files / verification | Actual scope |
| --- | --- | --- |
| SAF-01 | `pkg/config/service_auth.go`, `internal/serviceauth`, TLS/config/server/interceptor, `tests/configs`, `tests/interceptors`; [service docs](../../../../docs/service-authentication.md) | **COMPONENT PASS**: required TLS 1.3, explicit digest expiry/previous overlap, immutable caller, constant-time comparisons, exact methods/namespaces, localized generic error; six-method in-memory TLS integration, duplicate/spoof metadata rejection, CA/hostname/plaintext rejection, startup cleanup. Tidak membuktikan external readiness. |
| SAF-02 | Auth/session domain, bound session reader/writer, `pkg/jwt/access.go`, `internal/sessions/principal.go`, `AuthenticateSession`; root jwt/feature/repository suites; [session docs](../../../../docs/session-authentication.md) | **COMPONENT PASS**: EdDSA-only required claims, canonical owner/session, issuer/legacy-ID/audience policy, JWT 15m/skew30s/session7d boundaries, primary ACTIVE/version/owner/revocation checks, immutable restricted principal. Real Login/session/refresh producer dan activation belum tersedia. |
| SAF-03 | Permission reader/resolver + `ReadSnapshotProvider`; `tests/features/common`, transaction and auth read integration; [snapshot docs](../../../../docs/authorization-snapshot.md) | **COMPONENT PASS / DB proof PENDING**: active union/source roles, partial revoke, inactive/cross-app/access gates, exact iam.user.read, empty-page denial, no numeric/JWT grant. Tidak mengklaim committed PostgreSQL revocation/interleaving proof. |
| SAF-04 | `ReadUserDataSnapshot`, `ReadUsersSnapshot`, bound page/identity reader; `tests/integration/auth_reads` | **PARTIAL**: candidate protected reads konsisten, self owner dari claims, restricted privilege omission, safe identity projection, preserved page helper, query/commit failure tanpa partial success. Existing RPC handlers/legacy concurrent Me belum diganti, additive role/application response completion dan handler/locale integration pending 4.3–4.4. |
| SAF-05/06/08/09/10 | Command consumers, dedicated PostgreSQL, real producer, caller/client/key readiness | **PENDING/BLOCKED**: tidak dibuktikan oleh deterministic fixtures atau in-memory TLS tests. |

ORM v1.7.0 `QueryPageWithConfig` → `ScanPaginate` menjalankan count dan data sebagai statement berbeda. Untuk mempertahankan existing filters/search/auth_id/last_id, alternatif design digunakan: feature-owned **RepeatableRead READ ONLY** primary transaction, state actor → permissions → count/data atau safe Me response pada handle sama. Mutation Begin tetap ReadCommitted. SQL driver assertions memverifikasi isolation/read-only/context/owner/query order, lifecycle dan absence of pool fallback. Ini bukan PostgreSQL MVCC proof.

Final commands (prefix `rtk proxy`), semuanya PASS:

| Mode | Commands |
| --- | --- |
| Workspace | `env GOPROXY=off go test ./...`; `env GOPROXY=off go vet ./...`; `env GOPROXY=off go test -race ./...`; `env GOPROXY=off make build` |
| Published standalone | `env GOWORK=off GOPROXY=off go test ./...`; `env GOWORK=off GOPROXY=off go vet ./...`; `env GOWORK=off GOPROXY=off make build` |
| Spec/review | `openspec validate auth-session-authorization-foundation --strict`; `openspec validate --specs --strict` (4/4); `git diff --check`; changed Go gofmt check; local link/status review |

Targeted suites dijalankan setelah setiap group. Regression assertions sempat menemukan dependency marker yang hilang melalui legacy HandleDBError; snapshot path kini mempertahankan cause dan UNAVAILABLE, membedakan known query validation sebagai BAD_REQUEST. Vet menemukan protobuf return-by-value menyalin mutex; komponen Me kini mengembalikan pointer dan nil pada failure. Seluruh final verification di atas diulang setelah perbaikan dan PASS.

Test default tanpa DB/network service: caller integration memakai TLS bufconn, repository/feature read integration memakai controlled per-test SQL connector. Tidak menjalankan real Login producer, opt-in PostgreSQL suite atau baseline database fixtures. Binary lokal hasil make build diabaikan Git. Perubahan lama sync/archive, dependency pin dan perubahan user tetap dipertahankan. Task 8 tidak dicentang sebagai final integration complete karena mandatory production/DB/readiness proof masih terbuka.

## Consumer components resume — 10 Oktober 2026

Task 4.3–6.4 menghubungkan prepared production handlers dan persistence consumers
sesuai independent-component sequence design section 2. Final default-server
registration hanya beralih sekaligus di task 7.4. Tidak ada runtime toggle,
HS256 fallback atau independently registrable partial service.

| Acceptance | Current implementation and actual proof | Boundary |
| --- | --- | --- |
| SAF-03/04 | `SessionReadHandler` reuses existing list mapping; principal claims → fresh primary snapshot. Tests cover permission/restricted/empty-page/owner spoof, dependency/Indonesian locale, numeric roles/filter DISTINCT/auth_id/cursor and unchanged TesRPC. Legacy shared-err goroutine aggregation removed. Actual PostgreSQL two-backend barriers prove Me/list authorization+data snapshot consistency and next-request role/app/permission visibility without session/auth_version changes. | COMPONENT PASS; default server cutover remains 7.4; management RPCs/expanded outputs follow-up |
| SAF-05 | `PendingRegistrationHandler` + bound account/profile/member/audit/outbox: explicit PENDING, initial version/auth_version, no token/session/refresh/first-admin. Default member reader rejects inactive/wrong/privileged system member. Profile locks user then session, rechecks current actor/owner/restriction, uses safe mask/full replacement, version+1 once, atomic audit/event; no-op one audit/no writes or event. Feature mocks assert exact handle/order/count/failure cleanup. | COMPONENT PASS; activation/client compatibility required |
| SAF-06 | Real PostgreSQL production commands tested with tentative mutation, required audit/outbox and actual deferred-commit failures; committed version/no-op/pending row counts; current session/auth_version unchanged by Profile. Revocation/status committed before locked recheck deny all profile effects; commit/failure/panic/cancellation retain/release locks and reuse pool. | PASS on explicitly authorized dedicated test database and owned disposable schemas; not migration/deployment certification |
| Failure attempts | `NewSecurityAttemptInterceptor` invokes independent `RecordSecurityAttempt` only after consumer rollback. Actual PostgreSQL verifies rollback precedes audit, service actor/user null, generic bounded record and original dependency/canceled outcome retained despite audit failure. Quota/capacity denial uses limiter aggregate telemetry; Login generic credential denial is not duplicated. | COMPONENT PASS; final observer and pipeline composition owned by 7.4 |
| SAF-08 / task 7.1 | Archived real producer accepted. Fresh deterministic and PostgreSQL suites execute production Login/session/refresh/audit/EdDSA, rollback/sign/commit failure, stale proof, restricted consumers and consumer credential verification. No synthetic producer replaces runtime code. | Producer/consumer component integration PASS; not overall activation |
| SAF-09 / task 7.2–7.3 | Limiter component delivered, EXT-01 published. Private env.yaml safe probe parsed configuration but actual access sign/verify+HMAC, TLS pair and caller validation all false. External rest-orc trusted IP/client retry/token-empty/login-ulang and deployment readiness reports absent. Earlier schema/admin inspection remains historical. | BLOCKED; no fixture keys/admin/IP report counted as external readiness |
| SAF-10/11 | All six existing methods remain the exact policy target. Prepared handlers cannot register partial UserService; no new lifecycle/management RPC. Management idempotency AC-24..26 and full Phase 2/3 remain follow-ups. | Registered-server cutover and final mandatory acceptance BLOCKED |

The fresh [consumer-verification.json](consumer-verification.json) records actual
UTC time, HEAD/module version, working-tree Go/module/fixture hashes, commands,
exit status and safe PostgreSQL backend observations. Historical producer and
foundation reports are preserved. Code is uncommitted; HEAD alone is not the
implementation identity.

Commands use `rtk proxy` and explicit writable GOCACHE. Default tests have no real
DB/network service; TLS tests use in-memory transport. PostgreSQL execution uses
the opt-in bridge reading ignored `env-test.yaml`, explicit dedicated database
`grpc_auth_test`, tag `postgres`, database guard and owned disposable schema
fixtures. No DSN, credential/key content or private value hash appears in reports.
The full PostgreSQL suite is rerun in workspace mode and with `GOWORK=off -race`.
Assertions include physical backend IDs and channel barriers rather than sleeps.
Minimal test search storage does not certify production search indexing/triggers.

Operational handoff: [six-method and mutation matrix](../../../../docs/session-foundation-consumers.md),
[cutover/rollback and required owner evidence](../../../../docs/foundation-activation-readiness.md).
Frontend is an external consuming application, not a required rest-orc folder.
RabbitMQ is not needed to persist required pending outbox events. Audit/outbox
writes are mandatory, while external publication and management replay remain
separate increments. Foundation is not archived while activation is blocked.

Final source verification **2026-10-10 14:10:23 UTC**: workspace/standalone default tests, vet and make build PASS; full workspace race PASS; actual full PostgreSQL suites workspace and standalone with race PASS. PostgreSQL report records 18 backend observations per mode and non-skipped foundation/producer tests. The `postgres_suites_passed` counter in JSON includes nested PASS scenario lines, not only top-level suites. A first verification found a protobuf value-copy vet warning in legacy Me; returning the unchanged public value as a literal fixed it, and final checks were rerun after the repair and adapter safe-result validation. Only final passing results are represented by the source hashes in the fresh report.

Progress **32/36**: tasks 4.3–6.4, 7.1, 7.5 and 8.1–8.3 completed this resume. Remaining: 7.2, 7.3, 7.4, 8.4. Pause is caused by mandatory actual readiness gates, not missing RabbitMQ, test DB or the real Login producer. No foundation archive, deployment, commit or sibling changes performed.

## Local TLS and caller provisioning — 10 Oktober 2026

Actual private configuration revalidation at **2026-10-10 14:39:21 UTC** passes
Ed25519 sign/verify plus HMAC, currently valid TLS pair and caller configuration.
The existing cert.pem was a self-signed CA without SANs. Local setup retains that
CA and creates a separate server certificate with localhost/IPv4/IPv6 loopback
SANs and a new private key; the server certificate expires on 2027-01-08 UTC.
Only TLS paths are updated in private env.yaml. The previously empty rest-orc
current digest/expiry fields are provisioned with a separate random 32-byte
canonical base64url credential; existing method/namespace allowlists are kept.
Credential, keys, local configs and original-config backups remain private.

The local rest-orc .env is created from the existing environment template with
TLS enabled, localhost:9090 target, localhost server name and the existing CA.
Production rest-orc config.Load/tlsconfig.New pass with that configuration. An
in-memory TLS 1.3 handshake with the actual local certificate succeeds; wrong
hostname and untrusted CA fail verification. The generated service credential
passes canonical encoding/length and matches the configured SHA-256 digest.
No credential, key content or private value hash is recorded in the safe
[activation report](activation-readiness.json).

Verification: `rtk proxy env GOCACHE=/tmp/grpc-auth-producer-go-cache GOPROXY=off
go test ./test/client/grpcclient/... ./test/config/...` in rest-orc PASS; read-only
private configuration and TLS/credential probes PASS. All temporary probe Go
files are removed from the repositories. Local env, backups and credential
are confirmed ignored by Git. No service startup or database connection.

This is separately authorized local configuration provisioning. No sibling Go
source or foundation server registration is changed. Source-test evidence in
consumer-verification.json remains the earlier source snapshot. Progress remains
**32/36**. Actual rest-orc x-service-token transport, trusted IP/retry/client
compatibility and deployment/schema/minimal ACTIVE admin readiness still require
owner deliverables. Tasks 7.2, 7.3, 7.4 and 8.4 remain unchecked. Foundation is
not archived.

## grpc-auth-only readiness follow-up — 10 Oktober 2026

User menunda rest-orc dan meminta kelanjutan hanya dalam scope grpc-auth.
[Fresh preflight](grpc-auth-readiness-preflight.json) pada 14:44:14 UTC memeriksa
metadata dan aggregate database runtime lokal dalam RepeatableRead READ ONLY;
`transaction_read_only=on`, outcome rollback, tanpa migration/seed/write.
Observed: 1 ACTIVE canonical iam.platform_admin, 1 unrestricted admin dengan
iam.user.read, 1 safe iam.member tanpa permission, search table/pg_trgm tersedia,
2 noninternal search-related triggers, 0 profile tanpa search row. Seluruh kolom
historical preflight masih ada dengan metadata yang sama. Ini local inspection,
bukan functional trigger test atau deployed-environment certification.

[Actual local caller proof](grpc-auth-caller-verification.json) pada 14:45:51 UTC
menggunakan TLS 1.3 in-memory dengan sertifikat lokal dan production serviceauth
verifier, tanpa handler/database/default-server registration. Credential actual
terverifikasi untuk semua six methods. Missing/duplicate/unknown tokens, non-TLS
peer, unlisted method/namespace, canceled request dan exact expiry boundary
ditolak. Previous tidak dikonfigurasi; deployment rotation readiness tetap open.
Tidak ada raw credential, key content, private-value hash atau user identity di
report. Historical preflight dan source verification report dipertahankan.

Targeted verification PASS: `rtk proxy env
GOCACHE=/tmp/grpc-auth-producer-go-cache GOPROXY=off go test
./tests/configs/... ./tests/interceptors/... ./tests/ratelimit/...`. Temporary
probe Go file di root sudah dihapus. Production Go dan runtime configuration
tidak diubah pada follow-up ini; rest-orc tidak disentuh.

Local evidence diperbarui tanpa mengubah mandatory prerequisite. Progress tetap
32/36: 7.2/7.3 memerlukan EXT-02 dan deployment readiness, 7.4 menunggu keduanya,
8.4 menunggu seluruh mandatory acceptance. Default EdDSA/session registration
belum dialihkan; tidak melakukan archive, deployment, commit atau perubahan DB.

## Complete backend registration — 10 Oktober 2026

User explicitly approved wiring/testing grpc-auth now and preserving external
readiness before deployment. Proposal/design/specs/tasks were reconciled with
that choice; task 9.1 retains EXT-02 and actual deployment readiness. Task 7.2
limiter/local caller and task 7.3 key/TLS/caller/schema/admin evidence are accepted
within their local/backend scope. Existing actual local checks remain in
activation-readiness.json, grpc-auth-caller-verification.json and
grpc-auth-readiness-preflight.json, without private material or hashes.

Task 7.4 switches every existing RPC in source together through
`PrepareFoundation` → `NewGRPCServer`. The fixed chain is Language → Error →
Service → independent failed-attempt Audit → Session → Rate → authoritative
handlers. No legacy JWT fallback, partial service, runtime toggle or synthetic
session exists. Startup validates keys/HMAC/TLS/caller before resources, opens
pools with error-returning cleanup, reuses the primary pool and disables
sensitive ORM debugging. Server shutdown/listener failure cancels and joins the
one-minute limiter worker; its ticker stops on exit. Repeated stops are safe and
outbound clients/pools are closed in reverse initialization order.

| Test boundary | Expected and observed result |
| --- | --- |
| Complete server over controlled in-memory TLS/SQL | Actual Login commits before credentials; normal/restricted Me, ListUsers denied without permission, revoked/missing state and HS256/sid-less rejection; localized dependency error and independent audit after rollback |
| Complete server over owned dedicated PostgreSQL | All six require service auth; TesRPC preserved; Login-produced Me/List/Profile, PENDING Register without credentials, profile mutation/no-op, role removal visible on next empty-page list, restricted denial, revoked session denial |
| Quotas/audit | Canonical account burst enforced, published RetryInfo preserved, three generic Login audits without one audit per quota rejection; bounded attempt failure counter remains zero |
| Lifecycle | Invalid TLS/caller/incomplete pipeline creates no usable server; outbound/listener failures stop resources; controlled barriers prove worker cancellation and join before shutdown returns; repeated stops safe |
| Regression/concurrency | Workspace/standalone default tests, vet and builds PASS; full default race and standalone PostgreSQL race PASS |

Fresh verification on the current uncommitted source: **nine commands PASS** in
[server-verification.json](server-verification.json), with HEAD plus SHA-256 of
all Go sources/modules and the owned schema fixture. The two full opt-in runs
include TestPostgresRegisteredServer and all earlier physical-connection,
snapshot/revocation/locking/atomicity tests. Source identity is distinct from
historical consumer-verification.json. Default tests use controlled boundaries;
actual PostgreSQL requires explicit opt-in and the guarded grpc_auth_test DB.

An initial direct tagged test invocation without opt-in was correctly rejected
by the harness. The dedicated private-config runner then passed the targeted
registered-server suite and both complete tagged suites. No test outcome is
claimed from a skip. The tagged runs create/drop only owned fixture schemas;
no baseline DB write, migration, seed, Docker, generated/sibling/dependency edit,
deployment, commit/push or archive occurs in this implementation.

Backend progress is **36/37**. Task 9.1 remains mandatory for actual consumer
TLS/credential/IP/RetryInfo/PENDING/login-ulang/storage and deployment
rotation/schema/admin readiness. No whole-Phase-1/3 production-completion claim.

Final read-only dedicated-database cleanup verification: PostgreSQL 16.13,
remaining owned fixture schemas **0**. Strict change validation, all seven main
specs, local links, changed-source formatting and git diff --check PASS.

## Deployment gate moved to Phase 6 — 10 Oktober 2026

User instructed moving readiness deployment to the last phase. Phase 6 DEP-01
in docs/grpc-auth-mvp-spec.md now owns the entire former task 9.1: actual consuming
TLS/service credential, trusted IP/proxy, RetryInfo, PENDING/token-empty response,
safe credential storage/login ulang, actual deployment keys/caller rotation/
schema/minimal ACTIVE admin and coordinated release/rollback. The gate is still
unchecked and BLOCKED, mandatory before deployment, not POST_MVP.

Foundation task tracking is **36/36**, after transferring the task rather than
checking it off. Global phase tables, source/deployment cutover wording, OpenSpec
context/rules, proposal/design/specs/tasks and readiness docs were reconciled.
Phase 1–5 backend work retains security/contract/test prerequisites. Existing
server-verification.json is kept as historical execution evidence with its
original counts and source hashes. No new Go/runtime behavior or test result is
claimed by this revision.

Planning verification PASS: strict change validation, seven main specs, local
links/new Phase 6 anchor, AC ID preservation and git diff --check. OpenSpec apply
state is all_done (36/36). Go/module/schema hashes still match the previous
passed source report; application/DB tests were not rerun for documentation.
