# command-persistence Specification

## Purpose

Menyediakan persistence audit, pending event, dan hasil idempotency yang aman serta atomik untuk command IAM berikutnya, tanpa menganggap adanya tabel atau repository sebagai bukti command bisnis telah berjalan.

## Requirements

### Requirement: Required persistence shares the mutation transaction

Repository persistence SHALL memungkinkan satu consumer menyimpan mutation, audit, required outbox, dan safe idempotency result dalam satu transaction commit. Persistence MUST tidak melakukan independent commit, memakai fallback pool read/write setelah mutation lock, atau memanggil broker/external service dalam transaksi.

#### Scenario: All command writes succeed

- **WHEN** consumer terverifikasi menulis mutation, audit, event required, dan safe result lalu commit berhasil
- **THEN** seluruh row terlihat bersama dengan transaction outcome yang sama

#### Scenario: Any required write fails

- **WHEN** audit, required outbox, atau idempotency result gagal setelah mutation ditulis
- **THEN** seluruh write transaksi rollback dan tidak ada hasil sukses yang dapat di-replay

#### Scenario: Production mutation fails at each required write

- **WHEN** an authorized valid production command from the 21-method MVP mutation matrix fails at its domain write, required audit, required pending event or applicable idempotency-result write before commit
- **THEN** the whole transaction is rolled back, no success response or replayable success is returned, and no partial domain/audit/event/result state becomes committed

#### Scenario: Cancellation or panic interrupts a tentative command

- **WHEN** a request is canceled, its deadline expires or a panic occurs after tentative mutation but before commit
- **THEN** resources are cleaned up before the RPC completes or an independent failed attempt is recorded, with the corresponding safe CANCELED, DEADLINE_EXCEEDED or INTERNAL outcome and no partial pre-commit success

#### Scenario: Commit acknowledgement fails

- **WHEN** database commit reports a non-context failure, including an acknowledgement loss whose persistence outcome cannot be established
- **THEN** the RPC returns a safe dependency failure rather than success, retains the failure cause internally, does not automatically rerun the mutation and does not claim that an unknown outcome proves rollback

### Requirement: Audit stores verified actor and safe data

Audit SHALL berisi actor terverifikasi, action, target, safe before/after, reason bila relevan, occurred_at, serta correlation metadata yang dibatasi. Credential/password/hash/token/header auth MUST tidak tersimpan. Outcome SHALL memakai action/safe snapshot existing; persistence MUST tidak mengasumsikan outcome column baru.

#### Scenario: Safe audit is committed

- **WHEN** consumer menyediakan actor user/service yang telah diverifikasi beserta snapshot allowlist
- **THEN** audit menyimpan metadata bisnis aman dengan owner/correlation yang benar dan tanpa credential

#### Scenario: Untrusted or sensitive audit input is supplied

- **WHEN** actor tidak tersedia atau snapshot berisi credential di luar safe contract
- **THEN** audit write ditolak sebelum commit dan consumer tidak boleh mengganti actor dengan identitas browser/payload atau secret-redaction yang menyamarkan data tersimpan

### Requirement: Pending outbox preserves the MVP event contract

Outbox SHALL menyimpan envelope catalog MVP: event_id, event_type, schema_version, occurred_at, producer, aggregate_type/id/version, actor_user_id bila ada, correlation_id, causation_id, traceparent bila tersedia, dan data aman. Event SHALL tetap pending tanpa broker; published_at MUST tidak diisi tanpa publication confirmation. Tidak ada event baru untuk mengisi version gap.

#### Scenario: Event is stored without a broker

- **WHEN** consumer commit dengan required event dan RabbitMQ tidak tersedia atau relay belum dibuat
- **THEN** mutation/audit/event tetap committed, published_at kosong, dan event belum dinyatakan delivered

#### Scenario: Batch or compound event is prepared

- **WHEN** consumer menyediakan event batch role atau compound application access sesuai catalog
- **THEN** pending payload mempertahankan changed_role_ids dan entity aggregate_version committed tanpa menggandakan event per role pada versi user sama

#### Scenario: A terminal session event is prepared by a future consumer

- **WHEN** consumer terverifikasi mencabut satu sesi ACTIVE menjadi REVOKED
- **THEN** event iam.session.revoked.v1 memakai lifecycle aggregate_version 2 satu kali; persistence tidak membutuhkan session version column atau menjalankan revocation sendiri

