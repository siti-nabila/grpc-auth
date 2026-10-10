# authoritative-authorization Specification

## Purpose

Mengambil keputusan permission dan ownership dari state primary yang konsisten dengan data atau mutation request, agar token yang masih valid selalu mengikuti akses user terbaru.

## Requirements

### Requirement: Effective permissions are an authoritative active-role union

Permission SHALL merupakan union mapping dari assignment ACTIVE dan role ACTIVE pada primary database, dengan contributing roles dipertahankan. Business application permission SHALL juga memerlukan application ACTIVE dan application access ACTIVE milik user. Role code numerik tertentu, wildcard, inheritance, direct grant, JWT role snapshot, event atau cache MUST tidak menjadi bypass. IAM self-service MUST tidak memerlukan admin role atau IAM application grant.

#### Scenario: Two active roles contribute the same permission

- **WHEN** dua role aktif memberi permission yang sama dan satu assignment dicabut sebelum evaluasi berikutnya
- **THEN** permission tetap diberikan dari role aktif yang tersisa dan source contribution mengikuti snapshot terbaru

#### Scenario: A role or application gate is inactive

- **WHEN** assignment/role nonactive atau business application/access nonactive
- **THEN** kontribusi role tersebut tidak memberi permission walaupun JWT masih mencantumkan role lama

#### Scenario: A self-service user has no admin role

- **WHEN** active verified user/session tanpa IAM admin role atau IAM app grant memanggil Me atau UpdateProfile yang memenuhi restricted/ownership policy
- **THEN** self-service tidak ditolak hanya karena tidak mempunyai privilege admin

### Requirement: Existing methods enforce exact permission and self ownership

`/user.v1.UserService/ListUsers` SHALL memerlukan verified active nonrestricted user/session dan exact permission `iam.user.read`. `/user.v1.UserService/Me` dan `/profile.v1.ProfileService/UpdateProfile` SHALL mengambil owner hanya dari principal; UpdateProfile MUST menolak restricted user dan hanya mengubah name/address/phone. Register/Login/TesRPC SHALL tetap mengikuti service-authenticated credential mode-nya, bukan user permission. Public methods, response safe fields, existing field numbers/types dan auth_id/last_id pagination MUST dipertahankan kecuali Register token-empty yang eksplisit.

#### Scenario: ListUsers is called directly without permission

- **WHEN** caller service dan user/session valid memanggil ListUsers tanpa iam.user.read
- **THEN** server mengembalikan PERMISSION_DENIED, bukan page kosong atau implicit role-code bypass

#### Scenario: Cross-user or privileged profile input is attempted

- **WHEN** pemanggil mencoba mengubah owner/email/password/status/role melalui profile operation
- **THEN** request tidak mengubah field terlarang atau user lain; only self profile fields yang valid dapat ditulis

#### Scenario: Existing page and replacement clients call valid operations

- **WHEN** authorized client memakai existing ListUsers filter/cursor atau UpdateProfile tanpa field_mask additive
- **THEN** filter/auth_id/last_id semantics dan legacy full profile replacement tetap kompatibel tanpa local generated contract tambahan

### Requirement: Authorization and sensitive response use the same snapshot

Read RPC SHALL membaca user/session/effective permission dan sensitive response data dari satu consistent primary snapshot. Evaluation yang dimulai setelah role/app/permission revocation committed SHALL memakai state baru tanpa menaikkan auth_version atau mencabut session untuk perubahan privilege tersebut. Request yang sudah authorized sebelum perubahan MUST tidak diberi janji cancellation in-flight. Dependency failure SHALL menghasilkan UNAVAILABLE, bukan allow, permission denial atau empty success.

#### Scenario: Revocation commits before the next evaluation

- **WHEN** role permission/assignment/app access diubah committed lalu ListUsers dievaluasi dengan token/session lama yang masih valid
- **THEN** identitas session tetap dapat valid tetapi decision memakai permission terbaru

#### Scenario: Sensitive page is empty

- **WHEN** page query tidak menemukan user tetapi actor tidak mempunyai permission atau session-nya tidak valid
- **THEN** server tetap menghasilkan denial yang sesuai dan tidak menyamarkan failed authorization sebagai empty success

#### Scenario: Policy changes during a read

- **WHEN** controlled concurrent commit mengubah permission antara candidate authorization/data reads
- **THEN** response dan authorization berasal dari satu snapshot yang pernah valid bersama, bukan gabungan state berbeda

### Requirement: Mutations recheck current authority after ordered locks

Feature mutation SHALL memeriksa ulang actor/user/session/ownership/permission yang relevan setelah shared ordered locks dalam transaction yang sama dengan target read dan seluruh write. Stale interceptor decision MUST tidak mengizinkan mutation. Lock contention SHALL menghasilkan ABORTED/RESOURCE_BUSY tanpa efek; dependency error bukan contention. Future management replay SHALL tetap memerlukan current actor/session/permission sebelum safe result dikembalikan.

#### Scenario: Actor is revoked after initial authentication

- **WHEN** actor session/status/authority berubah committed sebelum mutation memperoleh lock dan recheck
- **THEN** mutation ditolak menurut current state tanpa success audit/outbox atau target write committed

#### Scenario: Profile lock cannot be acquired

- **WHEN** UpdateProfile memperoleh acquired=false pada TryLock
- **THEN** RPC mengembalikan ABORTED/RESOURCE_BUSY tanpa mutation, audit sukses atau event

#### Scenario: A future safe replay loses actor authority

- **WHEN** management result tersedia tetapi actor session/endpoint permission kini invalid
- **THEN** consumer tidak mengembalikan replay yang melewati current authorization

Evidence: [SAF-03..06, SAF-10..11](../../changes/archive/2026-10-10-auth-session-authorization-foundation/design.md#acceptance-dan-evidence-ledger). Phase 2 management/EvaluateAccess RPC tidak dibuat; resolver/read/recheck evidence hanya subset AC-09..13/23/27/31, bukan full management flow PASS.
