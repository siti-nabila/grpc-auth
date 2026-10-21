# Tasks

Contract dan backend increment A telah diimplementasikan. Hasil aktual dan
source manifest ada pada [evidence](evidence.md); contract publication ada pada
[contract inventory](contract-inventory.md). Seluruh36/36 task telah PASS, termasuk PostgreSQL standalone normal/race.
Deployment/migration/
bootstrap/consumer readiness tetap mandatory Phase6 DEP-01. Tidak melakukan
automatis sync/archive/commit/push grpc-auth atau deployment.

## 1. Pembuatan contract api-contracts dan backend prerequisites

- [x] 1.1 Di api-contracts baca AGENTS.md/status/proto/catalog/generated/tests dan consumer terkait, pertahankan perubahan user; catat inventory method/message/tag/type/semantic/error serta desain additive untuk GetUser, CreateUser, UpdateUser, ChangeUserStatus. Completion berupa matriks contract sebelum/sesudah dan scope diff, tanpa code runtime auth/REST/DB di repository contract.
- [x] 1.2 Tambah GetUser dan CreateUser ke proto/user/v1/user.service.proto, reuse GetUserRequest/GetUserResponse/CreateUserRequest/CreateUserResponse existing; tambahkan status presence PENDING/ACTIVE dan safe profile/status/version/created_at/must_change_password fields tanpa mengubah tag/type lama. Tambah tests/ descriptor/serialization scenarios untuk exact full methods, compatibility legacy wrappers dan password field tetap kosong pada safe fixture; dokumentasikan input/output creation/detail di docs contract yang relevan.
- [x] 1.3 Tambah UpdateUser dan ChangeUserStatus RPC beserta request/response: target ID, expected_user_version, explicit profile presence/field_mask, status/reason, changed/version dan safe output. Tambah contract tests omitted versus selected-empty, mask/target/version/status presence dan tidak adanya actor/role/credential response injection fields; dokumentasikan metadata idempotency-key wajib/opsional dan consumer semantics.
- [x] 1.4 Lengkapi semantic error inventory, feature dictionary user serta mapping/locales yang diperlukan empat RPC: reuse existing not-found/duplicate/validation/version/conflict/busy errors bila sesuai dan tambahkan illegal-transition/self-inactivation/LAST_ADMIN_REQUIRED hanya bila belum tersedia. Uji key/code/gRPC status, locale dan error decoder compatibility; update error contract docs tanpa remap legacy secara global.
- [x] 1.5 Jalankan generator repository api-contracts make proto, formatter source Go yang berubah, targeted contract tests, go test ./... dan go vet ./...; periksa generated diff dan descriptor seluruh RPC existing, regenerasi harus tidak menghasilkan perubahan tambahan. Verifikasi tests/ mengikuti runner/scenario pattern, README/docs aman, git diff --check/status serta tidak ada generated edit manual/secret/runtime code.
- [x] 1.6 Setelah 1.1–1.5 PASS, review diff contract dan siapkan publication commit/push normal pada branch existing sesuai otorisasi Git; catat commit/module/checksum dan verifikasi published module dapat diambil dengan GOWORK=off. Jangan membuat tag/release/merge/force-push; jika publication belum diizinkan/tersedia, simpan hasil siap review dan tandai hanya publication/dependent tasks BLOCKED, bukan mengabaikan pekerjaan contract yang dapat diselesaikan.
- [x] 1.7 Update dependency grpc-auth ke deliverable published 1.6; verifikasi module dapat diambil dan descriptor/inventory sama di workspace serta GOWORK=off, tanpa replace sementara, local generated atau module-cache edits.
- [x] 1.8 Cocokkan hanya metadata account/profile/search/admin_guard/session-generation/unique-email yang diperlukan dengan fixture dan schema-preflight evidence; simpan safe matrix/gap report, tidak mengeksekusi migration aplikasi atau mengubah timestamp legacy dengan asumsi timezone.
- [x] 1.9 Tambah typed config active/retained CreateUser password-fingerprint keyring dan startup validation memakai env.yaml existing; uji missing/short/duplicate-secret/unknown-version serta retained replay version dengan deterministic fixtures dan dokumentasikan placeholder aman di env.example.yaml tanpa secret nyata.

