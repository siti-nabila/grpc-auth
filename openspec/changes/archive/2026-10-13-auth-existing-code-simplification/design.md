# Design

## Context

Motivasi dan scope ada di [proposal](proposal.md). Keputusan user: seluruh production existing dinilai dan diperbaiki bertahap; legacy yang terbukti tidak digunakan boleh dihapus; fungsi existing boleh diubah; tetap mengikuti [AGENTS.md](../../../../AGENTS.md). Invocation propose hanya membuat artifact, bukan implementasi.

Inspeksi 13 Oktober 2026 menemukan HEAD `2462cb09d0f8e7eef3b17a70456451b8bd2dd6ec`, branch `feat/role-management`, dan increment C beserta archive yang belum committed. Baseline implementasi adalah **working tree saat apply dimulai**, termasuk file untracked C. Published contract yang dipin adalah `v0.0.0-20261012102712-bfb073390ea9`; versi tersebut tidak diubah.

Graph dibangun pada `bf6be877296b8f4191f2e2db5f744b23ab636ed7`, sehingga dipakai sebagai routing, lalu diperiksa terhadap source. Degree hub auth/idempotency/outbox/user-role/transaction domain adalah 236/228/222/217/214. Hub ini sudah memiliki contract/persistence masing-masing; degree tinggi bukan alasan membuat facade baru. AST EXTRACTED menunjukkan `PrepareFoundation` menuju producer, handler dan transaction provider; wiring source saat ini juga mencakup assignment yang belum ada pada graph. Tidak ada root `graph-instruction.md` atau AGENTS lebih dekat pada area yang diperiksa.

Kandidat yang telah dibaca:

| Area | Temuan source | Perubahan yang dituju |
| --- | --- | --- |
| User/role/assignment transactions | Tiga loop discovery/recheck lock hampir sama; replay dan fresh execution memiliki commit terpisah; authorization berulang | Satu algoritma discovery kecil, orchestration per feature dan helper otorisasi existing yang dapat mengembalikan principal |
| Assignment mutation | Compound validation target/role/pair dan persistence version/audit/outbox dalam satu alur | Guard bernama berdasarkan invariant dan tahap persistence lokal; grant/revoke tetap eksplisit |
| Auth/session/profile | ChangePassword, Register, Login, Refresh, admin/self revoke dan list mencampur proof, lock, recheck, persistence dan mapping | Ekstraksi per tahap bisnis, tanpa universal command runner |
| Catalog | Normalize mencampur enam jenis query validation; reader membangun filter/count/page/cursor | Helper normalisasi per tanggung jawab, whitelist dan SQL adapter existing dipertahankan |
| Startup | PrepareFoundation merangkai banyak feature sekaligus | Setup per feature mengikuti prepareLifecycle/prepareRoles existing |
| Legacy | sessionUserServer memakai UserHandler hanya untuk TesRPC; handler auth/profile lama dan token interceptor lama bukan jalur server aktif | Pisahkan TesRPC, hapus consumer lama beserta dependency eksklusifnya setelah reference check |
| User list | SearchUsers masih dipakai ReadUsersSnapshot, tetapi userService/NewUserService memiliki dependency reader/config yang tidak dipakai SearchUsers | Pertahankan query behavior; kecilkan service dan hapus wiring legacy yang eksklusif |

Sejumlah kalimat AGENTS, transaction-ownership dan dokumen baseline menggambarkan register-token/HS256/pre-cutover. Keputusan user sebelumnya dan main specs yang lebih baru menetapkan PENDING/token-empty, EdDSA, session-aware pipeline dan additive profile mask. Refactor mengikuti behavior terbaru tersebut; tidak menghidupkan fallback lama atau menulis ulang archive/spec historis untuk meratakan perbedaannya. Status historis pada spec MVP juga bukan status runtime saat ini.

## Goals / Non-Goals

**Goals:** orchestration terbaca berurutan, fungsi fokus, duplikasi algoritma nyata berkurang, dependency hanya yang digunakan, dan seluruh outcome existing tetap sama. Scope produksi luas tidak berarti setiap file wajib diubah.