#### Scenario: Login and normal refresh do not invent creation events

- **WHEN** `/user.v1.UserService/Login` or normal `/auth.v1.AuthService/RefreshSession` rotation commits its valid session/token state and success audit
- **THEN** no session-created or refresh-rotated domain event is fabricated and credentials appear only in the committed Login/Refresh response

#### Scenario: A single session reaches its terminal state once

- **WHEN** `/auth.v1.AuthService/Logout`, `/session.v1.SessionService/RevokeMySession`, `/session.v1.SessionService/RevokeUserSession` or proven refresh reuse changes one ACTIVE session to REVOKED
- **THEN** exactly one required iam.session.revoked.v1 uses the session UUID aggregate and lifecycle revision 2, without resurrecting terminal state or duplicating an event on replay

#### Scenario: Password and global revocation use one user aggregate event

- **WHEN** `/auth.v1.AuthService/ChangePassword`, `/session.v1.SessionService/RevokeAllMySessions` or `/session.v1.SessionService/RevokeAllUserSessions` effectively invalidates the user sessions
- **THEN** exactly one iam.user.sessions_revoked.v1 uses the committed user aggregate version, PASSWORD_CHANGED is retained when applicable and no per-session event fan-out is added

#### Scenario: Status mutation carries its existing revocation signal

- **WHEN** `/user.v1.UserService/ChangeUserStatus` commits a valid suspend or deactivate transition
- **THEN** its required iam.user.status_changed.v1 carries sessions_revoked=true atomically with status/auth-version/session invalidation and does not add duplicate session events; later activation does not restore old sessions

#### Scenario: Role assignment and application access remain compound commands

- **WHEN** `/user.v1.UserService/GrantUserRoles`, `/user.v1.UserService/RevokeUserRoles`, `/application.v1.ApplicationService/GrantApplicationAccess` or `/application.v1.ApplicationService/RevokeApplicationAccess` commits an effective permitted batch/compound change
- **THEN** one corresponding catalog event contains the safe changed_role_ids at the committed user version, a batch remains all-or-nothing with maximum 20 roles, and no extra per-role event or auth_version/session invalidation is introduced

### Requirement: Idempotency scope includes actor method target and key

Idempotency SHALL memakai actor user terverifikasi, full RPC method, canonical target, dan metadata key sebagai unique scope. Key SHALL nonempty, maksimal 200 karakter; hash input SHALL mempertimbangkan normalized payload, target, presence, expected entity version, dan set canonical, tanpa trace/deadline. Caller service identity dicatat tetapi MUST tidak menjadi unique dimension tambahan.

#### Scenario: Same scoped key is used for the same request

- **WHEN** consumer menemukan committed result dengan actor/method/target/key dan fingerprint sama, serta current identity/endpoint permission sudah diverifikasi consumer
- **THEN** hasil aman pertama dikembalikan sebelum pemeriksaan target expected version terbaru, tanpa mutation, audit, atau event tambahan

#### Scenario: Same scoped key has a different request

- **WHEN** scope/key sama tetapi fingerprint payload/presence/expected version berbeda
- **THEN** consumer menerima IDEMPOTENCY_CONFLICT dengan target mapping ABORTED tanpa mutation

#### Scenario: Same key is used on a different target

- **WHEN** actor/method/key sama dipakai untuk canonical target berbeda
- **THEN** scope berbeda dan hasil target pertama tidak di-replay untuk target kedua

#### Scenario: Key is invalid

- **WHEN** key kosong/whitespace atau melebihi 200 karakter
- **THEN** request contract invalid tanpa persistent success record

### Requirement: Safe replay lasts twenty four hours

Safe idempotency result SHALL disimpan 24 jam setelah creation/commit dan unexpired result MUST tidak dibuang untuk menghemat storage. Expired scope SHALL diperlakukan sebagai request baru hanya dengan exclusion dan transaction yang sama. Persistence MUST tidak menyimpan raw password, authentication hash, refresh/access token, atau credential response untuk replay.

#### Scenario: Response is lost after commit

- **WHEN** consumer menyelesaikan management mutation tetapi response hilang dan request identik dikirim ulang selama window
- **THEN** satu committed result di-replay tanpa menjalankan efek domain lagi

#### Scenario: Retention expires

- **WHEN** waktu terkontrol melewati expires_at dan request scope/key diulang
- **THEN** result lama tidak memberi janji deduplication; consumer mengendalikan replace/creation atomik dan tetap memeriksa constraints/version untuk operasi baru

