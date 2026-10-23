# Plan Keyword Pagination per Feature

Status: draft implementasi. Dokumen ini belum menjalankan perubahan database, code, atau contract.

## 1. Tujuan

Menyediakan pencarian keyword full-text/trigram untuk list role, permission, application, dan assignment role user menggunakan tabel turunan seperti `user_profile_search`.

grpc-auth mencari keyword melalui `fts_keyword` atau `fts_lexeme_text` pada tabel search masing-masing feature, seperti `ListUsers`. FE cukup mengirim keyword, misalnya `reviewer`; FE tidak perlu mengetahui nama kolom sumber atau sintaks full-text PostgreSQL.

Function/trigger database menjadi pemilik komposisi dokumen search. grpc-auth tidak memiliki whitelist isi keyword berdasarkan nama, email, phone, atau field bisnis lain. Jika sumber dokumen diperluas, grpc-auth tetap menerima keyword baru tanpa perubahan code, selama struktur tabel search dan contract pencarian tetap sama.

### Sumber Awal Keyword Pagination per Feature

| Feature | RPC | Sumber keyword yang diinginkan | Kolom fisik saat ini |
| --- | --- | --- | --- |
| Users | `ListUsers` | `user.phone`, `user.email`, `user.name` | `profile.phone`, `auth.email`, `profile.name` |
| Role | `ListRoles` | `role.name`, `role.code` | `role.role_name`, `role.code` |
| Permission | `ListPermissions` | `permission.code` | `permissions.code` |
| Application | `ListApplications` | `application.code`, `application.name` | `applications.code`, `applications.name` |
| Assignment | `ListUserRoles` | `role.name`, contoh `reviewer` | `role.role_name`, melalui `user_role.role_id` |

Matriks ini menentukan komposisi awal function database, bukan batas field yang di-hard-code pada grpc-auth. Pada formula awal, address, description permission, kode aplikasi milik role, serta role code pada assignment tidak dimasukkan. Field tersebut dapat ditambahkan kemudian lewat function/trigger dan rebuild data search yang terkait.

Nama seperti `user.name` merupakan nama logis untuk menjelaskan sumber dokumen. grpc-auth hanya memetakan feature ke tabel search, key join, `fts_keyword`, dan `fts_lexeme_text`. Kolom yang disimpan oleh function menentukan cakupan pencarian aktual.

`ListUserApplicationAccess` merupakan fitur berbeda dari assignment role user. Perubahan pencariannya tidak termasuk scope awal ini.

## 2. Kondisi Existing

- List users membaca `auth`, `profile`, dan `user_profile_search`. Reader memakai `ups.fts_keyword` untuk full-text serta `ups.fts_lexeme_text` untuk pencarian substring.
- Handler users saat ini memilih `full_text_trigram` dan logical field `keyword`. Reader menjalankan kombinasi full-text prefix dan `ILIKE`, bukan pencarian kemiripan dengan toleransi typo.
- List role, permission, application, dan assignment memakai jalur catalog bersama. `Catalog.normalizeSearch` saat ini menerima `contains`/`prefix`, melakukan trim, dan reader memakai `ILIKE` dengan karakter wildcard di-escape.
- Catalog saat ini mempunyai cakupan pencarian lebih luas daripada matriks keyword baru, misalnya description permission dan role code pada assignment.
- Enum protobuf `FULL_TEXT` dan `FULL_TEXT_TRIGRAM` sudah tersedia. Tidak perlu RPC baru atau mengedit generated protobuf untuk memakai mode tersebut.
- SQL user management existing memperbaiki index `user_profile_search`, tetapi tidak mendefinisikan ulang formula/trigger pencariannya. Fixture PostgreSQL hanya mengisi nama; fixture tersebut belum membuktikan bahwa data search aktual berisi email dan phone.

### Baseline Function dan Trigger Users dari User

Definisi yang diberikan user sudah mencakup email, name, phone, serta phone versi digit-only. Ini menjadi baseline rancangan, bukan asumsi bahwa function belum tersedia:

