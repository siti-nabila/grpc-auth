# Proposal

## Why

Consumer saat ini belum memperoleh profil dan akses pengguna login yang lengkap dari satu `Me`; response belum mengisi detail role/aplikasi serta permission map. Pengayaan ini menyediakan satu view authoritative dari snapshot yang sama sehingga consumer tidak perlu mengagregasi RPC permission terpisah.

## What Changes

- Perluas `/user.v1.UserService/Me(google.protobuf.Empty)` untuk mengisi safe identity/profile, `created_at`, status, entity version, restricted marker, role aktif dan akses aplikasi aktif dalam namespace service caller.
- Tambahkan `map<string, bool> permissions = 15` pada `user.v1.UserData` di api-contracts. Isi union permission efektif dengan nilai `true`; key tidak ada berarti tidak memiliki izin.
- Gunakan session recheck, permission reader dan resolver existing dalam satu transaksi primary read-only Repeatable Read milik feature; tidak mengagregasi public RPC atau membuka snapshot tambahan untuk pengayaan.
- Pertahankan restricted Me minimum, gate otorisasi existing, legacy role fields, semua nama RPC dan tag/type protobuf existing. `GetMyEffectivePermissions`, `GetUserEffectivePermissions`, dan `EvaluateAccess` tetap tersedia.
- **BREAKING (semantics `Me.phone`, disetujui pada plan):** response Me menjadi concat literal `country_code + phone`, dengan NULL country code menjadi string kosong. Bentuk wire tetap string field 5; data legacy dapat menghasilkan prefix berulang. Response profile/detail/list user dan nilai tersimpan tetap existing.
- Kegagalan dependency/finalisasi tidak boleh menghasilkan partial success atau map kosong sebagai fallback. Perubahan akses committed terlihat pada snapshot Me berikutnya dengan credential yang masih valid.

## Capabilities

### New Capabilities

- `self-access-snapshot`: profil dan privilege pengguna login melalui satu Me, contract map permission, namespace filtering, restricted behavior, phone concat, snapshot/failure semantics, dan verification criteria.

### Modified Capabilities

Tidak ada delta requirement pada RPC permission existing, authentication, atau authorization. Capability baru memperinci pengayaan Me yang disetujui; guard dan policy existing tetap menjadi prerequisite.

## Impact

| Scope | Owner / area | Prerequisite dan batas |
| --- | --- | --- |
| MVP_REQUIRED | grpc-auth: session read handler, auth feature snapshot, safe identity/access readers dan tests | Contract permission tersedia; pertahankan authentication/namespace/transaction/error gates |
| EXTERNAL_REPOSITORY | api-contracts: `proto/user/v1/user.payload.proto`, generated descriptors, compatibility/roundtrip tests | Tambahan tag 15 dan generator dilakukan di repository contract pada tahap implementasi yang diotorisasi; tidak membuat protobuf lokal |
| EXTERNAL_REPOSITORY follow-up | rest-orc dan CLM consumer | Mapping/validasi dari Me serta pencegahan concat ulang; implementasi consumer tidak termasuk change backend ini |
| POST_MVP / di luar change | UI, CLM resource policy/integrasi lanjutan, caching akses, broker/relay | Tidak menjadi dependency implementasi pengayaan Me |

Tidak ada RPC, konfigurasi runtime, migration, tabel, mutation, idempotency, atau dependency ORM baru. Publikasi contract/pembaruan pin consumer merupakan permintaan terpisah; validasi `GOWORK=off` menunggu versi published yang memuat tag 15. Kesiapan deployment lintas service tetap mandatory Phase 6 DEP-01, bukan bukti completion backend.

Acuan keputusan: [plan Me](../../../../docs/me-access-snapshot-plan.md). Hasil implementasi dan verifikasi scoped dicatat pada [evidence](evidence/progress.md); publication/deployment tidak disimpulkan dari validasi proposal.