**Non-Goals:** feature berikutnya, perubahan wire API/error/schema/runtime configuration, dependency update, package/directory rename massal, transaction framework, reflection, service locator, cache authz atau helper satu baris tanpa nilai semantik. `pkg/jwt` EdDSA, limiter, service-token verifier, database provider, logger dan error encoder yang sudah fokus dipertahankan; hanya consumer/wiring yang terkait kandidat di atas diperbaiki.

## Decisions

### 1. Empat tahap refactor dengan baseline yang aman

Urutan: legacy/dependency cleanup → management/assignment/shared authority → auth/session/profile → query/adapter/wiring. Setiap tahap menjalankan targeted regression dan full default suite sebelum tahap berikutnya. Baseline gagal karena environment atau defect existing dicatat dan dibedakan dari regression; jangan menandai task verifikasi PASS sebelum hasil nyata tersedia.

Pada apply, rekam status, file hashes, contract pin, registration inventory dan daftar scenario baseline dalam evidence change ini. Jangan reset, stash, memindahkan untracked C, membuat commit awal, atau mengandalkan HEAD untuk rollback. Refactor harus mempertahankan perubahan C. Alternatif rewrite sekaligus ditolak karena mengaburkan sumber regression.

### 2. Legacy dihapus berdasarkan consumer, bukan nama file

Pindahkan implementasi TesRPC tanpa mengubah response ke adapter kecil yang digunakan sessionUserServer. Setelah itu hapus method/constructor lama Register/Login/Me/ListUsers pada UserHandler, ProfileHandler lama, authService token-only, profileService lama dan TokenInterceptor lama bila seluruh reference production membuktikan tidak ada consumer aktif.

Telusuri dependency eksklusif legacy sampai constructor, domain dan mock; hapus hanya leaf yang kehilangan semua consumer yang sah. Tetap pertahankan DefaultArgon2Config, ReadUserDataSnapshot, SearchUsers, bound profile/auth readers/writers, session-aware handlers dan komponen lain yang shared. Hapus field auth/profile/role/user-role/config pada userService yang tidak digunakan SearchUsers; service list aktif hanya memerlukan context dan UserReader.

Test yang hanya menguji jalur lama bukan alasan mempertahankan runtime duplikat. Sebelum menghapusnya, pastikan rollback/panic/ownership/normalization/error-redaction yang masih relevan diuji melalui jalur aktif. Test aktif tidak boleh dihapus untuk membuat suite hijau; legacy collocated tests tidak dipindah massal. Exported API `pkg/` yang pemakai eksternalnya belum dapat dibuktikan tetap dipertahankan; penghapusan API module publik bukan bagian refactor ini. Loader/config key existing tetap kompatibel.

Alternatif mempertahankan seluruh legacy ditolak sesuai keputusan user, tetapi dead-code cleanup tetap terbatas pada hasil reference check yang dapat direview.

### 3. Discovery lock dibagi, transaksi tetap eksplisit per feature

Tambahkan helper feature kecil `LockDiscoveredAuthority(owner, held, discover, busy) (restart []string, err error)` di package common. `discover` adalah callback tanpa argumen yang membaca repositories **bound ke transaction pemanggil**. Helper hanya menjalankan algoritma yang sudah sama pada user-management, role-management dan assignment:

1. Canonical held keys; maksimal tiga putaran discovery pada transaksi yang sama.
2. Baca discovered keys, cari key yang belum dipegang, dan deteksi predecessor melalui RequiresLockRestart.
3. Bila predecessor ditemukan, kembalikan enlarged restart set tanpa mengambil predecessor atau menulis target.
4. Acquire missing keys dengan TryLock owner, canonicalize held, lalu baca ulang discovery.
5. Bila stabil, selesai; bila belum stabil setelah putaran ketiga, kembalikan enlarged restart set.

Feature memegang loop luar maksimal tiga **transaksi** seperti existing. Restart mengakhiri transaksi lama melalui deferred Finish sebelum Begin baru. Helper tidak melakukan Begin/Commit/Rollback, Bind, permission check, mutation, audit atau idempotency. Lock error mempertahankan busy catalog pemanggil dan dependency/cancellation cause. Jangan menerapkan discovery/retry baru pada session admin atau consumer lain yang protocol existing-nya berbeda.

