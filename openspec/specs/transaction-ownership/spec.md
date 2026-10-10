# transaction-ownership Specification

## Purpose

Menjamin Register dan UpdateProfile tidak melaporkan sukses sebelum transaksi berhasil, serta tidak meninggalkan perubahan parsial ketika operation, commit, cancellation, atau panic gagal.

## Requirements

### Requirement: Features own transaction and lock orchestration

Call site aplikasi untuk `Begin`, `Commit`, `Rollback`, dan `TryLock` SHALL berada di `internal/features/`, termasuk helper `features/common` bila diperlukan. Feature SHALL menentukan transaction boundary, canonical lock scope/order dan outcome, memulai transaksi dengan request context melalui injected provider, serta memberikan handle yang sama kepada repository. Repository MUST hanya melakukan read/write pada handle yang diberikan dan MUST NOT memulai/menutup transaksi atau memperoleh lock secara mandiri. Implementasi teknis provider dan adapter SHALL tetap memakai infrastructure/ORM existing.

#### Scenario: Feature orchestrates a successful command

- **WHEN** command mutasi memperoleh lock dan seluruh repository operations berhasil
- **THEN** feature memanggil Begin, TryLock, repository read/write, kemudian Commit pada satu handle; repository tidak memanggil lifecycle atau lock sendiri, dan response sukses hanya dibuat sesudah commit berhasil

#### Scenario: Feature orchestrates a failed command

- **WHEN** repository operation gagal atau command mengalami cancellation/panic sebelum commit
- **THEN** feature atau helper features/common menutup transaksi melalui Rollback, menjaga error/cause, dan repository tidak melakukan independent rollback atau commit

#### Scenario: Multiple repositories share one command transaction

- **WHEN** satu command menggunakan domain, audit, outbox atau idempotency repositories
- **THEN** seluruh repository menggunakan handle yang diberikan feature tanpa membuka transaksi terpisah atau mengambil lock tersembunyi, sehingga feature tetap mengendalikan satu outcome atomik

### Requirement: Register succeeds only after one atomic commit

Service SHALL memperlakukan auth, profil awal, dan assignment default member pada `/user.v1.UserService/Register` sebagai satu perubahan atomik. Credential response SHALL hanya dikembalikan setelah commit berhasil; kegagalan pembuatan credential sebelum commit MUST menggagalkan perubahan tersebut.

#### Scenario: Registration commits all existing records

- **WHEN** email/password valid, default member tersedia, seluruh write dan pembuatan token existing berhasil, lalu commit berhasil
- **THEN** satu auth, satu profil, dan satu assignment milik user baru tersimpan bersama dan response memakai UserTokenResponse existing

#### Scenario: Any registration write fails

- **WHEN** write auth, profil, atau assignment default gagal sesudah transaksi dimulai
- **THEN** request gagal tanpa credential response dan tidak ada subset perubahan tersebut committed

#### Scenario: Token generation or commit fails

- **WHEN** pembuatan token atau commit gagal
- **THEN** kegagalan diteruskan ke boundary error dan tidak ada response sukses atau token yang dikembalikan; commit outcome yang tidak diketahui tidak diklaim pasti rollback

### Requirement: Profile success reflects the committed owner's record

`/profile.v1.ProfileService/UpdateProfile` SHALL menentukan owner dari authenticated request context, mengubah hanya name/address/phone profil owner, dan mengembalikan ID serta user_id record yang committed. Service MUST mempertahankan full replacement scalar fields existing; field-mask/partial update tidak ditambahkan dalam change ini.

#### Scenario: Own profile update commits

- **WHEN** user terautentikasi mengirim tiga field profil dan update serta commit berhasil
- **THEN** response ProfileResponse berisi profile ID dan user_id owner yang benar serta nilai committed, tanpa mengubah email, password, status, role, atau ownership

#### Scenario: Profile update or commit fails

- **WHEN** update atau commit gagal
- **THEN** request mengembalikan error, bukan profil sukses, dan operation error tidak hilang karena cleanup membaca nilai error lama

#### Scenario: Missing principal or missing profile

- **WHEN** identitas context tidak tersedia atau profil owner tidak ditemukan
- **THEN** request gagal sesuai error catalog yang relevan, tidak menulis profil lain dan tidak mengembalikan success default ID nol

