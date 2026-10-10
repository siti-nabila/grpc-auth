# transaction-locking Specification

## Purpose

Mencegah operasi bersamaan pada account atau profil yang sama menghasilkan state tidak konsisten, sambil membedakan contention biasa dari kegagalan dependency dan menjamin lock berakhir bersama transaksi.

## Requirements

### Requirement: Account creation shares a canonical exclusion scope

Registrasi email canonical sama SHALL dikoordinasikan dalam satu exclusion scope sampai transaksi selesai. Check duplicate dan default-member state yang dipakai creation SHALL konsisten dengan write pada transaksi tersebut. Unique constraints MUST tetap menolak duplicate meskipun caller lain tidak mengikuti protocol.

#### Scenario: Two registrations overlap on the same account

- **WHEN** dua transaksi registrasi dengan email canonical sama berjalan pada koneksi berbeda dan transaksi pertama masih memegang scope
- **THEN** hanya satu memperoleh scope; request lain menerima RESOURCE_BUSY/ABORTED tanpa partial write, bukan duplicate yang disimpulkan dari state uncommitted

#### Scenario: Registration arrives after the first commit

- **WHEN** request kedua memperoleh scope sesudah registrasi pertama commit
- **THEN** duplicate check membaca record committed dan menolak creation kedua sesuai catalog duplicate existing

#### Scenario: Another writer bypasses cooperative locking

- **WHEN** writer lain mencoba memasukkan email yang sama tanpa mengikuti exclusion scope
- **THEN** unique constraint tetap melindungi identity dan request tidak menghasilkan duplicate account

### Requirement: Profile mutation shares the user's exclusion scope

UpdateProfile SHALL memakai user yang berasal dari authenticated context sebagai target exclusion scope, bukan profile ID atau user ID dari payload. Profil SHALL dibaca ulang setelah exclusion diperoleh, agar response dan mutation berasal dari owner yang sama.

#### Scenario: Two profile updates contend for the same owner

- **WHEN** dua UpdateProfile untuk user sama overlap sebelum commit pertama
- **THEN** satu operasi menang dan operasi yang belum memperoleh scope gagal busy tanpa mutation; winner mengembalikan profil committed miliknya

#### Scenario: Different users update independent profiles

- **WHEN** dua user berbeda mengubah profil dan tidak berbagi invariant guard
- **THEN** keduanya dapat memegang scope masing-masing tanpa serialisasi global yang tidak diperlukan

### Requirement: Busy is a conflict rather than a dependency error

Kegagalan memperoleh scope karena contention SHALL menghasilkan logical RESOURCE_BUSY dengan gRPC ABORTED pada catalog fitur terkait. Database unavailable, timeout, atau cancellation MUST mempertahankan semantics dependency/DEADLINE_EXCEEDED/CANCELLED yang tepat; kegagalan itu MUST tidak dilaporkan sebagai busy, duplicate, atau permission denial.

#### Scenario: Scope is held by another transaction

- **WHEN** scope belum diperoleh tetapi database menjawab dengan contention normal
- **THEN** caller menerima ABORTED/RESOURCE_BUSY, tidak ada business write, dan transaction ditutup tanpa retry mutation tersembunyi

#### Scenario: Lock operation fails because the database is unavailable

- **WHEN** pemeriksaan scope mengalami kegagalan dependency
- **THEN** caller menerima dependency error yang benar dan tidak dianggap memperoleh scope atau melakukan mutation

### Requirement: Exclusion lasts until transaction outcome is visible

Scope yang diperoleh SHALL tetap melindungi mutation sampai commit/rollback selesai. Failure/cancellation/panic cleanup SHALL membuat scope dapat diperoleh kembali dan MUST tidak mengembalikan koneksi ke pool dengan lock tertinggal.

#### Scenario: Another connection checks before commit

- **WHEN** transaksi pertama sudah menulis tetapi belum commit dan koneksi kedua mencoba scope sama
- **THEN** koneksi kedua tidak memperoleh scope; lock tidak dilepas sebelum mutation menjadi visible

#### Scenario: Transaction ends or aborts

- **WHEN** transaksi pertama commit, rollback, atau dibatalkan karena failure/panic/cancellation
- **THEN** transaksi baru pada koneksi berbeda dapat memperoleh scope, tidak melihat partial write, dan pool dapat digunakan kembali

### Requirement: Shared invariants use one deterministic lock protocol

Semua consumer fondasi yang melindungi invariant sama SHALL menggunakan canonical scope yang sama dan urutan multi-scope deterministik. Feature atau helper features/common SHALL menentukan scope/order dan memanggil TryLock pada handle request sebelum repository check/read/write; repository MUST NOT memperoleh lock secara mandiri. Duplikasi scope pada satu request SHALL tidak membuat operasi kedua pada owner berbeda. Foundation MUST tidak mengklaim last-admin, actor policy, atau refresh invariant sudah berjalan sebelum command consumer terkait diterapkan.

#### Scenario: Two consumers request an overlapping scope set

- **WHEN** dua consumer meminta kumpulan scope sama dalam urutan input berbeda
- **THEN** ordering canonical memberikan keputusan konsisten, tanpa deadlock karena reversed acquisition, dan semua scope bertahan sampai transaction selesai

#### Scenario: Target discovery changes the required scope set

- **WHEN** recheck menemukan scope tambahan yang mendahului kategori lock yang sudah dipegang
- **THEN** transaksi rollback dan diulang sebagai operasi baru yang terbatas, bukan mengambil predecessor lock dengan urutan terbalik atau menulis dengan state discovery lama
