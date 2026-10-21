# Proposal

## Why

Increment C sudah mengelola assignment role, tetapi belum ada RPC untuk membuka akses aplikasi bisnis beserta role awal atau menutup akses beserta seluruh assignment aplikasi itu secara atomik. Increment D melengkapi gerbang akses yang sudah dipakai resolver permission, setelah [refactor existing](../2026-10-13-auth-existing-code-simplification/evidence/verification.md) selesai.

## What Changes

- Tambahkan `/user.v1.UserService/ListUserApplicationAccess`: histori ACTIVE/REVOKED, metadata aman dan current user version dari satu primary snapshot. Query lengkap mengikuti keputusan user: filter application/status, literal search, projection dan multi-sort seluruh scalar publik.
- Tambahkan `/application.v1.ApplicationService/GrantApplicationAccess`: target ACTIVE, aplikasi bisnis ACTIVE dan 1–20 submitted role IDs ACTIVE dari aplikasi yang sama; access dan role pilihan berubah dalam satu transaksi. Grant bersifat additive: role aktif lain dipertahankan; regrant hanya mengaktifkan role pilihan, tanpa memulihkan histori role lain.
- Tambahkan `/application.v1.ApplicationService/RevokeApplicationAccess`: revoke access serta seluruh assignment ACTIVE aplikasi tersebut, termasuk ketika target/role/aplikasi nonactive. Access absent adalah no-op pada access tanpa insert histori; assignment yang masih ACTIVE tetap dibersihkan. Aplikasi terdaftar dan nonsistem tetap wajib.
- Mutasi memerlukan dua exact permissions sekaligus, expected_user_version dan idempotency-key. Satu effective compound command menaikkan user version sekali dan menyimpan satu audit, satu application event serta safe replay result; auth_version/session/refresh dan aplikasi lain tetap.
- Pakai transaction owner, discovery locks, authoritative permissions, safe persistence, catalog query dan reusable provider existing. Pertahankan flow lokal yang eksplisit, guard clause dan fungsi fokus menurut KISS/DRY/YAGNI; tidak membuat framework command atau refactor lintas scope.
- Rencanakan contract additive di api-contracts, publication, backend wiring dan required workspace/standalone/PostgreSQL gates sebelum tiga RPC dibuka. Default registration menjadi tepat33 methods dengan baseline30 tetap kompatibel.

## Capabilities

### New Capabilities

- `user-application-access`: authoritative historical access listing dan compound grant/revoke dengan AND permissions, versioning, safe replay, atomic persistence dan compatibility.

### Modified Capabilities

Tidak ada requirement existing yang diganti. Capability baru memakai [authoritative authorization](../../../specs/authoritative-authorization/spec.md), [command persistence](../../../specs/command-persistence/spec.md), [transaction locking](../../../specs/transaction-locking/spec.md) dan [assignment C](../../../specs/user-role-assignment/spec.md) sebagai fondasi. ListApplications tetap catalog read; GrantUserRoles tetap memerlukan business access yang sudah ACTIVE.

## Impact

| Scope | Owner dan perubahan yang direncanakan | Prerequisite / status saat proposal |
| --- | --- | --- |
| EXISTING | grpc-auth: EdDSA session foundation, primary snapshots, assignment reader/writer, resolver, idempotency/audit/outbox, limiter dan hasil refactor | Tersedia pada working tree; perubahan A/B/C/refactor belum committed dan harus dipertahankan |
| MVP_REQUIRED D | grpc-auth: feature application access, domain/read/write adapters, thin handlers, composition, interceptor admission, tests dan API/evidence documentation | NOT_STARTED; dependency contract D belum tersedia |
| EXTERNAL_REPOSITORY contract D | api-contracts: tiga RPC/messages additive, generated output, compatibility/locale/error tests dan publication | Belum ada pada proto checkout; pin consumer saat ini `v0.0.0-20261012102712-bfb073390ea9`. Pekerjaan contract merupakan deliverable terencana D; perubahan sibling baru dilakukan pada apply dengan scope eksplisit, Git publication mengikuti otorisasi yang berlaku |
| EXTERNAL_REPOSITORY consumers | rest-orc/UI: transport mapping dan consumer integration | Di luar implementasi change ini; tidak mengedit repository tersebut |
| DEP-01 | Actual deployment schema/admin/key/TLS/caller/consumer readiness | Gate deployment terpisah, tetap terbuka; bukan task completion backend D |
| POST_MVP | Relay/broker/CLM resource policy, cache authz, distributed limiter, CRUD aplikasi/permission, wildcard/delegation/hierarchy | Tidak dibuat sebagai prerequisite atau placeholder |

Source inspection menemukan `user_application_access` ber-PK `(user_id, application_id)` tanpa surrogate ID; catalog existing mengasumsikan `id`. Desain harus menyesuaikan identity query D secara kecil tanpa mengubah cursor B/C. DDL aplikasi dan fixture tersedia, tetapi metadata live access belum ada pada baseline B: jangan mengklaim parity live atau menjalankan migration aplikasi otomatis. Tidak ada perubahan schema/config/credential maupun breaking change yang direncanakan.

Normative scope: [Phase2 increment D](../../../../docs/phase-2-management-plan.md#7-increment-d--application-access), [spec bisnis](../../../../docs/user-management-spec.md#33-akses-aplikasi), dan [matriks MVP](../../../../docs/grpc-auth-mvp-spec.md#42-matriks-full-rpc). AC-08/11/12/13/24–26/31 menjadi traceability D; EffectivePermission/EvaluateAccess public RPC dan CLM acceptance tetap E/Phase4/eksternal. Invocation ini hanya membuat planning artifacts; belum ada implementasi atau verification backend D.

## Implementation status

Backend D COMPLETE,33/33 tasks. Contract D published and pinned; all required workspace/standalone gates PASS. See [actual evidence](evidence/progress.md) and [API](../../../../docs/application-access-api.md). The impact table above records proposal-time status. DEP-01 remains open; sync/archive requires its own request.
