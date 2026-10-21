# Spec Delta

## ADDED Requirements

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

Evidence: [SAF-05..08, SAF-10..11](../../design.md#acceptance-dan-evidence-ledger). Delapan requirement foundation existing tetap berlaku; management idempotency production consumer, full lifecycle dan broker delivery masih follow-up.