Initial locks dan discovery callback tetap milik feature. Urutan global tetap idem → last-admin → policy → account/role-code → user → role → application → session; numeric IDs canonical dan deduplicated. Last-admin guard digunakan bersama RevokeUserRoles dan ChangeUserStatus, disertai locked current recount. Existing ORM PostgreSQL TryLock memakai pg_try_advisory_xact_lock pada handle yang sama dan retained sampai transaction selesai; unique/FK constraints tetap diperlukan.

`TransactionOwner.Finish` harus tetap di-defer langsung dari fungsi owner karena recover bergantung pada call site tersebut. Jangan membungkus recover/defer dalam helper generik. Commit error tidak memicu retry mutation dan outcome unknown tidak diklaim pasti rollback. Repository tetap read/write saja, tanpa pool fallback, independent transaction, network call atau concurrent write pada shared adapter.

Alternatif generic transaction runner dengan callback seluruh pipeline ditolak karena menyembunyikan boundary keamanan dan exception refresh.

### 4. Authority, command execution dan persistence disederhanakan secara lokal

Ubah helper internal AuthorizeCurrentPermission agar mengembalikan `(sessions.Principal, error)` dari AuthenticateSession → AuthorizeMethod → Permissions → ResolvePermissions/HasPermission. Sesuaikan seluruh pemanggil existing. User/assignment/admin paths yang sekarang memperbarui principal menggunakan hasil tersebut; role/catalog paths yang sekarang hanya memerlukan keputusan tetap dapat mengabaikan principal agar tidak menambah semantics baru. Read Me dan self-service tidak diberi permission admin tambahan.

Pada setiap command, tahap luar tetap mempersiapkan validation, principal awal, scope/fingerprint dan lock set. Setelah lock stabil: current authority → retained-result lookup/replay → fresh command bila tidak replay → Save bila key berlaku → commit. Ekstrak lookup/replay dan fresh execution sebagai fungsi lokal yang bermakna sehingga owner dapat memiliki satu commit sukses, dengan validasi safe-result dan error mapping masing-masing feature tetap sama. Jangan menyatukan perbedaan optional key, password fingerprint atau role conflict catalog melalui flag mode.

Assignment batch memisahkan validasi target/version, kelengkapan requested roles, dan konsistensi pair/role/application; urutan check/error tetap existing. Grant memvalidasi seluruh eligibility sebelum write. Revoke mempertahankan self-admin/last-admin checks bahkan pada nominal no-op. Persistence assignment dipisahkan menjadi advance-version dan evidence/outbox stages lokal; version naik sekali untuk batch efektif. Grant/revoke loops tetap terpisah karena eligibility dan mutation berbeda. Pengurangan jumlah baris bukan alasan membuat universal activate/revoke callback.

### 5. Auth/session/profile dipecah berdasarkan tahap bisnis

Register: normalize/validate/hash sebelum lock → locked duplicate/default-member check → create account/profile/member → audit/outbox → commit. Login: input/password proof → ordered locks/current proof → session/refresh/audit persistence → issue credential → commit. Ekstraksi mempertahankan lokasi expensive hashing dan urutan issue/write existing; tidak menambah reads, retry atau idempotency baru.

ChangePassword: pre-lock credential verification/hash → locked current identity/version proof → password/counter mutation → sessions/refresh invalidation → evidence → commit. Profile: selected fields → owner/session recheck → replace/no-op → version/audit/event → commit. Helper yang sudah fokus seperti preparePassword, persistPassword, selectedFields dan persistUpdate digunakan kembali.

Refresh: pisahkan identity/version checks, lifetime checks dan token-state checks dalam validateRefreshProof dengan guard yang jelas, tetapi pertahankan evaluasi/denial order. Token proof dikonfirmasi kembali setelah lock. Rotation tetap insert successor sebelum conditional consumption dan audit/signing sebelum commit; response credential hanya setelah commit. Reuse tetap branch eksplisit revoke family/session → required audit/outbox → **commit → ErrRefreshTokenReused**. Tidak dipaksa menjadi commit tunggal dengan branch sukses biasa.

