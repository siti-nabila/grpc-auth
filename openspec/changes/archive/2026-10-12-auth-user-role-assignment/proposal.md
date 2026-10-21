# Proposal

## Why

Increment A dan B sudah menyediakan administrasi user serta catalog/custom role,
tetapi administrator belum dapat melihat seluruh assignment, memberikan role,
atau mencabutnya melalui RPC. Increment C melengkapi assignment yang atomik dan
terotorisasi sambil menjaga admin aktif terakhir dan credential user yang masih valid.

## What Changes

- Tambahkan `/user.v1.UserService/ListUserRoles`,
  `/user.v1.UserService/GrantUserRoles`, dan
  `/user.v1.UserService/RevokeUserRoles` dengan exact permissions
  `iam.user.read`, `iam.user.role.grant`, dan `iam.user.role.revoke`.
- List memakai satu primary snapshot untuk authority, target, assignment, total,
  dan metadata sumber. Keputusan user 12 Oktober 2026: default mencakup ACTIVE
  dan REVOKED; filter application/status assignment, literal search, projection,
  serta multi-sort seluruh scalar publik mengikuti pola catalog B.
- Grant/revoke memakai nonempty set maksimal 20 role, expected_user_version,
  reason, mandatory idempotency-key, locked current authority dan all-or-nothing
  persistence. Grant membutuhkan target/role/application ACTIVE dan business
  application access ACTIVE; IAM tidak membutuhkan app-access grant.
- Satu pair user/role tetap satu record; regrant mengaktifkan pair yang revoked.
  Keputusan user 12 Oktober 2026: revoke role terdaftar tanpa pair assignment
  menjadi no-op; role ID yang tidak terdaftar menggagalkan seluruh batch.
- Effective command menaikkan user version sekali dan menyimpan satu audit,
  satu pending batch event dengan changed_role_ids, serta safe replay result.
  No-op/replay tidak menggandakan efek. Session, refresh dan auth_version tetap.
- Larang self revoke iam.platform_admin dan reuse global last-admin guard yang
  sama dengan ChangeUserStatus, termasuk concurrency lintas-command.
- Rencanakan additive proto/generated/error/locale contract di api-contracts,
  publication dan dependency update sebelum aktivasi backend. Registrasi target
  menjadi 30 RPC; 27 method baseline dan public contracts existing tetap kompatibel.

## Capabilities

### New Capabilities

- `user-role-assignment`: Protected assignment listing/query, atomic batch
  grant/revoke, same-pair lifecycle, safe replay, shared last-admin protection,
  current permission evaluation, numeric admission dan compatible contract.

### Modified Capabilities

Tidak ada perubahan requirement main spec existing. `user-status-management`
sudah mewajibkan shared last-admin guard dengan assignment revoke;
`authoritative-authorization`, `command-persistence` dan transaction foundations
menjadi prerequisite yang dipakai kembali, bukan capability baru yang diduplikasi.

## Impact

- **MVP_REQUIRED, grpc-auth:** Extend feature user, domain assignment contracts,
  transaction-bound reader/writer, handlers, foundation registration,
  interceptors serta root tests/. Baseline bersih commit `2462cb0`; source
  management/policy/resolver/persistence sudah tersedia, tiga RPC target belum ada.
- **EXTERNAL_REPOSITORY, api-contracts:** Tambahkan tiga RPC/messages dan semantic
  errors yang belum tersedia melalui generator resmi, localized mappings dan
  compatibility tests. Dependency saat ini
  `v0.0.0-20261012030550-fd1147049d52` belum memiliki RPC C. Contract work bagian
  task increment C; published module adalah gate sebelum backend wiring, bukan
  alasan menganggap implementation selesai atau membuat proto lokal.
- **Prerequisites/data:** Reuse user_role unique pair/status/provenance,
  user_application_access, admin_guard, auth.version dan persistence tables.
  Archived B metadata hanya baseline historis; kecocokan kolom/index/constraint
  C tetap perlu diverifikasi saat apply. Proposal tidak menjalankan DB/migration.
- **Out of this increment:** Application-access commands (D), effective-permission
  read RPCs (E), EvaluateAccess/ListAuditLogs, REST/UI/filter-options, bootstrap
  dan deployment. Actual deployment tetap mandatory Phase 6 DEP-01; tidak menjadi
  pending completion backend C. CRUD permission/application, DeleteRole, relay,
  CLM resource policy, distributed limiter dan delegation tetap POST_MVP.
- Perubahan saat propose terbatas planning artifacts; tidak mengedit sibling,
  code, dependency, SQL atau Git publication. Artefak ini bukan evidence PASS
  behavior. [Phase 2 plan](../../../../docs/phase-2-management-plan.md) serta
  [spec MVP](../../../../docs/grpc-auth-mvp-spec.md) tetap rujukan utama.
