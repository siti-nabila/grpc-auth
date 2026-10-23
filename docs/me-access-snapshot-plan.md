# Pengayaan Me di grpc-auth

Tanggal: 22 Oktober 2026

Status: implementasi backend dan verifikasi local workspace selesai pada [enrich-me-access-snapshot](../openspec/changes/archive/2026-10-23-enrich-me-access-snapshot/proposal.md). Perilaku source didokumentasikan pada [Me snapshot](me-access-snapshot.md); hasil aktual dicatat pada [evidence](../openspec/changes/archive/2026-10-23-enrich-me-access-snapshot/evidence/progress.md). Publikasi contract/pin, implementasi consumer dan deployment tetap terpisah.

## 1. Ringkasan

Jadikan `/user.v1.UserService/Me` sebagai satu panggilan untuk memperoleh identitas, profil, status akun, role, akses aplikasi, dan permission pengguna yang sedang login.

Perubahan implementasi mencakup grpc-auth dan api-contracts. Implementasi consumer rest-orc dan CLM menjadi pekerjaan berikutnya.

## 2. Contract dan Implementasi Me

- Pertahankan request `google.protobuf.Empty`, nama RPC, serta seluruh field protobuf existing.
- Pertahankan RPC `GetMyEffectivePermissions`, `GetUserEffectivePermissions`, dan `EvaluateAccess` existing.
- Tambahkan `map<string, bool> permissions` pada `UserData`, field number `15`. Permission efektif bernilai `true`; key yang tidak ada dianggap tidak memiliki izin.
- Isi ID, email, fullname, address, phone, country_code, created_at, status, version, dan must_change_password.
- Isi `roles` dan `application_access` dengan data aktif dalam namespace service caller. Pertahankan legacy `role_codes` dan `role_names` untuk compatibility, tanpa menjadikannya dasar pemberian izin.
- Baca identitas, user/session, profil, role, aplikasi, dan permission dalam satu transaksi read-only snapshot. Gunakan permission reader dan resolver existing.
- Pertahankan seluruh gate assignment, role, aplikasi, mapping permission, dan application access existing.
- Restricted user menerima profil minimum tanpa collection privilege. User/session tidak aktif, revoked, atau expired tetap ditolak.
- Kegagalan dependency atau finalisasi snapshot menggagalkan seluruh response.

### Mapping Phone pada Response Me

```text
Me.phone = country_code + phone
```

Country code NULL diperlakukan sebagai string kosong. Concat dilakukan langsung di grpc-auth, termasuk untuk data lama.

| Country code tersimpan | Phone tersimpan | Hasil `Me.phone` |
| --- | --- | --- |
| `+62` | `81234567890` | `+6281234567890` |
| `+62` | `081234567890` | `+62081234567890` |
| `+62` | `+6281234567890` | `+62+6281234567890` |
| NULL | `+6281234567890` | `+6281234567890` |

Tidak ada normalisasi, penghapusan prefix, separator, atau default country code saat concat. Data lama akan diedit kemudian oleh user. Pembacaan Me tidak mengubah database.

Aturan concat hanya berlaku pada response Me. Mapping response profile, detail user, dan list user tetap existing, dan rest-orc tidak melakukan concat ulang.

## 3. Pemakaian oleh Consumer

- FE menggunakan permission map untuk menu dan tombol.
- rest-orc dan CLM memperoleh Me langsung dari grpc-auth menggunakan credential request yang sedang diproses.
- Permission route ditentukan server. Map kiriman browser atau hasil Me dari request sebelumnya tidak menjadi bukti izin.
- grpc-auth tetap memeriksa otorisasi setiap RPC protected; CLM tetap memeriksa aturan resource.
- `version` tetap versi entity user, bukan versi kebijakan permission.
- Tidak ada credential atau metadata sesi tambahan dalam response Me.

## 4. OpenSpec dan Verifikasi

Change `enrich-me-access-snapshot` di grpc-auth berisi proposal, design, delta spec `self-access-snapshot`, tasks lintas repository, dan evidence implementasi. Checklist hanya selesai setelah hasil verifikasi terkait tersedia; deployment/implementasi consumer tetap pekerjaan terpisah.

Acceptance test mencakup:

- Response lengkap dari satu snapshot dan pembatasan namespace caller.
- Permission duplikat/overlap serta revoked, disabled, dan soft-deleted sources.
- Restricted user, invalid session, dependency failure, cancellation, panic, dan cleanup.
- Perubahan akses terlihat pada panggilan Me berikutnya menggunakan token existing.
- Compatibility field protobuf lama dan roundtrip map baru.
- Seluruh contoh concat, termasuk NULL dan data lama, tanpa write database.

Jalankan generator api-contracts, targeted test, seluruh test dan vet kedua repo, regression test rest-orc, serta race test terarah. Uji PostgreSQL memakai suite opt-in pada database test khusus.

Verifikasi lokal memakai workspace. Publikasi contract dan pembaruan pin consumer dilakukan melalui permintaan terpisah; setelah versi tersedia, verifikasi grpc-auth dengan `GOWORK=off`.