## 2. Domain, authorization dan safe replay

- [x] 2.1 Tambah input/result/domain contracts kecil di module user, validation INTEGER ID/status/profile/reason/version/mask/key, typed permissions/status/actions; uji omitted/selected-empty/forbidden fields dan batas schema melalui tests/ dengan existing testutils.
- [x] 2.2 Implementasi management authority recheck dan canonical lock assembly di feature/common sesuai idempotency→last-admin→policy→account→user→role→application→session; uji stale actor/permission, canonical actor=target deduplication, predecessor restart, acquired=false versus dependency failure, serta call order/tx identity tanpa lifecycle di repository.
- [x] 2.3 Extend typed safe-result allowlist sesuai published management response dan reuse Fingerprinter/ReplayPassword untuk CreateUser; uji exact original response replay, safe strict decoding, same-key different-password/presence/target, expiry24h, retained/unknown key version dan current authority sebelum replay sebelum expected-version check; dokumentasikan scope/payload aman di evidence change.

## 3. GetUser

- [x] 3.1 Tambah tx-bound safe detail reader dan feature primary consistent snapshot untuk actor/session/permission/target/profile/eligible collections; uji PENDING/ACTIVE/SUSPENDED/DEACTIVATED, missing/deleted targets, absent profile, dependency failure dan cleanup tanpa mutation/event.
- [x] 3.2 Tambah handler GetUser dengan mapping allowlist dan exact target validation; uji nil/invalid request, target versus actor, locale/context/deadline forwarding, current authority dan serialized response tidak mengandung password/hash/token.
- [x] 3.3 Dokumentasikan safe detail/collection policy dan update UM-02 dengan actual targeted test evidence; verifikasi local links serta snapshot test memastikan data dan authority tidak bercampur.

## 4. CreateUser

- [x] 4.1 Implementasi feature-owned creation memakai normalized account lock/shared creation persistence, explicit PENDING/ACTIVE, profile/default nonadmin creation, must_change_password=true dan initial counters1; uji duplicate/invalid input/privilege denial, current permission, hash-failure dan absence session/credential writes.
- [x] 4.2 Integrasikan required USER_CREATED audit, satu pending iam.user.created.v1 dan keyed safe result dalam business tx; uji failure setiap read/guard/affected-row/write/audit/event/result, panic/cancel/commit ambiguity, rollback setelah partial write dan independent failure-audit failure tanpa menimpa error asli.
- [x] 4.3 Tambah handler CreateUser sesuai additive contract; uji safe response, metadata key, normalized payload/status presence dan existing User.password selalu kosong, termasuk lost-response replay dan password fingerprint conflict.
- [x] 4.4 Tambah controlled flow CreateUser PENDING→activate serta ACTIVE→restricted Login→ChangePassword→fresh Login; buktikan exact restricted allowlist, counters/credential invalidation tanpa privilege escalation, dokumentasikan password-delivery ownership external dan actual UM-03/05/06 evidence.

## 5. UpdateUser

- [x] 5.1 Tambah feature/read/write target profile dengan presence/mask dan conditional expected_user_version; uji selected fields/empty/omitted, stale-before-no-op, dependency/affected-row failures, user version+1 sekali dan auth_version/session tetap.
- [x] 5.2 Integrasikan USER_PROFILE_UPDATED + pending iam.user.profile_updated.v1, optional idempotency dan new no-op audit; uji rollback seluruh tentative writes/result, exact original replay setelah target version berubah, optional-key absence dan response-loss, termasuk profile search visibility pada ListUsers.
- [x] 5.3 Tambah handler UpdateUser dan dokumentasi editable fields/version/replay; uji forbidden email/password/status/owner/role, invalid mask, safe mapping/context/locale serta rekam UM-04/05/06 actual evidence.

## 6. ChangeUserStatus dan last-admin

