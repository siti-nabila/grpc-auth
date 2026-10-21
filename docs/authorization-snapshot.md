# Primary read snapshot

`SessionReadHandler` menghubungkan Me/ListUsers ke `authfeature.ReadUserDataSnapshot` dan `user.ReadUsersSnapshot` memakai immutable principal claims, verified caller dan current snapshot state. Real production Login/EdDSA producer sudah tersedia dan dipakai oleh integration tests. Default service registration beralih bersama seluruh pipeline setelah readiness gate 7.2–7.4 PASS; tidak ada fallback atau partial runtime mode. Lihat [consumer matrix](session-foundation-consumers.md).

## Isolation dan ownership

ORM published v1.7.0 `orm/query_page.go:101` menggunakan `QueryPageWithConfig` → `query/pagination.go:71` `ScanPaginate`. Implementasi menjalankan `buildCount`/`ScanQuery`, kemudian `scanPaginateBuilder`/`ScanQuery` untuk data. Count dan data merupakan dua statement. Existing filter/search/column whitelist serta cursor `auth_id`/`last_id` bergantung pada path tersebut. Menggabungkan actor/permission ke salah satu statement saja tidak membuat keduanya memakai snapshot sama.

Karena itu digunakan alternatif yang sudah diizinkan design: `domain.ReadSnapshotProvider.BeginSnapshot(ctx)` membuka transaction **RepeatableRead READ ONLY** melalui reusable primary pool. Feature memegang lifecycle dengan `TransactionOwner`, commit setelah seluruh read sukses, rollback saat denial/failure, dan membuang partial response. Mutation `Begin(ctx)` tetap **ReadCommitted**, tanpa perubahan isolation global. Repository menerima handle/context dan tidak memperoleh pool baru.

Urutan Me: verified service → begin snapshot → joined actor/session state → current principal/restricted check → safe identity → self profile → active legacy roles hanya jika nonrestricted → commit. Owner berasal dari verified `sub`; forwarded `user_id` tidak digunakan. Restricted response memuat identity/profile/status/version/must_change_password tanpa privilege collections. Reader identity tidak mengambil password/hash/credential.

Urutan ListUsers: verified service + namespace IAM → begin snapshot → actor/session state → restricted check → current permission contributions → exact `iam.user.read` → count → page data → commit. Authorization selalu dievaluasi walaupun hasil halaman kosong. Denial berhenti sebelum count/data. Seluruh SQL memakai handle snapshot yang sama.

## Permission resolution

Resolver mempertahankan union permission dan contributing role IDs/codes, diurutkan deterministik dan dideduplikasi. Assignment/role/application harus ACTIVE; application role dan permission harus sama; namespace permission harus sesuai application. Business application juga membutuhkan user application access ACTIVE. IAM self-service tidak memanggil permission resolver sehingga tidak memerlukan admin role atau application grant. Numeric admin code, wildcard dan JWT role tidak menjadi grant.

## Evidence dan batas

`tests/integration/auth_reads` memakai production provider, ORM adapter, reader, verifier dan feature read components dengan controlled SQL driver. Suite memeriksa satu begin/commit, RR/read-only options, request context, exact actor arguments, query order, restricted privilege omission, empty page, denial, query failures dan failed commit tanpa partial success. `tests/features/common` menguji union/partial revoke/inactive/cross-app gates; `tests/repositories/transaction` menguji join/mapping dan handle.

Driver tests membuktikan SQL/configuration/ownership, bukan PostgreSQL MVCC interleaving. Dedicated opt-in PostgreSQL proof dan real Login consumer integration kini tersedia di `tests/integration/postgres`: channel barriers dan physical backend IDs membuktikan snapshot konsisten serta role/app change committed terlihat pada request berikutnya tanpa session/auth_version change. Protected default-server cutover tetap menunggu readiness gate. Expanded management role/application outputs dan RPC tetap follow-up. Perubahan privilege yang committed sebelum snapshot berikutnya harus terlihat tanpa auth_version/session change; request yang sudah memakai snapshot valid tidak dijanjikan cancellation in-flight.