| Trigger | Event | Function wrapper |
| --- | --- | --- |
| `auth_refresh_user_profile_search_after_email_update` | `AFTER UPDATE OF email ON public.auth`, hanya jika email lama dan baru berbeda | `trg_auth_refresh_user_profile_search()` |
| `profile_refresh_user_profile_search_after_insert_update` | `AFTER INSERT OR UPDATE OF user_id, name, phone ON public.profile` | `trg_profile_refresh_user_profile_search()` |

Function refresh yang diberikan user menggunakan pola berikut:

```sql
CREATE FUNCTION public.refresh_user_profile_search(IN p_profile_id BIGINT)
RETURNS VOID LANGUAGE PLPGSQL AS $$
BEGIN
    INSERT INTO user_profile_search (profile_id, fts_keyword, fts_lexeme_text)
    SELECT
        src.profile_id,
        src.fts_keyword,
        array_to_string(tsvector_to_array(src.fts_keyword), ' ')
    FROM (
        SELECT
            p.id AS profile_id,
            to_tsvector(
                'simple',
                concat_ws(
                    ' ', a.email, p."name", p.phone,
                    regexp_replace(coalesce(p.phone, ''), '\D', '', 'g')
                )
            ) AS fts_keyword
        FROM profile p
        JOIN auth a ON a.id = p.user_id
        WHERE p.id = p_profile_id
    ) src
    ON CONFLICT (profile_id) DO UPDATE SET
        fts_keyword = EXCLUDED.fts_keyword,
        fts_lexeme_text = EXCLUDED.fts_lexeme_text;
END;
$$;
```

Potongan ini adalah dokumentasi baseline, bukan script migration yang dijalankan oleh task ini. Definisi kedua wrapper belum diberikan; verifikasi bahwa wrapper memanggil refresh untuk semua profile yang terpengaruh, termasuk saat `profile.user_id` berubah.

Sebelum implementasi, inspeksi definisi tabel, function, trigger, index, dan versi PostgreSQL aktual. Graph digunakan untuk routing awal; source working tree tetap menjadi acuan karena sudah ada perubahan lokal.

## 3. Pemisah dan Pembentukan Keyword

`fts_keyword` menyimpan `tsvector` hasil tokenisasi, bukan simbol pemisah. Function database mengatur pemisahan `.`, `-`, dan spasi saat membentuk dokumen search. grpc-auth tidak mengambil field sumber lalu merakit dokumen sendiri.

### Target Tokenisasi: Simbol Setara Spasi

Ketentuan ini berlaku khusus untuk **role, permission, application, dan assignment**. Titik (`.`), tanda hubung (`-`), serta whitespace dianggap setara dengan satu spasi. Setiap bagian menjadi lexeme tersendiri di `fts_keyword`, tanpa menyimpan bentuk gabungan seperti `iam.role.create` sebagai satu token. Formula list users yang diberikan user tetap dipertahankan.

Contoh target berikut berlaku saat satu nilai sumber diproses:

| Nilai sumber | Setara dengan input | Target `fts_keyword` |
| --- | --- | --- |
| `iam.role.create` | `iam role create` | `'create':3 'iam':1 'role':2` |
| `iam auditor` | `iam auditor` | `'auditor':2 'iam':1` |
| `application-clm` | `application clm` | `'application':1 'clm':2` |
| `iam.-role  create` | `iam role create` | `'create':3 'iam':1 'role':2` |

Angka setelah `:` menunjukkan posisi token pada dokumen hasil normalisasi. Urutan tampilan lexeme bukan urutan kata sumber; misalnya `create` dapat tampil lebih dahulu meskipun posisinya ketiga.