- [x] 6.1 Tambah tx-bound status/last-admin reader-writer dan feature global last-admin guard sebelum policy/aggregate locks; uji setiap valid/invalid transition, explicit status/reason, self-inactivation denial, version conflict/no-op order dan count dependency failure fail-closed.
- [x] 6.2 Integrasikan suspension/deactivation user+auth counters sekali, all sessions/families revoke, assignment preservation, USER_STATUS_CHANGED + satu pending status event sessions_revoked=true + safe result; uji per-write/late-guard/read failure rollback dan tidak ada duplicate global/per-session event.
- [x] 6.3 Implementasi activation/reactivation tanpa auth_version reset atau session restoration dan safe handler mapping/key replay; uji old access/refresh/latest-revoked Logout proof tetap invalid, revoked assignment/access tetap revoked dan eligible fresh Login menggunakan generation saat ini.
- [x] 6.4 Dokumentasikan transition/event/no-op/self/last-admin policy serta actual UM-09 evidence; targeted tests mencakup valid-key replay versus stale new version, actor invalidation sebelum recheck dan safe independent failure audit.

## 7. Admission dan production wiring

- [x] 7.1 Hubungkan GetUser ke shared read120/minute burst20 dan tiga mutation ke shared admin30/minute burst5; uji cross-method quota sharing, invalid-input admission, RetryInfo/capacity/dependency outcomes dan no business effects saat rate rejection.
- [x] 7.2 Wire concrete dependencies pada default configs server, empat methods pada UserService dan exact method/namespace service allowlist; controlled registered-gRPC tests membuktikan total19 methods, TLS/service/user/restricted/permission negatives, locale/cancellation serta existing resource cleanup.
- [x] 7.3 Tambahkan docs penggunaan backend management yang aman dan catat UM-01/07/08 evidence dari tests wiring; pastikan contoh tidak memuat key/password asli dan DEP-01 tetap mandatory belum PASS, tanpa REST/frontend/deployment edits.

## 8. Dedicated PostgreSQL dan concurrency integration

- [x] 8.1 Extend dedicated opt-in PostgreSQL harness untuk four registered RPCs dan atomic row assertions; test nyata rollback tiap required write/late business guard serta independent failure audit, exact audit/outbox/result counts dan pending events tanpa broker. Missing dedicated DSN/database membuat gate BLOCKED, bukan PASS.
- [x] 8.2 Tambah two-physical-connection/barrier scenarios duplicate normalized email dan same scoped key replay/conflict/expiry, stale target update dan actor invalidation; verifikasi constraints/locks serta pool/deadline/panic cleanup, bukan sleep sebagai satu-satunya proof.
- [x] 8.3 Tambah controlled last-admin concurrent status commands pada target berbeda dan races Login/Refresh/password versus inactive status; verifikasi minimal satu eligible admin tersisa, credentials tidak bertahan setelah invalidation committed, correct version/auth_version/event uniqueness. Cross-command role-revoke race dilanjutkan increment C ketika RPC-nya tersedia.
- [x] 8.4 Jalankan relevant tagged PostgreSQL suite dan race di dedicated test DB eksplisit; catat versi PostgreSQL/module/source digest, safe environment label, command/waktu/expected/actual UM-10 tanpa DSN/secret, tanpa Docker atau aplikasi DB migration.

## 9. Regression dan final evidence

- [x] 9.1 Jalankan gofmt changed Go, targeted tests lalu go test ./..., go vet ./..., go test -race ./... dan make build; ulangi workspace serta GOWORK=off dengan published dependency, termasuk existing15 RPCs, refresh commit-on-denial, generation proof dan seluruh dynamic ListUsers sort/filter/cursor/unique-total regression; semua required check harus PASS.
- [x] 9.2 Lengkapi evidence ledger UM-01..10 dengan actual results/commit atau source manifest/published contract/check commands, update roadmap hanya increment A yang terbukti; verifikasi dependency/schema blockers tidak diabaikan, remaining Phase2 B–E dan Phase4/5/6 tidak ditandai selesai.
- [x] 9.3 Jalankan openspec validate auth-user-management --strict, cek requirement/scenario→test/evidence coverage, relative links/anchors, git diff --check/status dan scope/secret/generated review; completion hanya setelah backend requirements/tests tersedia, tanpa otomatis sync/archive/commit/push/deploy.