Session admin/self/global: pecah authority/target proof, fresh execution dan persistence lokal; jangan menyamakan single-session no-op dengan global credential invalidation. Session list memisahkan query normalization dan result mapping, mempertahankan snapshot dan ownership checks. Clock/entropy tetap injected; jumlah/lokasi sampling waktu yang memengaruhi expiry, fingerprint atau timestamps tidak digeser tanpa characterization evidence.

### 6. Query dan composition mempertahankan bentuk existing

Catalog.Normalize menggunakan helper lokal untuk pagination, projection, sort, filters dan search, dalam urutan validasi existing, kemudian cursor validation. Clone input slices/search pointer seperti baseline agar request tidak dimutasi. Jangan membuat query-options object baru atau menggabungkan ListUsers full-text dengan catalog literal search.

Reader catalog tetap memakai readCatalog generic existing dan typed row projection. Pisahkan count, page construction dan page metadata/cursor bila meningkatkan keterbacaan; whitelist domain dan SQL-expression mapping tetap berada di layer masing-masing. Tidak mengubah joins, parameters, DISTINCT totals, collation, keyset null rules, offset fallback atau query count. Cursor existing tetap dapat digunakan pada query yang sama sesudah refactor, termasuk hidden sort values dan target binding assignment.

Handler hanya parse/validate/map; business decision tetap feature. Existing protobuf DTO tidak masuk helper business baru. PrepareFoundation mendelegasikan setup user-management/assignment/profile-registration yang jelas seperti prepareLifecycle/prepareRoles, sambil mempertahankan constructor fail-fast, satu primary pool, synchronized entropy dan registration/interceptor order. Tidak menambah factory registry, dependency container atau resource baru.

### 7. Compatibility inventory: tiga puluh full methods

Semua baris memerlukan TLS dan verified service sesuai full-method allowlist; namespace checks yang sudah ada tetap dilakukan pada boundary yang sama. `U` berarti active, nonrestricted verified user/session; self berarti owner dari proof, bukan payload. Restricted hanya Me minimum, ChangePassword, Logout. `R` adalah persistent refresh proof; bukan access-JWT fallback. Tabel ini memetakan runtime existing, bukan membuka method target dari spec MVP.

| Full method | Authority/credential selain service |
| --- | --- |
| /user.v1.UserService/Register | Tanpa user credential; PENDING/token-empty |
| /user.v1.UserService/Login | Password proof/current ACTIVE account |
| /user.v1.UserService/TesRPC | Tanpa user credential; response existing |
| /user.v1.UserService/Me | Verified self; restricted minimum boleh |
| /user.v1.UserService/ListUsers | U + iam.user.read |
| /user.v1.UserService/GetUser | U + iam.user.read |
| /user.v1.UserService/CreateUser | U + iam.user.create |
| /user.v1.UserService/UpdateUser | U + iam.user.update |
| /user.v1.UserService/ChangeUserStatus | U + iam.user.status.update |
| /user.v1.UserService/ListUserRoles | U + iam.user.read |
| /user.v1.UserService/GrantUserRoles | U + iam.user.role.grant |
| /user.v1.UserService/RevokeUserRoles | U + iam.user.role.revoke |
| /profile.v1.ProfileService/UpdateProfile | U + self |
| /auth.v1.AuthService/RefreshSession | R; valid ACTIVE unrestricted account; reject authorization metadata |
| /auth.v1.AuthService/Logout | Tepat satu valid access/refresh mode; self; restricted boleh |
| /auth.v1.AuthService/ChangePassword | Verified self/current password; restricted boleh |
| /session.v1.SessionService/ListMySessions | U + self |
| /session.v1.SessionService/RevokeMySession | U + target-session ownership |
| /session.v1.SessionService/RevokeAllMySessions | U + self |
| /session.v1.SessionService/ListUserSessions | U + iam.session.read |
| /session.v1.SessionService/RevokeUserSession | U + iam.session.revoke |
| /session.v1.SessionService/RevokeAllUserSessions | U + iam.session.revoke |
| /role.v1.RoleService/ListRoles | U + iam.role.read |
| /role.v1.RoleService/GetRole | U + iam.role.read |
| /role.v1.RoleService/CreateRole | U + iam.role.create |
| /role.v1.RoleService/UpdateRole | U + iam.role.update |
| /role.v1.RoleService/ChangeRoleStatus | U + iam.role.update |
| /role.v1.RoleService/ReplaceRolePermissions | U + iam.role.permission.update |
| /permission.v1.PermissionService/ListPermissions | U + iam.permission.read |
| /application.v1.ApplicationService/ListApplications | U + iam.permission.read |