#### Scenario: Password-bearing request needs a fingerprint

- **WHEN** future CreateUser consumer menghitung fingerprint request password
- **THEN** perbandingan memakai keyed digest dengan secret/version terpisah yang dapat memverifikasi record unexpired, tanpa menyimpan password atau hash login

### Requirement: No-op replay and entity version remain distinct

Valid management no-op baru SHALL dapat menyimpan changed=false dan satu NO_OP audit sesuai policy, tanpa state-change event atau kenaikan entity version. Replay SHALL tidak menambah audit/event/version. Entity version MUST tidak diganti auth_version; foundation MUST tidak menaikkan auth_version untuk perubahan profile/role/app.

#### Scenario: A newly scoped command has no effective change

- **WHEN** consumer menetapkan no-op valid untuk key baru
- **THEN** safe changed=false result serta NO_OP audit dapat committed bersama tanpa event/version increment

#### Scenario: The no-op result is replayed

- **WHEN** result no-op yang sama diulang selama window
- **THEN** result asli di-replay tanpa NO_OP audit kedua

#### Scenario: Management no-op and self-service behavior keep their separate contracts

- **WHEN** a valid new keyed management request has no effective change, or an identical self `/profile.v1.ProfileService/UpdateProfile` is accepted under its existing contract
- **THEN** the appropriate NO_OP audit is written once without an event/version increment; self profile receives no new persistent replay, and self revoke-all is not converted into an ordinary management no-op

#### Scenario: Current authority is lost before a saved result is requested

- **WHEN** the verified actor repeats a committed management scope/key after losing current permission or session validity
- **THEN** the request is denied under current authentication/authorization before exposing a saved result, even though an unexpired result exists

#### Scenario: An identical committed result precedes target stale-version checks

- **WHEN** a currently authorized actor retries the same unexpired scoped key/payload after response loss, including the original expected entity version
- **THEN** the original safe management result is returned without another effect/audit/event or VERSION_CONFLICT caused solely by the already committed change

#### Scenario: Concurrent key scope and payloads remain distinct

- **WHEN** requests run concurrently with the same actor/full-method/canonical-target/key and equal or different normalized payloads, or reuse the key for another canonical target
- **THEN** equal requests produce at most one effective commit with replay or RESOURCE_BUSY for contenders; a changed fingerprint conflicts as ABORTED/IDEMPOTENCY_CONFLICT when the record is available, and another target never receives the first target result

#### Scenario: Presence expiry and role updates preserve identity semantics

- **WHEN** an optional field changes between omitted and explicit empty, an idempotency record expires, or `/role.v1.RoleService/UpdateRole`, `/role.v1.RoleService/ChangeRoleStatus` or `/role.v1.RoleService/ReplaceRolePermissions` effectively changes a role
- **THEN** presence remains part of the fingerprint, expiry permits a new command only under the same exclusion/validation protocol, and role changes increment only the appropriate entity version while current permission evaluation changes without revoking sessions or increasing user auth_version

### Requirement: Security revocation may commit before a denial response

Foundation SHALL mendukung owner pada layer features yang secara eksplisit memanggil Commit untuk security mutation, required audit/outbox, lalu mengembalikan authentication denial; RPC error MUST tidak otomatis menentukan rollback untuk kasus itu. Foundation MUST tidak melakukan token rotation, mengaktifkan kembali token consumed, atau memperpanjang absolute session expiry.

#### Scenario: Future reuse consumer commits revocation and then denies

- **WHEN** consumer reuse yang telah membuktikan credential memilih commit revocation/audit/event lalu menghasilkan UNAUTHENTICATED
- **THEN** committed security state tetap tersimpan meskipun outcome transport adalah error

#### Scenario: Security writes fail before commit

- **WHEN** salah satu required security write gagal
- **THEN** owner rollback, mempertahankan failure dependency, dan tidak mengklaim revocation telah committed

#### Scenario: Refresh busy is not evidence of consumed-token reuse

- **WHEN** a valid `/auth.v1.AuthService/RefreshSession` contender cannot acquire its required exclusion
- **THEN** RESOURCE_BUSY maps to ABORTED without converting contention into reuse revocation or delivering rotated credentials

#### Scenario: Production consumed-token reuse commits before denial

