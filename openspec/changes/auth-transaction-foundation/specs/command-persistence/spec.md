# Spec Delta

## Purpose

Menyediakan persistence audit, pending event, dan hasil idempotency yang aman serta atomik untuk command IAM berikutnya, tanpa menganggap adanya tabel atau repository sebagai bukti command bisnis telah berjalan.

## ADDED Requirements

### Requirement: Required persistence shares the mutation transaction

Repository persistence SHALL memungkinkan satu consumer menyimpan mutation, audit, required outbox, dan safe idempotency result dalam satu transaction commit. Persistence MUST tidak melakukan independent commit, memakai fallback pool read/write setelah mutation lock, atau memanggil broker/external service dalam transaksi.

#### Scenario: All command writes succeed

- **WHEN** consumer terverifikasi menulis mutation, audit, event required, dan safe result lalu commit berhasil
- **THEN** seluruh row terlihat bersama dengan transaction outcome yang sama

#### Scenario: Any required write fails

- **WHEN** audit, required outbox, atau idempotency result gagal setelah mutation ditulis
- **THEN** seluruh write transaksi rollback dan tidak ada hasil sukses yang dapat di-replay

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

### Requirement: Security revocation may commit before a denial response

Foundation SHALL mendukung owner pada layer features yang secara eksplisit memanggil Commit untuk security mutation, required audit/outbox, lalu mengembalikan authentication denial; RPC error MUST tidak otomatis menentukan rollback untuk kasus itu. Foundation MUST tidak melakukan token rotation, mengaktifkan kembali token consumed, atau memperpanjang absolute session expiry.

#### Scenario: Future reuse consumer commits revocation and then denies

- **WHEN** consumer reuse yang telah membuktikan credential memilih commit revocation/audit/event lalu menghasilkan UNAUTHENTICATED
- **THEN** committed security state tetap tersimpan meskipun outcome transport adalah error

#### Scenario: Security writes fail before commit

- **WHEN** salah satu required security write gagal
- **THEN** owner rollback, mempertahankan failure dependency, dan tidak mengklaim revocation telah committed

### Requirement: Foundation does not fabricate production command evidence

Audit/outbox/idempotency repositories MUST tidak menambahkan RPC management, credential replay, Register general idempotency, atau fake actor_service. Production wiring SHALL dilakukan atomik sejak command MVP berikutnya diaktifkan, setelah caller/authentication prerequisites tersedia.

#### Scenario: Persistence repositories pass integration tests

- **WHEN** repository consumer fixture lulus pada PostgreSQL test khusus
- **THEN** evidence menyatakan persistence capability saja dan tidak menandai Register PENDING, authoritative permission, last-admin, refresh, atau admin RPC end-to-end PASS