`fts_lexeme_text` tetap diisi dari `array_to_string(tsvector_to_array(fts_keyword), ' ')`. Pemisahan simbol sudah selesai ketika vector dibentuk; hasil array cukup digabung memakai spasi. [Acuan fungsi array PostgreSQL](https://www.postgresql.org/docs/16/functions-array.html).

Untuk catalog, perlakukan simbol pemisah seperti whitespace **sebelum** membentuk vector:

```text
Dokumen sumber: clm.document-read
  -> normalisasi pemisah di function database: clm document read
  -> to_tsvector('simple', ...): lexeme clm, document, read
  -> tsvector_to_array(fts_keyword)
  -> array_to_string(..., ' '): teks lexeme dengan satu spasi
```

Contoh ekspresi normalisasi pada function database:

```sql
to_tsvector(
    'simple',
    regexp_replace(coalesce(raw_document, ''), '[.[:space:]-]+', ' ', 'g')
)
```

`raw_document` berarti hasil penggabungan field sumber oleh refresh function. Pola contoh tersebut menyamakan titik, hyphen, dan whitespace sebagai pemisah. Daftar simbol tambahan dikelola di function normalisasi database; jangan otomatis membuang semua punctuation karena email, phone, atau kode dapat mempunyai aturan berbeda.

Gunakan helper database dengan signature tetap untuk normalisasi dokumen catalog dan input query catalog. Function refresh memakainya sebelum `to_tsvector`; jalur query memakainya sebelum membentuk full-text query maupun pola substring. Dengan demikian `document.read`, `document-read`, dan `document read` memperoleh pemisahan token yang sama tanpa daftar simbol di Go.

`array_to_string(..., ' ')` tetap digunakan pada semua feature. Bila daftar simbol di helper berubah, rebuild dokumen search dan uji ulang query. Baseline users tidak diubah otomatis; normalisasi catalog dipisahkan dari aturan email/phone users.

Target tokenisasi untuk dokumen catalog baru:

| Input sumber | Bentuk internal untuk search | Token target |
| --- | --- | --- |
| `clm.document.read` | `clm document read` | `clm`, `document`, `read` |
| `senior-reviewer` | `senior reviewer` | `senior`, `reviewer` |
| `Senior Reviewer` | `senior reviewer` | `senior`, `reviewer` |
| `senior.-reviewer` | `senior reviewer` | `senior`, `reviewer` |

Ketentuan yang direncanakan:

1. Raw keyword dari FE diterima tanpa trim di boundary REST. Data sumber dan nilai response tidak diubah oleh proses search.
2. Function pembentuk dokumen mengatur kapitalisasi/pemisah lalu menggunakan `to_tsvector('simple', ...)`. Perlakuan titik/hyphen pada catalog harus diuji; jangan mengubah formula users existing secara otomatis karena email/phone memerlukan perilaku berbeda.
3. Pemrosesan input query harus kompatibel dengan dokumen yang disimpan. Catalog memakai helper normalisasi database yang sama pada dokumen dan input query, dengan nama/signature tetap melalui ORM. Parsing/escaping tidak memeriksa apakah token berasal dari field bisnis yang dikenal Go; komposisi sumber tetap berada pada refresh function.
4. Function menggabungkan field sumber dengan spasi dan menangani nilai nullable. Setelah `fts_keyword` terbentuk, isi `fts_lexeme_text = array_to_string(tsvector_to_array(fts_keyword), ' ')`, mengikuti baseline users. Jangan menyimpan concat raw sebagai formula berbeda pada setiap feature.
5. Simbol selain pemisah tersebut tidak menjadi operator pencarian. `%`, `_`, backslash, tanda kutip, `&`, `!`, dan tanda kurung harus ditangani sebagai input literal dan tidak menyebabkan error parser atau memperluas wildcard.
6. Keyword omitted/kosong/whitespace-only mengikuti perilaku tanpa filter existing. Jika keyword nonempty tidak menghasilkan lexeme, cabang full-text tidak cocok; cabang text tetap boleh mencocokkan substring literal pada mode gabungan. Bila kedua cabang tidak cocok, hasil kosong, bukan list tanpa filter.
7. Pertahankan batas keyword existing per feature. Untuk catalog, batas existing adalah 200 karakter Unicode setelah normalisasi; validasi raw input juga dibatasi agar input berlebih tidak lolos hanya karena tokenisasinya menjadi pendek. Batas users diverifikasi dari source saat implementasi, tidak disamakan secara diam-diam.

Email dan phone pada users memerlukan regression test tersendiri: `@`, `+`, underscore, serta email lengkap harus tetap dapat dicari sesuai perilaku existing. Jangan berasumsi parser bawaan PostgreSQL selalu memisahkan email atau kode bertitik menjadi kata yang diinginkan. [Acuan parser PostgreSQL](https://www.postgresql.org/docs/16/textsearch-parsers.html).

`fts_lexeme_text` berasal dari daftar lexeme, bukan kalimat sumber. Jangan mengasumsikan urutan lexeme mempertahankan frasa raw. Uji pencarian multi-token pada cabang full-text dan substring secara terpisah; kecocokan pada salah satu cabang cukup, dengan semantics operator masing-masing.

## 4. Desain Tabel Search

Buat empat tabel turunan. Masing-masing memiliki satu baris per record sumber, sehingga join tidak menggandakan item list.

| Tabel search | Primary key sekaligus foreign key | Sumber dokumen |
| --- | --- | --- |
| `role_search` | `role_id INTEGER` → `role.id` | `role.role_name` + `role.code` |
| `permission_search` | `permission_id INTEGER` → `permissions.id` | `permissions.code` |
| `application_search` | `application_id INTEGER` → `applications.id` | `applications.code` + `applications.name` |
| `user_role_search` | `user_role_id INTEGER` → `user_role.id` | Nama role dari `user_role.role_id` → `role.role_name` |

Kolom tambahan yang sama pada keempat tabel:

| Kolom | Tipe | Kegunaan |
| --- | --- | --- |
| `fts_keyword` | `TSVECTOR NOT NULL` | Dokumen full-text dari sumber yang telah ditokenisasi |
| `fts_lexeme_text` | `TEXT NOT NULL` | `array_to_string(tsvector_to_array(fts_keyword), ' ')` untuk substring/trigram |

- Gunakan PK/FK yang sama tanpa sequence atau surrogate ID tambahan. FK memakai `ON DELETE CASCADE` karena data search merupakan turunan; ini tidak menambahkan kemampuan hard-delete ke API.
- Buat GIN index pada `fts_keyword` dan GIN `gin_trgm_ops` pada `fts_lexeme_text`. Periksa extension `pg_trgm` sebelum migration.
- Tabel search tidak menyimpan password, token, status otorisasi, atau keputusan permission. Status, akses aplikasi, dan identitas user tetap dibaca dari tabel authoritative.
- Tidak perlu tabel `assignment` baru. Record assignment existing berada pada `user_role`; `user_role_search` hanya proyeksi search-nya.
- Pisahkan search assignment dari `role_search` agar formula awal assignment hanya berisi nama role, sedangkan role search juga memuat kode. Ini aturan function awal, bukan larangan di grpc-auth bila function assignment kelak sengaja diperluas.
- Konsekuensi tabel per assignment adalah penyimpanan nama berulang dan pembaruan banyak baris ketika nama role berubah. Ukur biaya ini pada role dengan banyak assignment sebelum rollout.

GIN trigram dapat mendukung `ILIKE`, tetapi index tidak menjamin seluruh pencarian lebih cepat. Keyword sangat pendek dapat membutuhkan scan; verifikasi dengan data yang mewakili volume aktual. [Acuan pg_trgm](https://www.postgresql.org/docs/16/pgtrgm.html#PGTRGM-INDEX).

## 5. Sinkronisasi dan Backfill

Gunakan trigger database untuk menjaga tabel turunan tetap sinkron dalam transaksi yang sama dengan perubahan sumber. Pilihan ini mencakup perubahan dari service, seed, dan proses administrasi database tanpa menambahkan write search pada setiap use case.

Setiap feature memiliki refresh function dengan pola UPSERT yang sama seperti `refresh_user_profile_search`: baca sumber, hitung `fts_keyword`, turunkan `fts_lexeme_text` dari vector tersebut, lalu upsert berdasarkan key sumber. Trigger memanggil function ini; grpc-auth tidak mengisi ulang kolom search pada request list maupun mutation aplikasi.

| Perubahan sumber | Sinkronisasi |
| --- | --- |
| Insert/update nama atau kode role | Upsert `role_search` |
| Update nama role | Perbarui `user_role_search` untuk seluruh assignment role tersebut, termasuk yang sudah revoked |
| Insert assignment user-role | Isi `user_role_search` dari nama role saat itu |
| Perubahan `user_role.role_id`, bila dilakukan proses administrasi | Bangun ulang search assignment tersebut |
| Grant ulang/revoke dengan role yang sama | Pertahankan search; status difilter dari `user_role` |
| Insert/update kode permission | Upsert `permission_search` |
| Insert/update kode atau nama application | Upsert `application_search` |
| Delete record sumber | FK cascade menghapus search turunannya |

Aturan transaksi dan migrasi:

- Trigger hanya berjalan untuk perubahan field sumber yang relevan. Status, version, atau reason saja tidak memerlukan rebuild keyword.
- Kegagalan trigger menggagalkan transaksi sumber; jangan menyimpan perubahan sumber dengan search yang tertinggal.
- Role rename dan grant/revoke concurrent harus diuji. Tentukan urutan locking yang konsisten dengan transaksi assignment existing agar nama lama tidak mengalahkan nama baru dan deadlock tidak bertambah tanpa analisis.
- Pembaruan proyeksi tidak menambah user version, role version, audit bisnis, atau idempotency record tersendiri. Perubahan entity sumber tetap mengikuti aturan existing.
- Buat script migration baru yang berurutan: preflight, schema/function/trigger, backfill, index, verifikasi, dan rollback. Jangan menulis ulang migration user management yang sudah digunakan.
- Saat memasang trigger dan backfill, gunakan batas locking singkat yang terukur untuk mencegah write concurrent meninggalkan proyeksi lama. Untuk data besar, rancang batch dan verifikasi ulang dengan trigger aktif; index concurrent dijalankan terpisah dari transaction block bila dipilih.
- Backfill harus dapat dilanjutkan secara aman, mencakup data ACTIVE maupun REVOKED/DISABLED, serta memverifikasi missing row, orphan, duplikasi, dan isi dokumen. Jangan mencatat email/phone atau raw keyword pengguna ke log migration.
- Pertahankan baseline users yang diberikan: email/name/phone/digit-only phone sudah masuk formula. Verifikasi kesesuaian database dengan definisi tersebut dan selaraskan fixture test. Ubah function users hanya bila ada kebutuhan tokenisasi tambahan yang eksplisit; pertahankan PK/FK existing.

### Menambah Sumber Keyword tanpa Mengubah grpc-auth

Contoh: permission awalnya hanya memuat `permissions.code`, kemudian ingin memasukkan `permissions.description`.

1. Ubah refresh function permission agar `fts_keyword` juga memuat description. `fts_lexeme_text` tetap diturunkan dari vector yang sama.
2. Sesuaikan trigger `UPDATE OF` dan dependensi antar tabel agar perubahan description ikut memanggil refresh. Mengubah function saja tidak cukup jika field baru belum memicu trigger.
3. Rebuild/backfill row permission existing; penggantian function tidak otomatis memperbarui row search lama.
4. Verifikasi keyword yang hanya ada pada description dapat ditemukan dengan request dan binary grpc-auth yang sama. Tidak perlu menambah whitelist sumber keyword, protobuf field, atau mapping kolom baru di Go.
5. Perbarui dokumentasi sumber dan test database. Jangan memasukkan field sensitif atau field di luar cakupan data yang boleh ditemukan caller.

Perubahan tabel search, nama kolom, key join, konfigurasi bahasa, atau semantics query tetap perlu dianalisis tersendiri. Jaminan tanpa perubahan code berlaku untuk perluasan isi dokumen dengan contract dua kolom search yang tetap.

## 6. Integrasi Query grpc-auth

### Contract Search

Gunakan `PageQuery.search` existing dengan bentuk logis:

```json
{
  "search": {
    "fields": ["keyword"],
    "keyword": "senior-reviewer",
    "mode": "SEARCH_MODE_FULL_TEXT_TRIGRAM"
  }
}
```

Ini contoh payload protobuf JSON, bukan endpoint HTTP baru. rest-orc nantinya memetakan query REST `keyword=<teks>` ke bentuk tersebut dan menentukan mode di server.

- Tambahkan logical search field `keyword` pada empat catalog dalam scope, terpisah dari whitelist projection/sort. Nama ini bukan kolom response atau kolom yang boleh dipakai sort.
- Full-text/trigram hanya memakai logical field `keyword`; grpc-auth selalu membaca dua kolom search, sedangkan function database memilih sumber awal maupun tambahan. Tolak kombinasi `keyword` dengan physical fields yang membuat cakupan ambigu; keyword value tidak dibatasi pada field/nilai yang dikenal Go.
- Pertahankan `contains`/`prefix` dan explicit search fields existing sebagai jalur compatibility. Jalur keyword baru mencari isi dokumen search aktual; matriks awal bukan filter tambahan dalam code.
- Jangan mengganti default protobuf mode yang belum diisi pada caller lama secara global. rest-orc memilih `FULL_TEXT_TRIGRAM` secara eksplisit. Migrasi caller lama ke jalur keyword didokumentasikan; menghapus jalur compatibility merupakan pekerjaan terpisah.
- Dukung `FULL_TEXT` secara eksplisit pada jalur keyword; gunakan query parser aman untuk teks biasa. Tanda `-` bermakna pemisah menurut plan ini, bukan operator NOT.
- Cek contract source dan handler mapping; enum yang dibutuhkan sudah ada. Tidak menambah dependency atau memperbarui module contract dalam pekerjaan plan ini.

### Predicate dan Pagination

1. Reader menggabungkan tabel search sesuai catalog hanya saat ada keyword aktif.
2. Untuk mode `FULL_TEXT_TRIGRAM`, bentuk predicate setara `ListUsers`: **`fts_keyword` cocok dengan full-text prefix seluruh token memakai AND**, **atau `fts_lexeme_text` cocok dengan substring keyword**. Tidak mensyaratkan keduanya cocok. Mode `FULL_TEXT` hanya memakai cabang vector sesuai contract mode tersebut. Trigram mendukung index substring, bukan jaminan pencocokan typo.
3. Parsing input query harus aman dan kompatibel dengan formula database. Prefix query berasal dari token/lexeme yang di-escape dengan benar; jangan meneruskan raw input ke `to_tsquery` atau sekadar menambahkan `:*` pada simbol arbitrer. Query tidak memeriksa sumber field atau isi dokumen melalui allowlist bisnis Go.
4. Escape `%`, `_`, dan backslash pada cabang `ILIKE`, lalu bind parameter melalui ORM. Jangan memakai operator similarity `%` atau ranking baru tanpa perubahan requirement.
5. Filter keyword digabung dengan AND terhadap scope authorization, `user_id`, aplikasi, status, dan filter existing lainnya. Khusus assignment, `ur.user_id = target` tetap wajib meskipun nama role sama untuk banyak user.
6. Jalur count dan page memakai predicate serta join search yang sama. PK/FK 1:1 mencegah penggandaan row; total dan next cursor tetap mengikuti semantics existing.
7. Pertahankan sort/tie-breaker existing; tidak mengurutkan ulang berdasarkan skor relevansi. Binding cursor mencakup canonical keyword, mode, field, filter, target, dan versi semantics pencarian baru. Cursor dari query/scope berbeda harus ditolak.
8. Empty keyword tidak mewajibkan adanya search row sehingga list tanpa keyword tetap utuh. Cutover hanya setelah kelengkapan backfill terbukti; jangan menambahkan fallback `ILIKE` diam-diam bila proyeksi belum lengkap.

Sumber query dan document harus diolah terpisah. PostgreSQL menyediakan parser query untuk teks biasa; input `to_tsquery` memerlukan sintaks yang valid. [Acuan text search PostgreSQL](https://www.postgresql.org/docs/16/textsearch-controls.html#TEXTSEARCH-PARSING-QUERIES).

### ORM dan Batas Layer

- Handler memetakan protobuf, domain memvalidasi mode/logical field/panjang keyword, feature mempertahankan authorization dan snapshot, reader membangun join/predicate melalui ORM. Validasi ini tidak membatasi token pada sumber awal di matriks.
- `catalog.go` reader/domain merupakan jalur bersama yang sudah ada; tambahkan mapping search per catalog di sana atau file pendamping kecil. Hindari empat implementasi pagination yang menduplikasi count/cursor.
- Writer aplikasi tidak menjalankan raw SQL untuk sinkronisasi search. DDL/function/trigger diletakkan pada script migration yang direview.
- Mapping Go hanya berisi feature → tabel/alias search → key join → dua kolom search. Jangan membuat daftar sumber seperti `email`, `phone`, atau `description` di Go untuk menyusun/membatasi dokumen keyword.
- ORM existing mempunyai `full_text_trigram`, tetapi builder prefix-nya hanya memecah whitespace dan menambahkan `:*`; normalisasi delimiter serta escaping harus diverifikasi sebelum digunakan ulang.
- Gunakan API ORM/query builder yang tersedia dengan ekspresi statis dan bound arguments. Bila perlu kemampuan ORM baru, catat gap dan scope sibling ORM terlebih dahulu; jangan mengubah repository ORM atau bypass ORM dalam implementasi grpc-auth tanpa scope eksplisit.

## 7. Contoh Hasil yang Diharapkan

| Feature | Data | Keyword | Hasil |
| --- | --- | --- | --- |
| Role | Nama `Senior Reviewer`, kode `clm.senior_reviewer` | `reviewer` | Cocok melalui nama |
| Permission | Kode `clm.document.read` | `document.read` | Cocok setelah pemisahan titik |
| Permission | Kode `clm.document.read` | `document-read` | Cocok dengan token yang sama |
| Permission | Kode `clm.document.read`, formula awal tidak memuat description | `dokumen` hanya ada pada description | Tidak cocok sebelum description dimasukkan ke dokumen search |
| Permission | Function diperluas untuk description dan data search sudah di-backfill | `dokumen` hanya ada pada description | Cocok melalui salah satu kolom search tanpa perubahan code grpc-auth |
| Application | Kode `clm`, nama `Claim Management` | `claim-management` | Cocok melalui nama |
| Assignment | User 123 memiliki role bernama `Reviewer` | `reviewer` | Menampilkan assignment user 123 sesuai filter status |
| Assignment | Nama role `Pemeriksa`, kode `clm.reviewer`, formula awal hanya nama | `reviewer` | Tidak cocok pada formula awal; ditentukan isi dokumen, bukan filter Go |
| Assignment | Hanya user 456 memiliki role `Reviewer` | `reviewer`, target user 123 | Tidak menampilkan assignment user 456 |
| Semua | Teks tersimpan tidak mengandung `%` | `%` | Tidak menjadi wildcard seluruh data |
| Semua | Dokumen mempunyai token `reviewer` | `revi` | Cocok melalui full-text prefix |
| Semua | Vector tidak cocok tetapi substring literal ada pada `fts_lexeme_text` | Keyword substring tersebut | Cocok melalui cabang text pada mode `FULL_TEXT_TRIGRAM` |
| Semua | Dokumen hanya mempunyai `reviewer` | `reviever` | Tidak dijamin cocok; toleransi typo bukan scope |

Contoh hanya menunjukkan cakupan dan mekanisme pencarian. Validasi izin, filter aplikasi/status, serta pembatasan namespace tetap dapat mengeluarkan item dari hasil.

## 8. Tahapan Implementasi

| Tahap | Pekerjaan | Kriteria selesai |
| --- | --- | --- |
| 1 | Cocokkan baseline function/trigger users dari user dengan database dan write path; petakan caller lama | Formula users terkonfirmasi, wrapper terverifikasi, compatibility terdokumentasi |
| 2 | Tetapkan refresh function database, tokenisasi dokumen, dan escaping query beserta fixture | Vector dan text memakai pola users; pemisah catalog serta simbol literal aman |
| 3 | Tambahkan empat tabel, trigger, script backfill/index/verifikasi/rollback | Proyeksi lengkap dan sinkron secara transaksional |
| 4 | Tambahkan logical keyword dan mode FTS di domain/reader catalog | Empat list hanya membaca dua kolom search; tidak hard-code sumber dokumen |
| 5 | Selaraskan fixture users dan uji perluasan formula tanpa perubahan Go | Baseline users lulus; sumber baru dapat dicari setelah function/trigger/backfill |
| 6 | Test, analisis query plan, serta dokumentasi caller | Hasil, cursor, isolation, dan performa terbukti sesuai matriks |
| 7 | Deploy schema/backfill lebih dahulu, lalu cutover caller rest-orc | Tidak ada periode pencarian memakai proyeksi belum lengkap |

Sesudah verifikasi schema, artefak implementasi dapat dipecah ke OpenSpec proposal/design/spec/tasks. Dokumen ini merupakan plan awal; belum membuat atau menerapkan OpenSpec change.

## 9. Verifikasi yang Wajib Saat Implementasi

- Unit/scenario: routing tabel/dua kolom search per feature, case-insensitive, separator berulang, Unicode, simbol literal, empty keyword, dan batas panjang. Test komposisi sumber berada pada function/trigger database.
- Pemisah: pastikan `clm.document-read`, `clm document read`, serta kombinasi separator berulang membentuk token target yang sama. Verifikasi simbol dipisahkan sebelum `to_tsvector` dan `fts_lexeme_text` tetap berasal dari array lexeme yang digabung dengan satu spasi. Jangan menguji urutan lexeme sebagai urutan kalimat sumber.
- Target vector catalog: uji contoh `iam.role.create`, `iam auditor`, dan `application-clm` pada bagian 3, termasuk posisi token dan tidak adanya token gabungan bertitik/hyphen. Regression users memastikan ketentuan baru ini tidak mengubah formula user search.
- Dynamic keyword: dengan binary/request grpc-auth yang sama, tambah sumber pada refresh function, sesuaikan trigger, backfill, lalu buktikan keyword baru cocok lewat vector saja atau text saja. Go tidak boleh menolak karena token tidak ada pada sumber awal.
- Query: placeholder/arguments, parser escaping, grouping AND/OR dengan scope authorization, wildcard literal, serta equality predicate count/page.
- Pagination: offset dan cursor ascending/descending, tie-breaker, keyword/filter berubah, user target berbeda, serta jalur compatibility lama.
- PostgreSQL integration: insert/update/delete sumber, role rename dengan banyak assignment, grant ulang/revoke, transaksi rollback, perubahan seed, backfill, dan concurrent rename/grant.
- Users: baseline email lengkap/potongan email, phone asli/digit-only, nama, serta sinkronisasi perubahan auth/profile. `country_code` tidak masuk formula awal; bila kelak ditambahkan, perubahan function/trigger/backfill cukup selama contract query tetap.
- Performance: bandingkan `EXPLAIN (ANALYZE, BUFFERS)` pada database test representatif untuk keyword pendek, substring, banyak token, keyword tidak cocok, count, dan page. Ukur ukuran index dan biaya role rename; jangan memaksa index scan pada tabel kecil.
- Seluruh test baru berada di `tests/` mengikuti runner/scenario existing. Jalankan targeted test, `go test ./...`, dan `go vet ./...` pada tahap implementasi. PostgreSQL integration memakai environment test yang disediakan; jangan menjalankan Docker.
- Rollback: kembalikan caller ke mode lama dahulu; pertahankan tabel/trigger selama masa kompatibilitas. Penghapusan tabel/index dilakukan pada migration cleanup terpisah setelah dependency tidak ada.

## 10. Acuan Source

- [Handler users dan mapping search mode](../internal/handler/user.handler.go)
- [User search repository](../internal/repositories/reader/user.reader.go)
- [Query users](../internal/repositories/reader/user_list_query.go)
- [Validasi catalog](../internal/repositories/domain/catalog.go)
- [Cursor catalog](../internal/repositories/domain/catalog_cursor.go)
- [Query catalog bersama](../internal/repositories/reader/catalog.go)
- [Handler catalog](../internal/handler/catalog.go)
- [API assignment role user](user-role-assignment-api.md)
- [Migration user management existing](sql/user-management/README.md)
- [Fixture PostgreSQL existing](../tests/integration/postgres/fixtures/schema.sql)

Source lokal dan dokumentasi PostgreSQL diperiksa pada 22 Oktober 2026. Baseline trigger dan refresh function users berasal dari definisi yang diberikan user dalam percakapan. Kesesuaian deployment database, isi kedua function wrapper, dan performa belum diuji dalam penyusunan plan ini.