- **WHEN** `/auth.v1.AuthService/RefreshSession` proves a consumed token and successfully commits required session/family revocation, REFRESH_REUSE_DETECTED and applicable SESSION_REVOKED audit/event writes
- **THEN** the RPC returns UNAUTHENTICATED only after commit, terminal state and the single applicable event remain stored, and no grace, sliding expiry, legacy credential fallback or credential replay is introduced

#### Scenario: Reuse persistence or finalization cannot prove success

- **WHEN** required reuse audit/outbox persistence or its commit acknowledgement encounters a non-context dependency failure
- **THEN** the original persistence failure maps safely to UNAVAILABLE, no rotated credential is returned, and the response does not falsely assert committed revocation or guaranteed rollback after an unknown commit outcome

### Requirement: Foundation does not fabricate production command evidence

Audit/outbox/idempotency repositories MUST tidak menambahkan RPC management, credential replay, Register general idempotency, atau fake actor_service. Production wiring SHALL dilakukan atomik sejak command MVP berikutnya diaktifkan, setelah caller/authentication prerequisites tersedia.

#### Scenario: Persistence repositories pass integration tests

- **WHEN** repository consumer fixture lulus pada PostgreSQL test khusus
- **THEN** evidence menyatakan persistence capability saja dan tidak menandai Register PENDING, authoritative permission, last-admin, refresh, atau admin RPC end-to-end PASS

### Requirement: Register production wiring commits pending registration and verified audit atomically

`/user.v1.UserService/Register` SHALL membuat user PENDING beserta profile/default nonadmin member, USER_REGISTERED audit dengan verified service actor dan actor_user_id null, serta required pending `iam.user.created.v1` dalam satu transaction. Register MUST tidak membuat session/access/refresh atau otomatis mengangkat user pertama sebagai admin. Existing token field 1 SHALL dipertahankan kosong; metadata registration additive SHALL menunggu published api-contracts sebelum source wiring; compatible consuming-client readiness SHALL menjadi mandatory Phase 6 DEP-01 gate sebelum deployment, di luar checklist completion change backend. General idempotency/replay credential MUST tidak ditambahkan.

#### Scenario: Registration commits successfully

- **WHEN** verified caller mengirim valid registration dan seluruh required writes/commit berhasil
- **THEN** user PENDING, profile/member, safe audit dan pending event terlihat bersama; response token kosong dan tidak ada session/refresh

#### Scenario: Registration required persistence fails

- **WHEN** audit/outbox/mutation/commit gagal atau request canceled sebelum commit
- **THEN** tidak ada claimed success atau partial committed registration; failure cause/mapping tetap dipertahankan

#### Scenario: Duplicate or privilege-bearing registration is attempted

- **WHEN** duplicate canonical email atau forbidden privilege input mencapai validation/command boundary
- **THEN** request tidak memperoleh privilege dan tidak menghasilkan success audit/outbox; duplicate mengikuti existing business error contract

### Requirement: Profile production wiring commits safe fields audit and versioned event together

`/profile.v1.ProfileService/UpdateProfile` SHALL recheck active self principal setelah ordered locks dan commit name/address/phone, USER_PROFILE_UPDATED audit serta required pending `iam.user.profile_updated.v1` secara atomik untuk effective change. User entity version SHALL naik sekali, auth_version dan session SHALL tetap. Legacy full replacement tanpa mask SHALL dipertahankan; identical replacement SHALL menghasilkan changed=false dan satu NO_OP audit tanpa event/version increment. RPC MUST tidak diberi general persistent replay.

#### Scenario: Effective self profile update commits

- **WHEN** nonrestricted owner mengubah profile yang valid dan seluruh required writes/commit berhasil
- **THEN** hanya profile diri sendiri berubah, user version naik sekali, audit/event memakai committed version dan credential state tetap

#### Scenario: Profile does not change

- **WHEN** owner mengirim replacement identik yang valid
- **THEN** response safe mempunyai changed=false dengan satu NO_OP audit dan tanpa state-change event/version increment

#### Scenario: Audit or event write fails after profile write

- **WHEN** required audit/outbox gagal setelah tentative profile write
- **THEN** transaction rollback seluruh perubahan dan RPC tidak mengembalikan success

### Requirement: Production consumers preserve safe outcomes and prerequisite ownership

Scoped command SHALL memakai verified caller/user dari authentication boundary, request context dan satu feature-owned transaction. Required audit/event MUST tidak diganti best-effort side effect atau publication ke broker. Credential MUST tidak masuk safe snapshot/outbox/idempotency result. Real Login prerequisite SHALL commit session/refresh/SESSION_CREATED audit sebelum credential response, tanpa event creation baru atau credential replay; fixture persistence MUST tidak dinyatakan bukti producer production selesai.

