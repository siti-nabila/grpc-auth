# Tasks

## 1. Baseline dan reproducible workspace

- [x] 1.1 Capture source manifest/status/hashes grpc-auth dan ORM, inventory35 serta affected repository query inventory berdasarkan temuan design; verifikasi baseline/user hunks dapat dibandingkan tanpa reset dan semua raw statement/subquery call sites tercatat (ORM-01/07).
- [x] 1.2 Siapkan temporary workfile dengan seluruh member parent ditambah sibling ORM, tanpa mengubah parent go.work/go.mod untuk workaround; verifikasi go list kedua module menunjuk checkout yang benar dan ORM standalone memakai GOWORK=off (ORM-07).

## 2. ORM query composition — sibling ORM

- [x] 2.1 Implement WhereExists/WhereNotExists/WhereInQuery melalui child builder yang disnapshot dan dibangun dalam dialect/placeholder parent; tambah tests di root ORM test_scenarios/query/ untuk correlation, nested groups, SET-offset UPDATE, nil/invalid child dan fail-before-execution; verifikasi targeted tests/DryRun query/args/mode PostgreSQL/MySQL/Oracle (ORM-02).
- [x] 2.2 Implement LeftJoinQuery untuk grouped derived queries, safe alias/quoting, ordered bound args dan reusable child; tambah count/page/nested/mixed-configuration/repeated-DryRun immutability scenarios pada test root ORM; verifikasi targeted multi-dialect tests tanpa query campuran atau mutation (ORM-02).
- [x] 2.3 Dokumentasikan API komposisi dengan contoh correlated existence/IN dan grouped derived join pada docs ORM existing; verifikasi contoh memakai public API yang sama dengan DryRun/tests dan tidak memperkenalkan raw SELECT passthrough (ORM-02).

## 3. ORM INSERT result — sibling ORM

- [x] 3.1 Tambahkan CreateWith.ExecResult yang mengembalikan sql.Result melalui existing insert mapper/build/execution/logging dan transaction context; pertahankan Create/Exec/Scan/ScanInto/DryRun, tolak incompatible returning intent; tambah tests ORM test_scenarios/ untuk query/args/mode tiga dialect, null/zero/tag behavior, context/cause dan missing result (ORM-02).
- [x] 3.2 Verifikasi result diteruskan apa adanya untuk count0/1/multiple/negative dan RowsAffected failure memakai controlled mock, tanpa auto-finalization transaction; jalankan targeted dan regression ORM tests serta dokumentasikan perbedaan generated-ID/returning vs result execution (ORM-02/07).

## 4. Schema soft delete dan inventory mapping — grpc-auth

- [x] 4.1 Siapkan reviewed preflight/additive SQL docs/sql/role-permission-soft-delete/ untuk deleted_at timestamptz NULL tanpa default dan composite PK tetap; update fresh schema documentation serta tests/integration/postgres/fixtures/schema.sql, bukan menjalankan migration aplikasi; verifikasi existing-row preservation, incompatible-column rejection, nullability dan uniqueness pada opt-in isolated schema scenarios (ORM-03).
- [x] 4.2 Tambahkan model/inventory read role-permission pairs termasuk tombstones pada existing domain/reader contract dan request-bound primary handle; tambah repository scenarios di root tests/ untuk absent/active/deleted/mixed pairs, row identity/order/context dan dependency failure; verifikasi targeted runner (ORM-03).
- [x] 4.3 Dokumentasikan migration-before-activation, old-binary incompatibility setelah tombstone writes, compatible rollback/forward-fix dan ketiadaan backfill history pada docs role-management serta schema guide; verifikasi local links dan fresh/existing fixture schema parity (ORM-03/07).

## 5. Assignment/application writers dan transaction adapter

- [x] 5.1 Expose insert capability kecil pada leasedTransaction ke ORM adapter pada lease/context yang sama, sebelum writer consumer dimigrasikan; tambah transaction adapter scenarios di root tests/ untuk context/handle/finalization ownership dan verifikasi targeted runner (ORM-02/06).
- [x] 5.2 Refactor user_role_assignment writer memakai insert-specific models/ExecResult dan conditional Updates patch; tambah/ubah root tests/repositories/assignment scenarios untuk identity/status guards, clearing revoke provenance, preserved grant fields, strict count/cause/cancellation; verifikasi targeted runner (ORM-01/02).
- [x] 5.3 Refactor application_access writer dengan API ORM yang sama tanpa mengubah pair guards/lifecycle/compound version ownership; tambah/ubah root tests/repositories/application scenarios termasuk NULL reset dan missing/invalid result; verifikasi targeted runner (ORM-01/02).


## 6. Role mapping soft-delete/restore command

