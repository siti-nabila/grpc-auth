# Spec Delta

## MODIFIED Requirements

### Requirement: Existing methods enforce exact permission and self ownership

`/user.v1.UserService/ListUsers` SHALL memerlukan verified active nonrestricted user/session dan exact permission `iam.user.read`. `/user.v1.UserService/Me` dan `/profile.v1.ProfileService/UpdateProfile` SHALL mengambil owner hanya dari principal; UpdateProfile MUST menolak restricted user dan hanya mengubah name/address/phone. Register/Login/TesRPC SHALL tetap mengikuti service-authenticated credential mode-nya, bukan user permission. Public methods, response safe fields dan existing field numbers/types MUST dipertahankan kecuali Register token-empty yang eksplisit. ListUsers SHALL mempertahankan default ID ASC, alias auth_id, role_codes numerik dan numeric last_id untuk mode sort ID; dynamic public-field multi-sort/composite cursor, statuses/application_code filters, safe response enrichment serta unique-user total SHALL mengikuti capability user-list-sorting sebagai perluasan yang disetujui eksplisit.

#### Scenario: ListUsers is called directly without permission

- **WHEN** caller service dan user/session valid memanggil ListUsers tanpa iam.user.read
- **THEN** server mengembalikan PERMISSION_DENIED, bukan page kosong atau implicit role-code bypass

#### Scenario: Cross-user or privileged profile input is attempted

- **WHEN** pemanggil mencoba mengubah owner/email/password/status/role melalui profile operation
- **THEN** request tidak mengubah field terlarang atau user lain; only self profile fields yang valid dapat ditulis

#### Scenario: Existing page and replacement clients call valid operations

- **WHEN** authorized client memakai existing ListUsers role/filter dan numeric cursor pada mode ID atau UpdateProfile tanpa field_mask additive
- **THEN** default ID ASC/auth_id/numeric last_id dan legacy full profile replacement tetap kompatibel tanpa local generated contract tambahan; published additive created_at tidak mengubah existing tags/types

#### Scenario: Extended ListUsers query still requires current authority

- **WHEN** caller memakai dynamic sort/composite cursor, status/application filter dan keyword search
- **THEN** exact iam.user.read dan active nonrestricted principal tetap wajib, query extension tidak menjadi bypass dan hasil mengikuti satu consistent primary snapshot