#### Scenario: Pending event is stored while no relay exists

- **WHEN** Register atau effective UpdateProfile commit tanpa broker/relay
- **THEN** pending envelope catalog tetap tersimpan dengan published_at kosong dan command tidak bergantung delivery

#### Scenario: Login producer is integrated for activation

- **WHEN** actual production Login prerequisite memakai session writer dan success audit
- **THEN** session/refresh/audit mempunyai satu outcome commit, credentials hanya keluar sesudah commit, user version tetap dan tidak ada fabricated session-created event

#### Scenario: A failed command needs security attempt evidence

- **WHEN** command/authentication ditolak atau mutation transaction rollback
- **THEN** independent safe attempt audit dilakukan sesuai policy tanpa success effects; audit failure tidak memberi akses dan credential tetap redacted

#### Scenario: Register or profile response is lost

- **WHEN** commit berhasil tetapi response tidak diterima client
- **THEN** consumer tidak menjanjikan general persistent replay dan tidak menyimpan password/token sebagai response cache

#### Scenario: Direct calls retain the verified authority boundary

- **WHEN** a caller invokes a mutation directly with invalid required TLS/service identity, missing exact method/namespace authority, invalid required user session, a restricted actor outside the Me/ChangePassword/Logout allowlist, forbidden cross-owner target or protected system/last-admin change
- **THEN** the existing authentication/permission/business catalog mapping denies the request without success effects; actor identity from payload or metadata never replaces verified caller identity and no failed dependency becomes allow or an empty success

#### Scenario: Independent attempt persistence fails after command cleanup

- **WHEN** a verified-service request fails and its one independent safe attempt recorder errors or panics after command/read cleanup
- **THEN** the original safe outcome and locale remain, no credential/raw payload/private error is exposed, and failed observation is bounded without granting access or recursively creating attempts

#### Scenario: Login denial and refresh reuse are already accounted for

- **WHEN** generic LOGIN_FAILED or committed refresh reuse reaches the shared failed-attempt boundary
- **THEN** the boundary preserves UNAUTHENTICATED and does not add a duplicate independent attempt to the audit already owned by that path

#### Scenario: Rate or unverified identity is rejected

- **WHEN** admission rejects a request as RATE_LIMITED or no verified service actor exists
- **THEN** bounded limiter telemetry remains available where applicable but no per-rejection database attempt transaction or fabricated actor is created

#### Scenario: Existing clients keep their wire and credential contracts

- **WHEN** prior clients invoke any of the 37 registered methods after reconciliation, including Register, Login, RefreshSession, TesRPC and paginated reads
- **THEN** all existing protobuf field numbers/types, error/locale and pagination contracts remain; Register token field 1 stays empty/PENDING, only Login/Refresh return credentials, restricted methods stay Me/ChangePassword/Logout, and EdDSA-only issuer ApplicationName, ignored audience, 15-minute access bounded by absolute seven-day sessions and 30-second skew remain compatible

Evidence: [SAF-05..08, SAF-10..11](../../changes/archive/2026-10-10-auth-session-authorization-foundation/design.md#acceptance-dan-evidence-ledger). Delapan requirement foundation existing tetap berlaku. Catatan follow-up pada evidence foundation bersifat historis; status reconciliation production consumer terdapat pada evidence MAR di bawah, sedangkan broker delivery tetap terpisah.

## Traceability

The [MAR acceptance ledger](../../changes/archive/2026-10-18-auth-mutation-audit-reconciliation/design.md#acceptance-ledger) records MAR-01..07 scope, ownership, prerequisites and test requirements against the unchanged [MVP acceptance criteria](../../../docs/grpc-auth-mvp-spec.md#9-acceptance-criteria-dan-evidence). Its NOT_STARTED entries are the proposal baseline; [completed implementation evidence](../../changes/archive/2026-10-18-auth-mutation-audit-reconciliation/evidence/progress.md) records the current verified outcomes. The [mutation outcome matrix](../../changes/archive/2026-10-18-auth-mutation-audit-reconciliation/design.md#mutation-outcome-matrix) covers all 21 full methods. Phase 5 broader hardening, Phase 6 DEP-01, REST/UI and broker delivery remain separate.
