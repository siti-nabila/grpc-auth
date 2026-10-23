# Implementation Evidence

Tanggal awal: 22 Oktober 2026, 18:48 WIB (11:48 UTC). Source working tree mempunyai perubahan user sebelum change ini; HEAD di bawah bukan klaim seluruh perubahan sudah committed.

- grpc-auth HEAD: `38e70b922e73da354485fd5693c9088c8f036e38`.
- api-contracts HEAD: `b4ad807ad5faafe35c3c5081932597f43f8d9e7f`.
- Go: `go1.26.2`; workspace `/Users/bleki/Desktop/mini-projects/go.work` memakai sibling api-contracts.
- Generator: `libprotoc 33.4`, `protoc-gen-go v1.36.11`, `protoc-gen-go-grpc 1.6.1`.
- Contract source SHA256 sesudah tag 15: `69bfc6480d1a152b505c4f6fec63eec364c58563d761f2f6536a1bb0a13f2850`.

## Contract — MAS-EXT-01

| Command / check | Expected | Actual |
| --- | --- | --- |
| `rtk proxy make proto` di api-contracts | Generator berhasil; existing generated changes user tetap | PASS; hanya `pb/user/v1/user.payload.pb.go` berubah dibanding hash sebelum generator |
| `rtk proxy env GOCACHE=/tmp/grpc-auth-me-go-cache go test ./tests/protobuf/...` | Tag/type 1–14 tetap; map tag 15, Empty input, binary/proto-JSON dan legacy hop valid | PASS |
| `rtk proxy env GOCACHE=/tmp/grpc-auth-me-go-cache go test ./...` di api-contracts | Full contract suites PASS | PASS |
| `rtk proxy env GOCACHE=/tmp/grpc-auth-me-go-cache go vet ./...` di api-contracts | Tidak ada finding | PASS |

Dokumentasi contract ada di sibling `docs/me-access-contract.md`. Source/protobuf/tests sibling diterapkan melalui akses filesystem tambahan yang disetujui; tidak ada module publication, dependency pin, commit atau push.

Default Go build cache awal ditolak filesystem sandbox. Test diulang memakai GOCACHE di `/tmp`; kegagalan cache tersebut bukan failure contract. Targeted grpc-auth pertama menemukan duplicate-method allowlist pada helper fixture untuk Me; helper khusus Me kemudian disesuaikan, hasil ulang dicatat di bawah setelah tersedia.

## Backend dan PostgreSQL — MAS-01..06

Hasil akhir diperiksa pada 22 Oktober 2026, 19:03 WIB (12:03 UTC), pada working tree implementasi tanpa commit/publish. PostgreSQL **16.13**, dedicated test database identity verified. Environment opt-in/DSN/expected database dimuat privat dari `env-test.yaml` oleh launcher sementara di `/tmp`; tidak menyalin DSN/credential ke repository atau evidence. Seluruh schema yang ditulis/dihapus merupakan schema test dengan ownership guard existing. Tidak ada database aplikasi, migration aplikasi atau Docker yang dijalankan.

Semua command Go berikut memakai `GOCACHE=/tmp/grpc-auth-me-go-cache` untuk menghindari build-cache sandbox. PostgreSQL memakai explicit opt-in variables dan akses jaringan tambahan yang disetujui. Launcher aktual `go run /tmp/run-me-postgres.go <test arguments>` menjalankan command `go test` setara di bawah dengan environment privat tersebut; launcher tidak menjadi runtime dependency.

| Repository / command | Expected | Actual |
| --- | --- | --- |
| grpc-auth: `go test ./tests/integration/auth_reads ./tests/repositories/reader/user ./tests/features/auth_feature ./tests/handler` | Me fields/namespace/minimum, reader gates, literal concat, context/failure/finalization valid | PASS |
| grpc-auth: `go test ./tests/integration/auth_reads -run TestMeAccessSnapshot -count=1` | Registered Me-only caller without admin; expired/revoked/cross-user/stale-version/suspended denied | PASS |
| grpc-auth: `go test ./...` | Semua default suites dan existing methods/regressions tetap lulus | PASS |
| grpc-auth: `go vet ./...` | Tidak ada finding | PASS |
| grpc-auth: `go vet -tags=postgres ./tests/integration/postgres/...` | PostgreSQL test source tanpa finding | PASS |
| grpc-auth: `go test -race ./tests/integration/auth_reads ./tests/repositories/reader/user ./tests/features/auth_feature` | Snapshot/mapping/shared-state scenarios tanpa data race | PASS |
| grpc-auth: `go test -tags=postgres ./tests/integration/postgres -run '^TestPostgresConfiguration$'` | Dedicated-DB guard tetap valid tanpa network prerequisite | PASS |
| grpc-auth: `go test -tags=postgres ./tests/integration/postgres -run '^TestPostgresMeAccessSnapshot$' -count=1 -v` | Active/soft-delete/namespace gates, no writes, consistent snapshots dan cleanup | PASS, 2.061s package elapsed |
| grpc-auth: `go test -race -tags=postgres ./tests/integration/postgres -run '^TestPostgresMeAccessSnapshot$' -count=1 -v` | Scenario PostgreSQL Me yang sama tanpa race | PASS, 5.867s package elapsed, dijalankan tanpa proses suite lain |
| grpc-auth: `go test -tags=postgres ./tests/integration/postgres -count=1` | Full PostgreSQL lifecycle/auth/role/access/transaction regression | PASS, 36.887s, dijalankan setelah race selesai |
| rest-orc: `go test ./...` | Consumer tetap compile dan existing mappings/handlers/middleware lulus dengan contract workspace baru | PASS; source consumer tidak diubah |

