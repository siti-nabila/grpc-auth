# Spec Delta

## Purpose

Menyediakan identitas user terverifikasi yang terikat persistent session dan state primary database, dengan batas waktu, restricted policy, serta dependency activation yang tidak membuka token legacy.

## ADDED Requirements

### Requirement: Protected credentials identify a persistent active session

Protected RPC SHALL memverifikasi JWT EdDSA/Ed25519, exact issuer ApplicationName, decimal user sub, UUID sid/jti, iat, exp dan auth_version. Legacy user_id bila hadir MUST sama dengan sub; audience sengaja tidak divalidasi. Primary snapshot SHALL membuktikan user ACTIVE, session ACTIVE, session.user_id sama dengan sub, session belum expired dan token auth_version sama dengan user. Missing/invalid credential atau state SHALL menghasilkan generic UNAUTHENTICATED tanpa mempercayai user ID/role payload.

#### Scenario: Token and persistent state agree

- **WHEN** service caller valid memakai JWT valid dan primary snapshot memenuhi seluruh status/ownership/version checks
- **THEN** feature menerima verified user/session principal, bukan sekadar user ID yang diklaim request

#### Scenario: Signature claims or ownership do not agree

- **WHEN** algorithm/signature/issuer salah, required claim tidak valid/hilang, legacy user_id berbeda, sid milik user lain atau auth_version stale
- **THEN** protected RPC ditolak UNAUTHENTICATED tanpa success effects

#### Scenario: Session or user becomes inactive

- **WHEN** snapshot menunjukkan PENDING/SUSPENDED/DEACTIVATED user atau missing/revoked/expired session
- **THEN** protected request ditolak dan reactivation user saja tidak memulihkan session revoked

#### Scenario: Audience differs but the required checks pass

- **WHEN** JWT mempunyai audience berbeda atau tidak mempunyai audience, sementara semua required checks valid
- **THEN** audience tidak menjadi alasan denial

### Requirement: Session expiry is absolute and distinct from JWT skew

Access lifetime SHALL maksimal 15 menit dan exp MUST tidak melewati expires_at session. Session/refresh lifetime SHALL maksimal absolute 7 hari sejak Login; refresh tidak memperpanjang session. JWT clock skew 30 detik SHALL hanya berlaku pada pemeriksaan waktu token dan MUST tidak memperpanjang absolute session expiry.

#### Scenario: JWT skew overlaps the session expiry

- **WHEN** controlled clock telah mencapai expires_at session meskipun JWT masih dalam clock skew yang diperbolehkan
- **THEN** request ditolak UNAUTHENTICATED

#### Scenario: Issuer tries to extend the session window

- **WHEN** token exp melampaui batas session atau consumer mencoba memperpanjang expiry session melalui refresh
- **THEN** state/token tersebut tidak memenuhi authentication contract dan tidak memberi akses melewati absolute window

### Requirement: Restricted users have an exact self-service allowlist

must_change_password SHALL dibaca authoritative dari user. Restricted allowlist penuh SHALL hanya `/user.v1.UserService/Me`, `/auth.v1.AuthService/ChangePassword`, dan `/auth.v1.AuthService/Logout`. Me SHALL mengembalikan identitas/profil minimum tanpa role/app/permission admin. Method lifecycle yang belum tersedia MUST tidak dibuat sebagai placeholder oleh change ini.

#### Scenario: Restricted user reads their identity

- **WHEN** restricted principal valid memanggil Me
- **THEN** response hanya memuat identitas/profil minimum diri sendiri tanpa privilege metadata admin

#### Scenario: Restricted user calls an operation outside the allowlist

- **WHEN** restricted principal memanggil UpdateProfile, ListUsers, atau future RefreshSession dengan credential valid
- **THEN** request ditolak PERMISSION_DENIED tanpa mutation atau refresh rotation

### Requirement: Authentication dependency failure never becomes access or business denial

User/session primary read failure SHALL menghasilkan dependency error dengan target mapping UNAVAILABLE, bukan allow, empty success, atau PERMISSION_DENIED. Deadline/cancellation SHALL mempertahankan context dan shared transport mapping. Failed-attempt audit failure MUST tidak mengubah authentication denial menjadi allow; credential MUST tidak terekspos pada error/telemetry.

#### Scenario: Primary database is unavailable

- **WHEN** valid token tidak dapat divalidasi karena session/user primary read gagal
- **THEN** RPC mengembalikan dependency failure tanpa success effects atau fallback cache

#### Scenario: Security-attempt audit also fails

- **WHEN** authentication telah ditolak dan independent safe attempt audit gagal
- **THEN** denial tetap berlaku dengan bounded redacted audit-failure telemetry

### Requirement: Production activation requires a compatible real Login producer

Protected session pipeline source SHALL dihubungkan dan diuji bersama real production Login yang commit session/refresh dan audit sebelum mengembalikan EdDSA token berisi required claims. Published contract, local key/caller/basic limiter/schema/admin readiness SHALL menjadi mandatory backend gate. Actual consuming-client/IP/retry/compatibility dan deployment key/caller rotation/schema/admin readiness SHALL tetap mandatory sebelum deployment, dicatat pada mandatory Phase 6 DEP-01 dalam spec MVP di luar checklist change foundation. Source/backend increment SHALL dapat selesai setelah requirements/tests backend PASS tanpa menandai deployment readiness PASS. HS256/sid-less legacy token MUST ditolak sesudah cutover; fixture session, test-only issuer, token-only Login, dual algorithm atau runtime bypass MUST tidak menggantikan prerequisite. Backend readiness gagal SHALL menahan source wiring; external/deployment readiness gagal SHALL menahan deployment; rollback MUST tidak memulihkan revoked sessions/auth_version/legacy credentials.

#### Scenario: Foundation unit tests pass but Login remains legacy

- **WHEN** verifier/reader tests lulus tetapi production Login masih menghasilkan HS256 tanpa persistent session
- **THEN** activation tetap BLOCKED dan change tidak diklaim production-complete

#### Scenario: Coordinated production pipeline is verified

- **WHEN** seluruh backend prerequisite terpenuhi dan real Login integration menghasilkan committed session plus token yang diperiksa registered protected RPC
- **THEN** current credential dapat mengikuti policy protected dan HS256/token tanpa sid tetap ditolak; deployment tetap menunggu external readiness

Evidence: [SAF-02, SAF-04, SAF-06..10](../../design.md#acceptance-dan-evidence-ledger). Full password/revoke/refresh lifecycle tetap follow-up SAF-11; state-validation tests tidak menutup seluruh AC-14..18/33..34.

#### Scenario: Backend completes while consuming-client work is deferred

- **WHEN** user authorizes backend source wiring and all backend prerequisites/tests pass while EXT-02 consuming-client work remains deferred
- **THEN** all six existing RPCs use the complete session/caller/policy pipeline in source, backend evidence can pass, and the global Phase 6 DEP-01 deployment gate remains unchecked outside the completed backend change, without legacy fallback or deployment

Deployment gate owner/location: [Phase 6 DEP-01](../../../../../../docs/grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment); actual consumer/deployment readiness tetap wajib sebelum release.