Preserve service identity/credential digest/expiry and namespace policy, x-service-token versus authorization separation, locale, request/trace metadata, deadlines, cancellation and error redaction. Interceptor order tetap Language → Error → Service → Attempt → Session → Rate. Dependency failure tetap UNAVAILABLE, bukan empty page, permission denial atau allowed result. JWT tetap EdDSA-only, issuer ApplicationName tepat, audience ignored, access maksimal15 menit dibatasi session expiry, session/refresh absolute7 hari, JWT skew30 detik tanpa memperpanjang sesi. Tidak ada deploy cutover baru.

### 8. Persistence, counters dan idempotency tetap per command

Tabel berikut adalah ringkasan regression; details no-op/validation/event envelope tetap main specs dan source yang dirujuk proposal. Semua required persistence memakai satu handle/outcome.

| Command | Mutation/counters | Required audit/outbox pada effective change | No-op/replay |
| --- | --- | --- | --- |
| Register | Account PENDING v1/auth_v1 + profile + member | USER_REGISTERED + user.created | Duplicate tetap error; tanpa general replay/credential |
| Login | Session + refresh digest; user counters tetap | SESSION_CREATED; tidak menambah uncatalogued event | Tanpa general replay |
| UpdateProfile, UpdateUser | Profile fields; user version+1, auth_version tetap | USER_PROFILE_UPDATED + user.profile_updated | Fresh no-op audit, tanpa event/version; UpdateUser key opsional |
| CreateUser | Account/profile/member v1/auth_v1, must_change_password=true | USER_CREATED + user.created | Key wajib, replay safe result saja |
| ChangeUserStatus | Entity version+1; suspend/deactivate auth_version+1 dan revoke sessions/refresh; reactivate tidak memulihkan sesi | USER_STATUS_CHANGED + user.status_changed dengan compound sessions_revoked | Fresh no-op tanpa version/event; key wajib |
| GrantUserRoles, RevokeUserRoles | Requested pair changes, sekali user version+1; auth_version/session/app-access tetap | USER_ROLES_GRANTED/REVOKED + tepat satu batch user.roles_granted/revoked | Fresh full no-op satu NO_OP audit; key wajib; replay tanpa persistence |
| CreateRole, UpdateRole, ChangeRoleStatus | Role version1/+1, credentials user tetap | ROLE_CREATED atau ROLE_UPDATED + role.created/updated; status memakai reason ROLE_STATUS_CHANGED | Fresh no-op tanpa event/version; create/status key wajib, update opsional |
| ReplaceRolePermissions | Final mappings + sekali role version+1 | ROLE_PERMISSIONS_CHANGED + role.permissions_changed | Fresh no-op audit saja; key wajib |
| Refresh rotation | Successor insert + predecessor consumed/replaced; counters tetap | SESSION_REFRESH_ROTATED; tanpa rotation event | Tanpa grace/replay credential |
| Refresh reuse | Terminal session/family revoke; counters user tetap | REFRESH_REUSE_DETECTED dan effective SESSION_REVOKED + session.revoked revision2 | Repeated reuse tanpa duplicate terminal event; commit sebelum denial |
| Logout, RevokeMySession, RevokeUserSession | Terminal satu session + refresh; counters user tetap | SESSION_REVOKED + session.revoked revision2 | Proven terminal repeat no-op sesuai policy; hanya admin single punya key wajib |
| RevokeAllMySessions | User version/auth_version+1; seluruh session/refresh revoke; proven current session harus ikut tercabut | USER_SESSIONS_REVOKED + user.sessions_revoked pada user version | Count<1 tetap invalid-session/rollback; tanpa general key |
| RevokeAllUserSessions | User version/auth_version+1; seluruh session/refresh revoke | USER_SESSIONS_REVOKED + user.sessions_revoked pada user version | Global invalidation efektif meski count0; key wajib, replay tanpa persistence |
| ChangePassword | Password + user version/auth_version+1; seluruh session/refresh revoke | PASSWORD_CHANGED + USER_SESSIONS_REVOKED + user.sessions_revoked | Login ulang, tanpa general replay |