Proof dua physical backends pada race Me: mapping **11833/11834**, role **11837/11838**, application **11841/11842**, access **11845/11846**, user **11849/11854**, session **11857/11859**. Barrier membentuk RR snapshot pada koneksi A sebelum koneksi B commit. View lama konsisten dan snapshot berikutnya melihat perubahan/denial; tidak memakai sleep sebagai proof.

Registered Me-only scenario membuktikan tidak perlu allowlist RPC permission/admin permission untuk Me. Database scenarios memeriksa metadata tanpa mapping permission, overlapping sources, soft-deleted mappings, revoked/disabled gates, namespace subset, restricted minimum dan snapshot failure. Snapshot business state sebelum/sesudah Me memeriksa tidak ada perubahan auth/profile/assignment/access/session/refresh/success audit/outbox/idempotency. Default scenarios mencakup NULL/empty/legacy-prefix/empty-phone concat dan response nil pada setiap read/finalization failure.

Riwayat perbaikan verifikasi:

- Koneksi PostgreSQL awal ditolak sandbox; akses jaringan tambahan kemudian berhasil pada database test terverifikasi.
- Test baru sempat membandingkan nil versus empty map protobuf sebagai berbeda dan membuat fixture revoke tanpa reason; assertions/fixture diperbaiki mengikuti semantics protobuf serta constraint existing, tanpa mengubah source runtime.
- Full dan race PostgreSQL sempat berjalan bersamaan; advisory locks berbasis canonical user ID berlaku pada database yang sama lintas schema, sehingga terjadi RESOURCE_BUSY antarproses test. Kedua command dijalankan ulang secara berurutan dan lulus; tidak mengubah lock protocol runtime.
- Full suite menemukan assertion metadata fixture lama `70/35/22`, sedangkan schema country-code existing sudah `71/35/22`. Hanya expected column count pada `tests/integration/postgres/test_scenarios/schema.go` diselaraskan; schema/constraint/index runtime tidak diubah.

| Criterion | Result | Evidence |
| --- | --- | --- |
| MAS-EXT-01 | PASS | Generator, descriptor/binary/proto-JSON/legacy-hop dan full contract test/vet di atas |
| MAS-01 | PASS | Registered Me-only, invalid session states, restricted minimum, timestamp/identity assertions |
| MAS-02 | PASS | Unit reader/mapping + PostgreSQL active gates, namespace subset, overlap/soft-delete dan metadata tanpa permission |
| MAS-03 | PASS | SQL-fake failures, six two-backend barriers, PostgreSQL cancellation/deadline/panic/driver/finalization/pool cleanup |
| MAS-04 | PASS | Literal concat matrix default tests; registered PostgreSQL phone dan unchanged stored/profile/detail/list regressions |
| MAS-05 | PASS | Default/full PostgreSQL regression, snapshot state comparison, repeated reads/Idempotency-Key, safe fields/admission/error behavior |
| MAS-06 | PASS, local workspace only | Full test/vet kedua repository, targeted race, full PostgreSQL, rest-orc regression |

Source fingerprints implementasi:

| File grpc-auth | SHA256 |
| --- | --- |
| `internal/features/auth_feature/read_snapshot.go` | `7f3434d1f5e6facf7b0bf82590a11dfab34157a62ae62ca9211ec687b41fa1a6` |
| `internal/features/auth_feature/me_access.go` | `b59f55cfa1e56ff87abf035e8904080ffc7bfd554a1113e15261727d07af0b9e` |
| `internal/repositories/domain/user_identity.go` | `1056b6b1b3b59689a420fc5f57285efbd5fd007b2da1575af67f77bb07726378` |
| `internal/repositories/domain/self_access.go` | `09cdb7bd9aab2d41aeb8c393d081190ba3db2138f2cb50497db583113aa5e7ee` |
| `internal/repositories/reader/user_identity.go` | `fb6fc7f57bb01ce94942c392c2ce946c9cc2ceadb8c4f1ad2e616939240886f4` |
| `internal/repositories/reader/self_access.go` | `260336538c325ff2333752c2d395b2c998516156341851131a32651efee3fe53` |

Build path/composition wiring tidak diubah; default full tests mengompilasi package command/configuration. Seluruh test baru tetap di root `tests/`. Gofmt dan review diff/whitespace dilakukan; perubahan user existing dipertahankan.

OpenSpec strict validation PASS tanpa issue. Tabel/JSON/link/anchor/whitespace tujuh dokumen terkait valid; hanya expected additions dan perubahan source/test yang dijelaskan di atas ditulis. Seluruh 24 checkbox implementasi/verifikasi selesai; functional PASS berdasarkan command aktual, bukan status file-existence OpenSpec.

MAS-EXT-02 publication/pin/GOWORK=off tetap DEFERRED_TO_SEPARATE_REQUEST. Mandatory deployment Phase 6 DEP-01 dan implementasi consumer tetap follow-up, bukan PASS dari local workspace tests.