### Requirement: Failure and panic close transaction ownership safely

Setiap transaksi yang dimulai SHALL berakhir commit atau rollback melalui satu owner. Operation failure, cancellation, dan panic sebelum commit MUST memicu rollback, mempertahankan error/cause yang relevan, dan tidak berubah menjadi commit karena urutan cleanup. Panic SHALL menghasilkan internal error generik tanpa payload sensitif.

#### Scenario: Panic after a successful first write

- **WHEN** Register atau UpdateProfile panic setelah satu write berhasil tetapi sebelum commit
- **THEN** transaksi rollback, tidak ada perubahan parsial, lock dapat dipakai transaksi lain, dan caller menerima internal error generik

#### Scenario: Operation and rollback both fail

- **WHEN** operation menghasilkan error dan rollback juga menghasilkan error
- **THEN** operation failure tetap menjadi kegagalan request, cleanup failure dapat didiagnosis secara redacted, dan tidak ada response sukses

#### Scenario: A subsequent request uses the same repository owner

- **WHEN** satu request selesai dan dependency yang sama dipakai request berikutnya
- **THEN** transaksi berikutnya baru dan tidak menggunakan handle transaksi yang sudah ditutup atau milik request lain

### Requirement: Request cancellation applies to the whole transaction

Begin, seluruh state check/read, dan write terkait SHALL memakai deadline/cancellation request yang sama. State yang menentukan mutation SHALL dibaca ulang dalam transaksi sesudah lock; request terbatalkan MUST tidak menjalankan commit-success path setelah failure diketahui.

#### Scenario: Request is cancelled before begin

- **WHEN** request context sudah cancelled sebelum transaksi dimulai
- **THEN** request gagal dengan cancellation semantics tanpa business write

#### Scenario: Request is cancelled during a transaction

- **WHEN** context cancelled pada check/read/write sebelum commit
- **THEN** error dipropagasikan, transaction ownership ditutup aman, dan tidak ada mutation sukses parsial

### Requirement: Email normalization is consistent across registration and login

Register dan `/user.v1.UserService/Login` SHALL menerapkan trim dan lowercase sebelum validation serta lookup email, mengikuti keputusan MVP. Registrasi SHALL menyimpan email canonical dan duplicate check mengikuti canonical identity. Deployment MUST tidak menghapus, menggabungkan, atau mengganti akun legacy yang collision secara otomatis.

#### Scenario: Canonical email is used consistently

- **WHEN** user register dengan email uppercase/whitespace lalu login dengan variasi casing yang setara
- **THEN** validation/lookup memakai email canonical yang sama dan tidak menciptakan identity kedua untuk variasi tersebut

#### Scenario: A canonical email already exists

- **WHEN** registrasi menemukan identity dengan email canonical yang sama setelah lock
- **THEN** request ditolak dengan duplicate catalog existing dan tidak membuat auth/profile/assignment kedua

#### Scenario: Baseline contains normalization collisions

- **WHEN** preflight menemukan lebih dari satu akun legacy yang menjadi email canonical sama
- **THEN** normalization rollout berstatus BLOCKED sampai resolusi data disetujui, tanpa overwrite atau merge otomatis

### Requirement: Transaction hardening preserves the pre-cutover contract

Change ini MUST mempertahankan full method, field number/type, default member non-admin, dan credential semantics existing sebelum cutover. Register PENDING/token-empty, TTL access 15 menit/session absolut 7 hari/skew 30 detik, issuer ApplicationName, audience ignored, dan EdDSA Phase 3 tetap keputusan target MVP pada change keamanan berikutnya; fondasi ini MUST tidak menggantikan atau mengklaim menerapkannya.

#### Scenario: Existing protobuf clients use the foundation increment

- **WHEN** client existing memanggil Register/Login/UpdateProfile setelah fondasi diterapkan sebelum cutover
- **THEN** bentuk request/response tetap kompatibel, TesRPC dan pagination ListUsers tidak diubah, dan tidak ada field presence atau RPC baru yang diwajibkan

#### Scenario: Foundation evidence is reported

- **WHEN** test fondasi lulus
- **THEN** hasil itu hanya membuktikan lifecycle transaksi yang diuji; AC register PENDING, sesi persisten, permission authoritative dan cutover tetap belum PASS
