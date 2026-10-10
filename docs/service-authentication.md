# Service caller authentication

The gRPC server requires TLS 1.3 and one opaque service credential on every
registered RPC, including Register, Login and TesRPC. Startup rejects missing
TLS/caller configuration, invalid or expired certificates, mismatched keys,
duplicate identities/digests and malformed or wildcard authority. Validation
happens before logger/database/listener initialization. Startup failures close
constructed gRPC clients/server; shutdown drains RPCs before closing pools.

Configuration remains in local `env.yaml`; see [env.example.yaml](../env.example.yaml).
Supply `TLS.CertFile` (PEM certificate chain) and `TLS.KeyFile` (matching private
key). Clients must trust the issuing CA and verify the server's DNS/SAN hostname.
There is no plaintext or InsecureSkipVerify fallback. Config examples contain
empty values and cannot start the service until deployment values are supplied.

## Credential and rotation

Generate 32–256 cryptographically random bytes outside the repository and encode
them as unpadded, canonical base64url. Send that encoded string in the single
`x-service-token` metadata entry. Store only its hex SHA-256 digest in
`ServiceAuth.Callers[].Current.SHA256`; hashing applies to the encoded string,
not the decoded bytes. The verifier checks canonical encoding and decoded
length, but randomness must be guaranteed by the secret provisioning process.
Never copy the credential into source, examples, logs, audit or events.

Each caller has a server-owned `ServiceID`, `Current.SHA256`, `Current.NotAfter`,
exact `Methods` and exact `Namespaces`. Timestamps use RFC3339 with timezone.
Caller IDs/digests and allowlist entries must be unique. There are no default
credentials, expiry, wildcard grants or inferred permission grants.

For rotation, replace Current and explicitly configure Previous with SHA256,
NotAfter and OverlapUntil. OverlapUntil must be in the future at startup and no
later than either credential's NotAfter. Previous is accepted strictly before
both deadlines; at the boundary it is rejected without grace. Current is also
rejected at NotAfter. Remove expired Previous entries and restart to reload;
startup rejects stale overlap configuration rather than silently extending it.

Clients supply service credentials from their server configuration using secure
gRPC call credentials. `authorization: Bearer <user JWT>` remains separate.
Metadata such as x-service-name/user_id never establishes either identity.
Callers are checked before bearer parsing and any business handler. Empty,
unknown, expired, duplicate or malformed service tokens and non-TLS peers return
generic `auth.service_authentication_failed` / UNAUTHENTICATED. A verified caller
outside its method or namespace allowlist returns common.forbidden /
PERMISSION_DENIED. Language/Error interceptors wrap these outcomes. Unknown
unregistered methods retain gRPC UNIMPLEMENTED.

## Consumer boundary and readiness

`internal/serviceauth.CallerFromContext` returns immutable verified identity.
`Caller.ID()` is suitable for actor_service. Permission evaluation must call
`Caller.AuthorizeNamespace` with an exact namespace such as `iam` or `clm` before
using user permission results. Neither method nor namespace authority grants the
user's permission. Namespace checks are available for consumers; no future
EvaluateAccess RPC is registered by this component.

The default server composes Language → Error → Service → independent attempt
Audit → Session → Rate → authoritative handlers. Real Login and every consumer
use the same primary pool. All six methods require the service credential;
Register/Login/TesRPC alone are exempt from the user JWT. Source composition is
complete. External caller/IP/client compatibility, actual deployment rotation
and release readiness remain mandatory in [Phase 6 DEP-01](foundation-activation-readiness.md).

Verification: `rtk go test ./tests/configs/... ./tests/interceptors/...`; race
verification uses the same targets with `-race`. These suites use controlled
caller clocks and test-only certificate fixtures over in-memory transport,
without external services, database fixtures or opening a TCP listener.