Safe idempotency retention tetap24 jam; actor + full method + canonical target + key. Target creation account/{normalized_email} atau role-code/{code}; command lain user/{id}, role/{id}, user/{id}/session/{uuid} atau user/{id}/sessions. Fingerprint mempertahankan normalized fields, expected entity version, presence dan sorted role/permission sets; trace/deadline dikecualikan. Password CreateUser memakai keyed comparison existing, tanpa raw password/auth hash di result/audit/event. Replay perlu current authority dahulu, lalu mendahului expected-target-version; fresh stale no-op tetap conflict. Perbedaan payload pada scope sama conflict, target berbeda independen, expiry diproses atomik sebagai command baru. Response-lost memakai safe original result bila command memiliki key; rotation memerlukan login ulang, tidak retry credential issuance otomatis.

Grant max20 submitted role IDs, all-or-nothing; unknown role menggagalkan batch. Revoke registered-never-assigned pair no-op, tidak insert. Pair reactivation mempertahankan identity. Self admin revoke tetap denied; guard/count last-admin tidak dihapus. Role code/application immutable dan system-role metadata tetap protected. Required audit/outbox/idempotency failure membatalkan precommit business writes; separate safe failure audit berjalan setelah cleanup mengikuti interceptor policy dan tidak mengganti original error bila audit itu gagal.

Limiter tidak diubah: Login trusted-IP30/min burst10 dan HMAC account5/min burst3; Register IP5/min burst2; Refresh IP60/min burst10 dan verified-session10/min burst2; protected-read120/min burst20; admin-mutation30/min burst5; ChangePassword5/min burst2. Atomic layered admission, idleTTL15 menit, cleanup1 menit, cap10.000, structured RetryInfo dan trusted rest-orc client-IP/fallback service identity tetap. Tidak membuat method-policy registry baru untuk menyatukan authentication, permission dan quota yang memiliki boundary berbeda.

## Risks / Trade-offs

- **Penghapusan legacy membuang safety coverage** → petakan scenario ke consumer aktif sebelum menghapus test; pertahankan historical archive dan exported pkg compatibility yang belum terbukti aman dihapus.
- **Helper discovery mengubah ordering atau retry** → test strict call order, predecessor restart, unstable exhaustion, same-handle use dan real PostgreSQL dua koneksi; helper hanya digunakan tiga consumer dengan algoritma sama.
- **Ekstraksi mengubah panic/cleanup/error precedence** → defer Finish langsung, preserve named error handling dan error cause; test failures pada setiap tahap kritis, canceled context, panic dan commit uncertainty.
- **Helper berlebihan menambah indirection** → setiap ekstraksi memiliki satu tahap bisnis/query yang jelas atau reuse konkret; hindari wrapper satu baris, option bags, callback persistence generik dan threshold LOC arbitrer.
- **Cursor/SQL terlihat ekuivalen tetapi berubah hasil** → uji backward cursor fixtures, setiap scalar ASC/DESC, ties/nulls/projection, literal search, unique totals dan snapshot di PostgreSQL.
- **Baseline C belum committed** → catat file hashes/status sebelum apply, review hunks terhadap working-tree baseline dan pertahankan hunks user; evidence lama tidak diklaim sebagai verification refactor.

## Migration Plan

Tidak ada data migration, seed, config migration, key rotation atau perubahan consumer. Apply hanya setelah review artifact dan request implementasi terpisah. Deploy mengikuti prosedur existing sesudah gates refactor; actual deployment readiness DEP-01 tetap wajib dan tidak dianggap selesai oleh refactor ini.

