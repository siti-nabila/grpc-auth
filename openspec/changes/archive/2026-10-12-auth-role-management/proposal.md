# Proposal

## Why

User management increment A sudah selesai, tetapi administrator belum dapat
membaca catalog atau mengelola custom business role melalui gRPC. Increment B
menyediakan fondasi role/permission sebelum assignment user dan application access.

## What Changes

- Tambah delapan RPC: `/role.v1.RoleService/ListRoles`,
  `/role.v1.RoleService/GetRole`, `/role.v1.RoleService/CreateRole`,
  `/role.v1.RoleService/UpdateRole`, `/role.v1.RoleService/ChangeRoleStatus`,
  `/role.v1.RoleService/ReplaceRolePermissions`,
  `/permission.v1.PermissionService/ListPermissions`, dan
  `/application.v1.ApplicationService/ListApplications`.
- Read/detail memakai exact permission dan satu primary snapshot; catalog
  mencakup role yang belum dipunyai user, terpisah dari filter-options user.
- List memakai cursor, filter/search dan sort dinamis seluruh scalar publik,
  termasuk multi-sort, nulls last dan ID tie-breaker. Ini keputusan user
  11 Oktober 2026; query tiap catalog dijabarkan dalam design/specs.
- `affected_user_count` menghitung user unik dengan assignment ACTIVE untuk
  role, tanpa membatasi status akun atau application access, sesuai keputusan
  user. Count adalah dampak assignment, bukan jumlah permission efektif.
- Custom role dibuat hanya pada aplikasi bisnis dengan permission terdaftar
  pada aplikasi sama. System/IAM legacy roles dilindungi; code/application
  immutable, metadata update memakai presence/mask, status ACTIVE/DISABLED.
- Mutation memakai expected role version, ordered locks, current authority,
  required audit/outbox dan persistent safe idempotency secara atomik. Semua
  precommit error rollback; failure audit terpisah setelah cleanup.
- Disable/replace permission langsung memengaruhi evaluasi baru tanpa mencabut
  sesi atau menaikkan user version/auth_version. Enable mempertahankan assignment.
- Pembuatan proto/messages, dictionary/mapping/locales, generation, compatibility
  tests, publication dan dependency update menjadi task eksplisit `api-contracts`
  sebelum backend wiring; tidak membuat generated contract lokal.

## Capabilities

### New Capabilities

- `role-administration`: create/update/status/replace permission custom business
  role dengan system protection, version, replay dan persistence atomik.
- `iam-catalog-reads`: ListRoles/GetRole/ListPermissions/ListApplications dengan
  authoritative snapshot, safe response, assignment count dan dynamic cursor.

### Modified Capabilities

Tidak ada. Existing authorization, transaction, persistence dan numeric admission
requirements dipakai kembali; coverage full method baru ada pada dua capability
di atas. Main specs tidak diubah saat proposal.

## Impact

| Scope | Owner dan prerequisite | Deliverable |
| --- | --- | --- |
| EXISTING | grpc-auth; [increment A evidence](../2026-10-11-auth-user-management/evidence.md) | 19 methods, transaction/snapshot providers, primary permission resolver, audit/outbox/idempotency dan limiter dipertahankan |
| MVP_REQUIRED | grpc-auth; published contract B | Domain/reader/writer, feature role/catalog, handler, constructor/registration, interceptor policy/admission, tests/evidence; target 27 methods |
| EXTERNAL_REPOSITORY, pekerjaan terencana | api-contracts; contract B belum ada | Delapan RPC additive, safe messages, errors/locales, generator/test/publication; backend dependency memakai module published |
| EXTERNAL_REPOSITORY, di luar increment | rest-orc/UI | Integrasi HTTP/UI, user filter-options dan client readiness |
| MVP_REQUIRED, increment berikutnya | grpc-auth; C–E dan Phase 4–6 sesuai [roadmap](../../../../docs/phase-2-management-plan.md) | Assignment/app access/effective-permission RPC serta audit read/EvaluateAccess dan hardening tetap tracked |
| POST_MVP | Project terkait | DeleteRole, permission/application CRUD, relay RabbitMQ, CLM policy dan distributed limiter tidak ditambahkan |

Source inspection: role domain/reader/writer existing hanya default-role/legacy
create; belum ada role/permission/application proto/service di checkout contract.
Graph baseline `b89f372` dipakai sebagai routing, lalu diverifikasi terhadap
working tree. Batas schema/sequence dan gap contract dijabarkan dalam design.

Proposal hanya menulis planning artifacts di change ini. Apply berikutnya
menjalankan task implementasi; Git publication mengikuti otorisasi eksplisit
yang berlaku dan tidak berarti izin membuat tag/release/merge/force-push.
Actual migration/bootstrap/key/caller/consumer readiness tetap mandatory
Phase 6 DEP-01, bukan completion gate backend B. Seluruh behavior baru masih
NOT_STARTED; tidak ada klaim implementation PASS dari dokumen.
