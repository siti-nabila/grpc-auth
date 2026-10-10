# PostgreSQL foundation verification

Suite ini opt-in dengan build tag `postgres`; default `go test ./...` tidak
memilih package atau membuka database suite. Semua source/helper khusus suite
memiliki tag yang sama. Runner memakai testutils.Run dan scenario .go biasa.

## Prerequisites

User/CI menyediakan **database test khusus** terpisah dari database aplikasi.
Database aplikasi project bernama `user` secara eksplisit ditolak, bersama
system databases postgres/template0/template1. Suite tidak membuat database,
menjalankan Docker atau mengambil koneksi dari env.yaml. Jangan menggunakan
kredensial produksi atau menulis DSN/password dalam repository/report/chat.

Isi environment proses test secara privat:

- `GRPC_AUTH_POSTGRES_TEST_ENABLED=1` (nilai lain ditolak).
- `GRPC_AUTH_POSTGRES_TEST_DSN`: PostgreSQL DSN database test khusus.
- `GRPC_AUTH_POSTGRES_TEST_DATABASE`: nama database test yang harus sama persis
  dengan current_database(). Nama ini tidak boleh database aplikasi.

## Commands

Guard-only verification tanpa environment/network/database:

```sh
rtk proxy go test -tags=postgres ./tests/integration/postgres -run '^TestPostgresConfiguration$'
```

Readiness pada database test yang telah disediakan dan dipilih eksplisit:

```sh
rtk proxy go test -tags=postgres ./tests/integration/postgres
```

Jika configuration tidak lengkap, tidak cocok atau database tidak terjangkau,
requested readiness test **FAIL**, bukan SKIP/PASS; verification gate BLOCKED.
DSN wajib menyebut nama database test yang sama secara eksplisit (keyword atau URI); dbname hilang, berbeda atau override duplicate ke database aplikasi ditolak sebelum koneksi.
Koneksi memeriksa identity melalui transaksi READ ONLY sebelum pool diberikan
ke consumer, dan pool ditutup pada failure/cleanup. Pesan tidak memuat DSN,
credential atau detail server. Dua pool slots disediakan. Harness memverifikasi pg_backend_pid berbeda pada
koneksi A/B, scope sama busy dan scope berbeda tetap berjalan, serta acquisition
setelah commit dan acknowledged cancellation/rollback melalui channel/barrier.
Tidak ada sleep yang dipakai sebagai proof.

## Current coverage and remaining gates

Suite saat ini mencakup guard/readiness, schema fixture disposable, synthetic
user/profile/member assignment, defaults/constraints yang dicocokkan ke metadata
read-only, replacement connection, namespace collision/setup cancellation,
cleanup pool/schema dan interleaving dua physical connections. Seed hash berupa
marker yang tidak dapat dipakai login; tidak menyalin akun atau credential aplikasi.

Setiap schema memakai prefix grpc_auth_test_ dan nama unik process/counter.
CREATE dan fixture DDL berada dalam satu transaksi; schema existing tidak diadopsi
atau dihapus saat setup gagal. search_path dikirim sebagai startup option DSN
untuk setiap physical/replacement connection; public tidak menjadi fallback.
Cleanup memakai ownership privat, memverifikasi kembali database test dan hanya
menghapus namespace fixture miliknya. Database test disediakan terpisah di luar
suite; suite tidak membuat/drop database atau menjalankan migration/seed produksi.

Pengujian race pada test database yang dipilih eksplisit:

```sh
rtk proxy go test -race -tags=postgres ./tests/integration/postgres
```

Production lifecycle scenarios memakai BeginTx(requestCtx), owner feature,
bound repositories serta ORM v1.7.0 untuk commit/rollback/operation failure/panic/
automatic cancellation, lock release dan pool reuse. Register membuktikan
canonical-email contention, sequential partial-failure rollback dan response
setelah commit; profile membuktikan same-user exclusion, independent users,
owner/response identity dan full replacement.

Persistence consumer fixture menguji mutation/audit/outbox/safe-result atomicity,
actual FK/PK failure rollback, pending event, conflict/replay sebelum stale version,
current permission sebelum replay, different target, concurrent claims, lost
response, fixed 24-hour expiry replacement serta NO_OP tanpa event/audit kedua.
Security fixture commit status/auth_version + required audit/event lalu denial;
failure sebelum commit rollback semuanya. Tidak ada broker, refresh RPC/session
store atau general idempotency pada Register/self-profile.

F-01..F-12 hanya criterion fondasi scoped pada
[evidence ledger](../../../openspec/changes/auth-transaction-foundation/evidence.md).
Real RefreshSession, last-admin dan production privileged command wiring belum
dibuktikan. Fixture tidak menandai seluruh source AC atau MVP PASS.

Evidence menyertakan commit/module/PostgreSQL version, waktu, command tanpa
secret dan expected/actual result. Database test belum tersedia membuat required
PostgreSQL execution gates BLOCKED; mock/config tests tidak menggantikannya.
