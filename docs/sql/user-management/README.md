# SQL user management

Skrip disusun dari DDL PostgreSQL existing yang diberikan user, untuk PostgreSQL
16+. Ini artefak migration/seed; belum dijalankan pada database. Tidak memasang
migration framework atau mengubah generated contract/application code.

## Urutan

1. `00_preflight.sql`: inspeksi schema/index, duplicate/orphan, dan kandidat admin.
2. `01_schema_backfill.sql`: sekali; ALTER/CREATE TABLE, backfill, constraint/index.
3. `02_seed_catalog.sql`: catalog IAM/CLM dan system role IAM; boleh diulang.
4. `03_clm_roles_seed_optional.sql`: role awal CLM ketika integrasinya dimulai.
5. `04_consumer_schema_optional.sql`: processed_events di database consumer.

Jalankan file dengan client yang berhenti pada SQL error; untuk psql gunakan
`-X -v ON_ERROR_STOP=1 -f <file>`. File 01 harus dijadwalkan saat write aplikasi
dihentikan; file tersebut memakai transaction/lock dan tidak memakai CREATE INDEX
CONCURRENTLY. Diperlukan izin migration serta CREATE EXTENSION untuk pg_trgm.
Jika pg_trgm existing berada di schema selain public, sesuaikan schema operator
class pada index; skrip tidak memindahkan extension.

## Backfill/seed per tabel

| Tabel | DML |
| --- | --- |
| auth | Non-deleted -> ACTIVE, deleted -> DEACTIVATED; counter mulai 1; password hash/email dipertahankan |
| profile | Tidak ada DML; data tetap, tambah unique user_id dan perbaiki FK |
| role | Code 1 -> iam.member; code 2 -> iam.platform_admin sesuai keputusan user; lainnya -> iam.legacy_ID |
| user_role | Assignment existing menjadi ACTIVE; tidak membuat assignment baru |
| user_profile_search | Tidak ada DML; tidak mengubah formula/trigger atau isi pencarian |
| applications | Seed iam dan clm |
| permissions | Seed 16 permission IAM dan 8 permission CLM |
| role_permissions | Admin: daftar IAM eksplisit; auditor: 4 read permission; member: kosong |
| user_application_access | Tidak ada seed akses; IAM self-service tidak memerlukan gerbang app access |
| sessions | Tidak ada seed; dibuat saat login setelah implementasi session-aware auth |
| refresh_tokens | Tidak ada seed; dibuat/dirotasi oleh auth service, hanya hash disimpan |
| audit_logs | Record perubahan role/mapping nyata ketika seed; tidak mengarang audit historis |
| outbox_events | Event perubahan role/mapping nyata ketika seed; relay mengirim setelah implementasi |
| idempotency_records | Tidak ada seed; diisi saat command idempotent committed |
| admin_guard | Satu row id=1 |
| processed_events | Tidak ada seed; diisi consumer setelah commit |

Member dan role legacy disimpan sebagai role sistem untuk menjaga compatibility
dan mencegah edit IAM lewat API custom business role. Role legacy selain numeric
2 tidak otomatis memperoleh permission baru. CLM permission/application catalog
boleh disiapkan sebelum CLM; tidak memberikan user akses atau membuat fitur CLM.

Waktu asli grant/role creation/password change tidak tersedia di schema lama;
kolom historis terkait dibiarkan NULL. Default timestamp berlaku pada record baru.
Timestamp auth existing tetap TIMESTAMP tanpa timezone: konversi historis ke
TIMESTAMPTZ memerlukan konfirmasi timezone sumber, sehingga tidak ditebak.

## Index existing

| Index | Tindakan |
| --- | --- |
| PK_auth, unique_key_email | Pertahankan; tidak menambah index email biasa |
| idx_id_email | Pertahankan; cek EXPLAIN sebelum menghapus karena bisa membantu index-only query id+email |
| profile_pk | Pertahankan |
| idx_user_id | Ganti dengan index milik UNIQUE profile(user_id); migration abort bila duplicate |
| user_id_pkey, uq_role_code | Pertahankan |
| idx_role_id, idx_role_code | Hapus yang redundant dengan PK/UNIQUE B-tree |
| PK_user_role, user_fk, role_fk | Pertahankan; tambah unique pair dan index akses menurut user/role/status |
| PK_user_profile_search, uq_profile | Pertahankan |
| idx_user_profile_search_fts_keyword | Ganti B-tree dengan GIN tsvector |
| idx_user_profile_search_fts_lexeme_text_trgm | Ganti B-tree dengan GIN gin_trgm_ops |

GIN menyediakan index operator yang cocok untuk full-text/trigram; penggunaan
index tetap bergantung pada SQL/operator aktual dan query plan. Penambahan index
baru hanya untuk kolom/pola query baru. IF NOT EXISTS berdasarkan nama tidak
membuktikan dua definisi index setara.
ORM existing menggunakan `@@ to_tsquery(...) OR ILIKE '%keyword%'`; dua GIN
tersebut cocok dengan operator yang digunakan. Keyword sangat pendek tetap bisa
memerlukan scan karena trigram yang dapat diekstrak terbatas.

## Batas skrip

Duplicate/orphan tidak dihapus otomatis. Profile FK berubah SET NULL -> RESTRICT;
search FK berubah SET NULL -> CASCADE karena search merupakan turunan profil.
Tidak ada query untuk hard-delete user/profil. Tabel baru memakai INTEGER untuk
FK auth/role existing; UUID session/token/event disediakan aplikasi, dan JSONB
dipakai untuk snapshot/event aman. Password column diperlebar VARCHAR(100) ->
TEXT agar format hash tidak terpotong.

Schema sendiri belum menerapkan permission enforcement. Implementasi service
masih harus memvalidasi aplikasi yang sama untuk role/permission, transaksi
grant/revoke, admin terakhir dengan admin_guard FOR UPDATE, expected version,
idempotency, session/token binding, refresh rotation, dan restricted-password
session. Perubahan status/seed perlu dikoordinasikan dengan implementasi itu.

Hasil seed `active_platform_admins` harus >=1 sebelum enforcement diaktifkan.
Jika nol, bootstrap user/assignment harus memilih identitas eksplisit; skrip
tidak mempromosikan user pertama. Access token lama tidak dipindahkan ke sessions;
cutover meminta login ulang. Audit/outbox seed menjadi no-op pada pengulangan
yang tidak mengubah state. Role CLM existing tidak direset oleh seed opsional.