Rollback implementation dilakukan dengan membatalkan hanya hunks change ini berdasarkan recorded baseline, mempertahankan C dan perubahan user. Bila sudah dirilis, kembali ke artifact baseline terbaru yang masih session-aware/EdDSA dan schema-compatible; tidak kembali ke jalur token-only/HS256. Tidak menjalankan deployment atau rollback deployment sebagai bagian apply tanpa scope eksplisit.

## Acceptance dan Verification

Semua criterion berikut telah diverifikasi **PASS** saat apply pada 13 Oktober 2026; hasil command dan review tersedia pada [verification evidence](evidence/verification.md) dan [coverage mapping](evidence/coverage.md). Owner backend/evidence adalah grpc-auth; baseline adalah working tree C dan published contract existing. Detail task berada di [tasks](tasks.md). DEP-01 tetap prerequisite deployment terpisah.

| ID | Criterion dan test/evidence | Traceability existing |
| --- | --- | --- |
| ECS-01 | Legacy removal map, remaining consumer references, lean dependency structs; preserved active scenarios dan exact30 registered methods | M-01/05/06; transaction-ownership, service-authentication |
| ECS-02 | Shared discovery algorithm, bounds3x3, canonical keys, predecessor rollback, actor authority before target/replay; strict mock ordering dan real DB | AC-19/20/24..26/31; M-08/10/11 |
| ECS-03 | User/role/assignment success, invalid batch, system roles, stale version, no-op/replay, lost response, atomic failures, pair identity dan last-admin races | AC-04..07/09/10/19..26/35; M-05/07..11 |
| ECS-04 | Pending Register, generic Login denial, restricted session, profile presence/ownership, password/global invalidation, rotation/reuse commit-on-denial dan session expiry | AC-01/02/14..18/33/34; M-01..04/07 |
| ECS-05 | Catalog/assignment/user-list query behavior, unique totals, safe projection, old cursors, null/tie keysets, auth/data same snapshot dan empty/error distinction | AC-03/21/23/31; M-05/06/10/11 |
| ECS-06 | Startup fail-fast/resource cleanup, unchanged interceptor chain/quotas/errors/locale, full workspace/standalone gates dan dedicated PG normal/race | AC-24..26/31; M-01..11; DEP-01 remains separate |

Saat apply, tambahkan hanya gap characterization tests di root tests/ dengan testutils.Run dan scenario/mocks pattern existing; default suite deterministic tanpa external DB/network. Targeted packages: tests/features/common, auth_feature, profile, user, role, catalog, session; tests/handler, interceptors, configs, repositories dan integration/auth_reads sesuai tahap. Pertahankan legacy collocated tests yang masih relevan tanpa memindahkannya.

Final mandatory gates pada parent workspace dan `GOWORK=off`: `go test ./...`, `go vet ./...`, `go test -race ./...`, `make build`; gunakan RTK dan writable GOCACHE bila diperlukan. `make build` hanya menghasilkan ignored binary, tidak menjalankan server. Opt-in `go test -tags postgres -count=1 ./tests/integration/postgres/...` dan versi `-race` juga dijalankan pada kedua mode melalui test configuration existing. Load root env-test.yaml secara privat ke GRPC_AUTH_POSTGRES_TEST_ENABLED=1, GRPC_AUTH_POSTGRES_TEST_DSN dan GRPC_AUTH_POSTGRES_TEST_DATABASE; jangan print secret, memakai env.yaml fallback, application DB, Docker atau rerun application migration. Suite memakai dedicated isolated fixture/harness existing, dua physical connections dan barriers; unavailable gate dicatat BLOCKED, bukan skip/PASS.

Evidence implementation mencatat baseline/final source identity, command/mode/timestamp/exit status, scenario outcome, test retirement mapping dan diff review. Jumlah test dapat berubah karena legacy removal tetapi cakupan aktif ECS-01..06 tidak boleh menurun. Readability review menunjukkan orchestration yang terurut, validation guards yang bermakna, satu discovery algorithm dan dependency yang digunakan; tidak memerlukan dependency analyzer baru atau mengejar persentase pengurangan LOC. Planning/source inspection hanya evidence perencanaan, bukan backend PASS.
