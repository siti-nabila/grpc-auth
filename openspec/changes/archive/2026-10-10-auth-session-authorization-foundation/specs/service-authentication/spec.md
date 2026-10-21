# Spec Delta

## Purpose

Memastikan setiap RPC grpc-auth hanya dipanggil service caller terverifikasi melalui TLS, dengan credential terpisah dari identitas user dan batas akses method/namespace eksplisit.

## ADDED Requirements

### Requirement: Every RPC authenticates a service caller over TLS

Semua RPC SHALL memerlukan TLS dan opaque service credential melalui `x-service-token`, terpisah dari bearer JWT user. Credential SHALL memiliki minimal 32 byte entropy cryptographically random, diverifikasi melalui digest constant-time terhadap caller yang dikonfigurasi. Server MUST tidak mempercayai service/user identity dari browser atau payload; secret MUST tidak mempunyai default atau muncul di log/response/audit/event. Configuration wajib yang invalid SHALL menggagalkan startup tanpa partial insecure listener.

#### Scenario: Service authenticates without a user access token

- **WHEN** caller dengan TLS dan service credential valid memanggil `/user.v1.UserService/Register`, `/user.v1.UserService/Login`, atau `/user.v1.UserService/TesRPC`
- **THEN** service authentication berhasil tanpa mewajibkan access JWT, sementara validation/credential mode RPC masing-masing tetap berlaku dan TesRPC dipertahankan

#### Scenario: Missing invalid or ambiguous service credential

- **WHEN** salah satu RPC existing dipanggil tanpa credential, dengan credential tidak dikenal/expired, atau metadata credential ambigu
- **THEN** request menghasilkan generic UNAUTHENTICATED sebelum feature bisnis dan tidak menghasilkan success mutation

#### Scenario: TLS trust is invalid

- **WHEN** koneksi tidak memakai TLS atau client menemukan CA/hostname certificate tidak valid
- **THEN** koneksi ditolak dan tidak ada RPC bisnis yang dieksekusi

### Requirement: Caller authority uses exact method and namespace allowlists

Caller terverifikasi SHALL mempunyai exact full-method allowlist dan permission namespace allowlist dari server configuration. Caller valid di luar authority tersebut SHALL menerima PERMISSION_DENIED. Policy MUST tidak menggunakan prefix wildcard, user role, atau service name payload sebagai bypass; method baru MUST tidak memperoleh implicit allow. Namespace policy SHALL menjadi prerequisite future permission evaluation tanpa menambahkan RPC placeholder.

#### Scenario: Direct gRPC caller bypasses REST

- **WHEN** credential caller valid tetapi `/user.v1.UserService/ListUsers` tidak ada dalam method allowlist-nya
- **THEN** service authorization menolak request meskipun bearer user memiliki iam.user.read

#### Scenario: Namespace is outside caller authority

- **WHEN** consumer permission evaluation menggunakan verified caller yang tidak diizinkan untuk namespace permission yang diminta
- **THEN** caller policy menghasilkan PERMISSION_DENIED tanpa memberikan hasil allowed dari role user

#### Scenario: Unknown method is requested

- **WHEN** caller mencoba method yang belum diregistrasikan
- **THEN** tidak ada fallback policy yang memberi akses dan transport mempertahankan UNIMPLEMENTED untuk RPC tersebut

### Requirement: Caller rotation preserves explicit expiry and safe identity

Service credential rotation SHALL menerima current dan previous credential hanya dalam window overlap/not_after yang eksplisit. Expired previous credential MUST ditolak tanpa default grace. Downstream feature SHALL menerima service ID terverifikasi untuk audit, bukan raw credential; failure telemetry SHALL bounded dan redacted.

#### Scenario: Previous credential crosses its overlap boundary

- **WHEN** controlled clock melewati not_after previous credential
- **THEN** previous credential ditolak UNAUTHENTICATED sementara current credential yang valid tetap dapat mengidentifikasi caller yang sama

#### Scenario: Register audit needs a caller actor

- **WHEN** Register dengan service authentication valid menyimpan success audit
- **THEN** actor_service berasal dari verified caller dan actor_user_id null, tanpa menyimpan credential/header

Evidence dan dependency: [SAF-01, SAF-07, SAF-09, SAF-10](../../design.md#acceptance-dan-evidence-ledger). Backend evidence tersedia pada server-verification.json; actual external caller/deployment readiness tetap mandatory Phase 6 DEP-01 dan tidak dibuktikan oleh fixture lokal.
