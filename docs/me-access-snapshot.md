# Me: Profil dan Akses Pengguna Login

`/user.v1.UserService/Me` menerima `google.protobuf.Empty`. Caller memerlukan service credential melalui TLS, allowlist method Me, serta user JWT/session yang valid. Identitas user berasal dari principal; tidak ada input user ID atau application code dan tidak memerlukan permission admin.

## Data Response

Me membaca user/session, profil dan akses dalam satu primary read-only Repeatable Read snapshot. Field safe yang dikembalikan:

- `id`, `email`, `fullname`, `address`, `phone`, `country_code`, `created_at` dari waktu pembuatan user.
- `status`, `version` entity user dan `must_change_password`.
- `roles`: assignment/role/application ACTIVE pada namespace caller, dengan application access ACTIVE untuk aplikasi bisnis. Role tanpa mapping permission tetap dapat tampil.
- `application_access`: grant/application ACTIVE pada namespace caller, termasuk grant yang belum menghasilkan permission.
- `permissions`: `map<string, bool>` pada UserData tag 15. Setiap key yang hadir bernilai `true`; key tidak ada berarti tidak memiliki izin.

Role dan aplikasi diurutkan berdasarkan ID. Map tidak mempunyai urutan yang dijanjikan. Collection/map kosong dapat tidak tampil pada JSON sesuai marshaller; tidak ada permission default. Legacy `role_codes` numerik dan `role_names` tetap memakai mapping metadata existing dan tidak menjadi bukti izin atau pengganti collection scoped.

Restricted user (`must_change_password=true`) hanya memperoleh profil/status/version/created_at dan marker restriction, tanpa seluruh collection privilege lama maupun baru. Me melewati pembacaan privilege pada branch tersebut. User/session tidak aktif, revoked, expired atau auth_version tidak cocok tetap ditolak.

Contoh protobuf JSON dengan nama field proto (`UseProtoNames`), bukan contract envelope REST:

```json
{
  "id": "42",
  "email": "member@example.test",
  "fullname": "Member",
  "address": "Jakarta",
  "phone": "+6281234567890",
  "country_code": "+62",
  "created_at": "2026-10-01T00:00:00Z",
  "status": "ACTIVE",
  "version": "7",
  "role_codes": [1, 4],
  "role_names": ["Member", "Reviewer"],
  "roles": [{"role_id": "4", "code": "clm.reviewer", "name": "Reviewer", "status": "ACTIVE", "role_status": "ACTIVE"}],
  "application_access": [{"application_id": "2", "code": "clm", "name": "Claims", "status": "ACTIVE", "application_status": "ACTIVE"}],
  "permissions": {"clm.contract.read": true, "clm.contract.review": true}
}
```

## Phone Khusus Me

`Me.phone = country_code + phone` dari nilai tersimpan, dengan NULL country code menjadi string kosong. Concat literal ini berlaku juga untuk data lama dan restricted Me: `+62` + `0812` menjadi `+620812`, sedangkan `+62` + `+62812` menjadi `+62+62812`. Tidak ada normalisasi/penghapusan prefix/default/separator atau write database. `country_code` tetap dikirim terpisah.

Response profile/detail/list user mempertahankan phone existing. Consumer tidak melakukan concat ulang pada Me.phone. Perbaikan data lama merupakan pekerjaan terpisah.

## Penggunaan Permission

Permission merupakan union mapping aktif yang belum soft-deleted dan lolos namespace/application/access gates existing. Tidak ada privilege dari JWT role snapshot, nama role, numeric role code, wildcard, cache atau event. IAM self-service tidak memerlukan grant aplikasi IAM.

FE memakai map untuk menu/tombol. Server mengambil Me langsung dari grpc-auth untuk request saat ini dan menentukan permission route sendiri; map kiriman browser atau hasil Me lama tidak mengotorisasi aksi backend. `version` bukan versi kebijakan permission; perubahan role/app/mapping terlihat pada snapshot berikutnya dengan token yang masih valid.

Kegagalan pembacaan/finalisasi menggagalkan seluruh response, bukan map kosong sebagai fallback. Cancellation/deadline/panic memakai error boundary existing dan cleanup snapshot. Pembacaan tidak mengubah user/version/session, audit sukses, outbox atau idempotency result. RPC GetMyEffectivePermissions, GetUserEffectivePermissions dan EvaluateAccess tetap tersedia.

[Plan dan verifikasi](me-access-snapshot-plan.md) · [OpenSpec change](../openspec/changes/archive/2026-10-23-enrich-me-access-snapshot/proposal.md) · [Contract map](../../api-contracts/docs/me-access-contract.md).