- [x] 6.1 Feature CreateRole/ReplaceRolePermissions menentukan sorted mapping change set dan meneruskan command now dari injected clock; writer menjalankan conditional soft-delete/restore/insert melalui ORM pada same tx, mempertahankan unchanged pairs; perbarui internal domain contract/mocks dan tambah feature/repository scenarios untuk A/B -> B/C -> A/B, retained pair identity/timestamp dan empty final set; verifikasi targeted runners (ORM-03).
- [x] 6.2 Tambahkan relevant soft-delete/restore/insert count/error, stale/current no-op, protected system role, response-lost replay, no version/user/auth/session side effect dan Begin -> locks -> recheck/read -> write -> finalization ordering scenarios; verifikasi feature/root repository tests dan rollback pada semua error/cancel/panic (ORM-03/06).
- [x] 6.3 Perbarui docs role-management final-set/no-op/replay dan mutation/audit/outbox matrix agar soft-delete/restore tidak dianggap hard delete; verifikasi event/aggregate/schema contract tetap dan seluruh kriteria terkait ditautkan ke scenario actual (ORM-03/06).

## 7. Reader query construction dan nondeleted permissions

- [x] 7.1 Refactor ListUsers role/application EXISTS dan MIN collection keys menjadi ORM child queries/derived joins dengan semantics existing; tambah/ubah tests/repositories/reader/user dan registered read scenarios untuk counts, all sorts/null ties, filters/search, safe projection, numeric/composite cursor continuity dan bound args; verifikasi targeted runners (ORM-01/05).
- [x] 7.2 Refactor affected-user role count melalui grouped derived query yang dipakai detail/catalog projection/sort/count/cursor; tambah/ubah role/catalog scenarios termasuk empty/duplicate/disabled/nonactive-user rows sesuai definition existing; verifikasi targeted runners (ORM-01/05).
- [x] 7.3 Refactor DefaultMember NOT EXISTS dan revoke-user-refresh IN query melalui ORM composition; tambah root registration/refresh repository tests untuk zero active mapping meski tombstone tersimpan, request context dan full token scope; verifikasi targeted runners (ORM-01/04).
- [x] 7.4 Filter rp.deleted_at IS NULL pada authoritative contribution dan RolePermissions reads; tambah root repository/feature/registered E scenarios untuk sole/overlap sources, actor authority before target/replay, role-enable/app gates dan nonactive target; verifikasi existing original-JWT policy serta sorted results tanpa tombstones (ORM-04).
- [x] 7.5 Hapus dead raw SELECT example dan selesaikan audit seluruh production reader/writer/adapter, bukan hanya known files; verifikasi AST/source scan zero manual statement/subquery execution dan seluruh exceptions hanya migration/fixture/ORM builder; dokumentasikan audit inventory serta hasil (ORM-01).

- [x] 7.6 Setelah semua writer dimigrasikan, hapus StatementWriter/native ExecContext escape dan update transaction adapter documentation/tests; verifikasi tidak ada consumer tertinggal serta native handle hanya digunakan untuk finalization cleanup (ORM-01/06).

## 8. Dedicated PostgreSQL integration gates

- [x] 8.1 Sesuaikan fixture INSERT explicit column lists/counts dan failure injection lama dari hard DELETE menjadi soft-delete/restore UPDATE; tambah real persisted identity/deleted_at/regrant/default-member/current-permission assertions dan migration preservation scenario; verifikasi isolated opt-in targeted role/permission suites (ORM-03/04).
- [x] 8.2 Uji failure tiap mapping soft-delete/restore/new insert, role version, required audit/outbox/result/commit, cancelled/panic cleanup, no-op/replay concurrency dan current actor privilege loss memakai dua physical backend connections/barriers; verifikasi atomic before/after termasuk timestamps dan pool reusable, normal/race (ORM-06).
- [x] 8.3 Jalankan PostgreSQL ListUsers/role catalog continuation serta registered E consistent snapshot scenarios terhadap query shape baru; verifikasi no duplicate/skipped entities, null/cursor/count parity dan committed revoke/restore visibility dengan original JWT, normal/race (ORM-04/05/06).

## 9. Published dependency dan final integration

- [x] 9.1 Jalankan gofmt, targeted/full ORM test/vet/race dan siapkan concrete reviewed release diff/docs; catat source identity dan pertahankan user changes. Publication Git hanya setelah scope eksplisit; task published-release terkait tetap BLOCKED bila artifact belum tersedia (ORM-02/07).
- [x] 9.2 Setelah authorized immutable ORM release benar-benar tersedia, pin versi/checksum actual pada grpc-auth dan verifikasi standalone resolution tanpa implicit local replace/module-cache edits; catat origin commit dan go.mod/go.sum evidence. Jangan invent versi atau tandai workspace-only sebagai delivery (ORM-07).
- [x] 9.3 Jalankan targeted grpc-auth terlebih dahulu lalu full test/vet/race/build pada controlled workspace dan GOWORK=off; ulangi required full opt-in PostgreSQL normal/race terhadap source final dalam kedua mode, termasuk prior session/last-admin/atomicity regressions; catat command/env aman/time/expected-actual/source identity, unavailable gates BLOCKED (ORM-06/07).
- [x] 9.4 Lengkapi evidence ledger ORM-01..07 dan inventory35/unchanged protobuf-error/security-limiter checks; verifikasi strict OpenSpec, local links, diff whitespace/status, source manifests dan user-hunk preservation. Checklist hanya selesai sesudah semua required backend gates PASS; actual migration/deployment/consumer readiness tetap DEP-01 terpisah (ORM-07).
