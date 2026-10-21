# User management backend

Empat RPC additive tersedia pada `/user.v1.UserService/`. Semua memerlukan TLS,
opaque `x-service-token` yang mengizinkan exact method dan namespace `iam`, serta
access token user ACTIVE yang tidak restricted. Actor selalu principal verified;
ID payload hanya target. Respons tidak mengisi legacy `User.password` dan tidak
mengandung hash, access/refresh token atau auth_version.

| RPC | Permission | Input penting |
| --- | --- | --- |
| GetUser | iam.user.read | id INTEGER positif |
| CreateUser | iam.user.create | name, email, password awal, explicit status PENDING/ACTIVE; idempotency-key wajib |
| UpdateUser | iam.user.update | user_id, expected_user_version positif, field_mask dan field presence; key opsional |
| ChangeUserStatus | iam.user.status.update | user_id, explicit status, expected_user_version, reason 3–500; key wajib |

GetUser memakai satu snapshot primary untuk authority, profil dan collection.
Target nondeleted dalam seluruh status bisnis dapat dibaca setelah authorization.
Profil yang belum ada ditampilkan kosong. Roles/access hanya assignment ACTIVE
dengan role/aplikasi ACTIVE. Missing/deleted target menghasilkan catalog not-found;
dependency failure menggagalkan seluruh response.

CreateUser menormalisasi trim/lowercase email, memakai Argon2id existing dan
membuat profil/default `iam.member` tanpa permission tambahan. Email/password
6–50 karakter; name 3–100, address maksimal250 dan phone maksimal20. Initial
version/auth_version=1, must_change_password=true. Tidak membuat sesi. Pengiriman
password awal ke pemilik akun adalah tanggung jawab consumer/operator external;
service tidak mengirim email/password atau mencatat credential ke audit.
PENDING belum dapat Login. ACTIVE menghasilkan Login restricted dengan allowlist
Me/ChangePassword/Logout saja; ChangePassword mencabut credential semua device
dan membutuhkan Login baru dengan password baru.

UpdateUser hanya menerima `name`, `address`, `phone`. Mask dan presence harus
cocok; field omitted tidak berubah, selected-empty dapat mengosongkan address/
phone. Name kosong dan mask email/password/status/owner/roles ditolak. Perubahan
efektif menambah user version satu, menjaga auth_version/password/session.
Expected version diperiksa sebelum no-op baru; stale menghasilkan
VERSION_CONFLICT/ABORTED walaupun nilainya sama. No-op valid membuat satu NO_OP
audit tanpa event atau counter tambahan.

Transisi status: PENDING→ACTIVE/DEACTIVATED; ACTIVE→SUSPENDED/DEACTIVATED;
SUSPENDED→ACTIVE/DEACTIVATED; DEACTIVATED→ACTIVE. Tidak kembali ke PENDING.
Self suspend/deactivate ditolak. Shared last-admin guard dan recount menjaga
minimal satu ACTIVE user dengan ACTIVE assignment `iam.platform_admin`.
Suspend/deactivate menambah version/auth_version sekali, mencabut seluruh sesi/
refresh family dan menghasilkan satu `iam.user.status_changed.v1` dengan
sessions_revoked=true; tidak menduplikasi event pencabutan sesi. Assignment
historis tetap. Reactivate menambah version tanpa mereset auth_version atau
memulihkan sesi maupun assignment/access yang sebelumnya REVOKED. Login baru
menggunakan generation terkini.

Mutation, success/no-op audit, required pending outbox dan keyed safe result
commit pada satu transaksi. Setiap error precommit, cancellation atau panic
rollback seluruh tentative writes. Setelah cleanup, security-attempt interceptor
mencatat failure lewat transaksi terpisah dengan catalog reason aman dan bounded
context. Kegagalan audit mempertahankan error asli dan menambah telemetry counter.
Commit failure dapat berarti outcome unknown; retry dengan key yang sama agar
server dapat mengecek safe result. RabbitMQ tidak diperlukan untuk pending outbox.

Idempotency key nonblank maksimal200 karakter. Scope actor+full method+
`account/{normalized_email}` atau `user/{id}`+key berlaku24jam. Current authority
diperiksa sebelum replay, dan replay mendahului target version. Identical retry
mengembalikan response awal, termasuk profil/collection awal, tanpa effect/audit/
event tambahan. Payload/password/presence berbeda dalam scope sama menghasilkan
IDEMPOTENCY_CONFLICT/ABORTED. Key sama pada target berbeda mempunyai scope terpisah.
Expired record diproses sebagai command baru di bawah lock yang sama.

`CommandFingerprint.ActiveVersion` dan `Keys` wajib di env.yaml. Setiap key
minimal32byte, independen dari JWT/limiter dan berbeda antarversion. Pertahankan
key lama selama24jam setelah record terakhir bergantung padanya. Missing retained
version fail-closed UNAVAILABLE. Placeholder aman tersedia pada
[env.example.yaml](../env.example.yaml); tidak ada default secret.

GetUser berbagi read quota120/menit burst20; tiga mutation berbagi admin quota
30/menit burst5. Invalid input ikut admission verified; penolakan memberi
RATE_LIMITED/RESOURCE_EXHAUSTED dan RetryInfo tanpa business writes. RESOURCE_BUSY
berbeda dari dependency unavailable.

Contoh payload aman (ID/version berasal dari detail aktual):

```json
{"id":42}
{"user_id":42,"expected_user_version":7,"address":"","field_mask":"address"}
{"user_id":42,"status":"SUSPENDED","expected_user_version":8,"reason":"Security review"}
```

FieldMask JSON protobuf berupa string. Contoh CreateUser menggunakan password
dari input secret lokal, tidak literal yang disalin dari dokumentasi. Caller
mengirim `authorization: Bearer <access>` dan metadata `idempotency-key` pada
command yang mewajibkannya; jangan log metadata credential.

Backend evidence: [auth-user-management](../openspec/changes/archive/2026-10-11-auth-user-management/evidence.md).
Deployment/schema/admin/key/caller/consumer readiness tetap mandatory dan OPEN
pada [Phase6 DEP-01](grpc-auth-mvp-spec.md#101-phase-6-readiness-deployment).
Role/application management increment B–E belum diimplementasikan.
