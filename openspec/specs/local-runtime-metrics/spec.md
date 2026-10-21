# Local Runtime Metrics

## Purpose

Provide safe process-local observations of unary RPC outcomes, admission,
audit persistence and runtime resources so operators can inspect Phase 5
behavior without additional infrastructure or authority over business state.

## Requirements

### Requirement: Unary completion and duration observations preserve transport outcomes

For the 37 registered unary methods, the service SHALL observe each invocation
that enters the unary pipeline exactly once after error encoding, including
service, admission, session and handler rejection. Completion SHALL identify
the bounded method and final gRPC code; duration SHALL include the complete
pipeline and independent-attempt work. Duration SHALL expose count, sum and
fixed buckets at 1, 5, 10, 50, 100, 500, 1000 and 5000 milliseconds plus overflow.
Observation MUST NOT modify response, error details, locale, cancellation,
authorization or panic propagation, or invoke business handling again.
TLS handshake failures and unknown routes rejected outside the unary pipeline
SHALL NOT be represented as observed unary completions.

#### Scenario: Early service rejection remains visible
- **WHEN** `/user.v1.UserService/ListUsers` enters the unary pipeline with invalid service credentials
- **THEN** one UNAUTHENTICATED completion and its duration are recorded, the original localized error remains, and no handler or audit transaction runs

#### Scenario: Dependency and context errors retain their final code
- **WHEN** an observed `/authorization.v1.AuthorizationService/EvaluateAccess` encounters a dependency failure, or an observed call ends with cancellation/deadline
- **THEN** one completion uses the existing encoded UNAVAILABLE, CANCELED or DEADLINE_EXCEEDED outcome respectively, without converting dependency failure to an access denial

#### Scenario: Duration boundaries are deterministic
- **WHEN** controlled elapsed times fall exactly on or between the fixed histogram boundaries
- **THEN** count, sum and cumulative buckets represent each invocation once without negative durations or a fabricated percentile

#### Scenario: Existing panic behavior is retained
- **WHEN** downstream recovery handles a panic or a panic escapes the existing recovery boundaries
- **THEN** observation does not change that recovery/propagation behavior, double invoke handling, or prevent transaction cleanup

### Requirement: Security outcome metrics distinguish denial from dependency failure

The service SHALL expose finite-category auth failure, access denial and
dependency counters derived from existing catalog identities and safe final
codes. An encoded credential/session/service rejection SHALL remain distinct
from permission denial and dependency failure. A valid
`/authorization.v1.AuthorizationService/EvaluateAccess` response with
`allowed=false` SHALL count as denial while its RPC completion remains OK.
No metric SHALL become an authorization source or infer identity from a payload.

#### Scenario: A valid access decision denies permission
- **WHEN** a currently authenticated caller receives a valid allowed=false EvaluateAccess response
- **THEN** one denial is counted and one OK RPC completion is retained without manufacturing a transport error

#### Scenario: Generic Login denial and an unavailable dependency differ
- **WHEN** `/user.v1.UserService/Login` returns generic LOGIN_FAILED in one invocation and UNAVAILABLE in another
- **THEN** only the first contributes to credential failure, the second contributes to dependency failure, and neither exposes account existence or private error text

### Requirement: Observation state and exported data have finite cardinality

Observation state SHALL be process-local, concurrency-safe and bounded by the
registered 37-method inventory plus one unknown-method category, the fixed gRPC
code set and documented finite reason/origin groups. Arbitrary method text that
reaches observation SHALL map to unknown without changing routing. Metrics,
snapshots and summary logs MUST NOT include user/email/IP/session IDs, arbitrary
caller IDs, tokens, credential digests, payloads, permission input, raw errors,
DSNs or correlation identifiers. Snapshot reads MUST NOT expose mutable internal
state. Counters SHALL be cumulative until process restart and gauges SHALL
describe samples rather than accumulate samples as events.

#### Scenario: Arbitrary labels and secrets are supplied
- **WHEN** many distinct method strings, credential sentinels and private error messages are presented to observed boundaries
- **THEN** series count stays within the documented bound and no sentinel or arbitrary string appears in snapshots or logs

#### Scenario: Concurrent observations and snapshot reads occur
- **WHEN** admissions, completions, audit failures and snapshot reads run concurrently
- **THEN** exact completed-operation totals remain valid, snapshots cannot mutate future observations, and no data race or unbounded per-request allocation is introduced

### Requirement: Admission and audit failure observations retain single ownership

The service SHALL expose quota/capacity rejection counts and limiter entry
gauges, distinguishing service admission from downstream identity admission.
Each rejected stage SHALL count once even when it checks multiple keys.
Independent Login/security-attempt failures and required audit write failures
SHALL be observable at their owning boundary with finite origins. An uncertain
transaction finalization SHALL be identifiable as uncertainty, not a proven
lost audit row or guaranteed rollback. Related counters SHALL document their
overlap and MUST NOT double count the same failed operation under one origin.
Metrics failure MUST NOT replace the original business outcome or write audit.

#### Scenario: Layered refresh admission rejects once
- **WHEN** `/auth.v1.AuthService/RefreshSession` proves a session but its downstream layered admission rejects
- **THEN** one downstream rejection is counted, the service-stage admission remains consumed, and neither generic admission nor attempt auditing adds a duplicate rejection/audit

#### Scenario: An independent audit recorder fails
- **WHEN** an admitted Login denial or generic security attempt encounters recorder error or panic after cleanup
- **THEN** one failure for the owning attempt is observable while the original safe error/locale remains and no recursive attempt or per-error log is created

#### Scenario: Required audit write or commit acknowledgement fails
- **WHEN** an admitted mutation fails at its required audit write or loses acknowledgement after a possible commit
- **THEN** the appropriate write failure or uncertain-finalization observation is recorded without declaring an audit row lost, retrying the mutation or weakening atomicity

### Requirement: Runtime samples describe pending outbox and existing pool state

Runtime observation SHALL report pending outbox count and age of the oldest
pending record, using `published_at IS NULL` regardless of `next_attempt_at`.
Count and oldest creation time SHALL come from one consistent aggregate read
on the existing primary pool; no payload shall be selected. Empty pending state
SHALL report count/age zero; future creation time SHALL clamp age to zero.
Pool gauges SHALL include open, in-use, idle and maximum connections, and pool
wait count/duration SHALL retain their cumulative meaning. Collection MUST NOT
mutate outbox, publish events, open a pool per sample or authorize a request.

#### Scenario: Pending and published records coexist
- **WHEN** pending records include a future retry schedule and other records are published
- **THEN** all and only pending records contribute to count/oldest age and no payload, publication or retry state changes

#### Scenario: The aggregate is empty or has a future timestamp
- **WHEN** there are no pending records, or the oldest pending creation time lies after controlled sample time
- **THEN** the empty result reports zero count/age and the future result retains its count with nonnegative zero age

#### Scenario: A required outbox event commits without a broker
- **WHEN** a permitted mutation commits its required pending event while no relay is active
- **THEN** a later sample includes that event without claiming delivery or creating an event to fill an aggregate-version gap

### Requirement: Sampling and summary logging are isolated from request handling

The service SHALL collect runtime samples and emit a bounded local summary once
per minute using existing logging. Sampling SHALL be nonoverlapping with no
unbounded queue or catch-up burst, and the primary aggregate query SHALL have
a two-second timeout. Request observation SHALL perform no log/DB I/O or
per-request worker creation. Limiter cleanup SHALL progress independently of
sampling/logging. Output SHALL have fixed finite fields/record count and a
documented byte-size bound for finite numeric values.

#### Scenario: A log write is slow
- **WHEN** the periodic synchronous logger write is blocked by a test-controlled sink
- **THEN** request admission/completion and limiter cleanup still progress, no extra sampler is created, and later ticks do not enqueue summaries

#### Scenario: The primary aggregate read times out
- **WHEN** the collector query exceeds its two-second deadline or returns an error
- **THEN** collection failure becomes observable, no mutation or RPC outcome changes, and raw error/DSN/payload does not enter summary logs

### Requirement: Sample freshness and worker lifetime remain explicit

Before a successful aggregate sample, outbox status SHALL be unavailable rather
than healthy zero. After failure, the last successful value and observation
time SHALL remain with stale status until recovery. Collection/export failures
SHALL use bounded observation without recursive logging. Startup failure,
listener failure and shutdown SHALL cancel worker contexts, stop tickers and
join workers before closing their pool. Existing synchronous log writes SHALL
be allowed to finish; shutdown MAY wait for them as explicitly approved by the
user, without claiming bounded write cancellation or abandoning a goroutine.

#### Scenario: Initial failure and later recovery occur
- **WHEN** the initial aggregate read fails, a later one succeeds and a subsequent one fails
- **THEN** snapshots progress from unavailable to available to stale with the last successful value/time retained, without false zero pending state

#### Scenario: Shutdown occurs during a synchronous log write
- **WHEN** shutdown cancels workers while the test sink is still writing
- **THEN** no new sample starts, shutdown waits for that write, and after the sink is released workers join before pool closure with no detached timeout worker

#### Scenario: Construction or listener startup fails
- **WHEN** construction or listener startup fails after resources have been acquired
- **THEN** owned workers/resources are stopped in the existing cleanup order and repeated stop remains safe

### Requirement: Metrics preserve the admitted command and wire contracts

For requests admitted by the new service gate, the service SHALL retain all
37 RPC descriptors, catalog/locale/RetryInfo semantics, exact permission/ownership
checks, restricted Me/ChangePassword/Logout allowlist, authoritative primary
state, and existing pagination. Register token field 1 SHALL remain empty with
PENDING registration; credentials SHALL only be returned by committed Login or
Refresh. Required mutation/audit/outbox/result atomicity, no-op/replay/version,
response-loss behavior and refresh commit-before-denial SHALL remain unchanged.
Metrics MUST NOT invalidate sessions, increase auth_version, reauthorize cached
results or imply DEP-01/broker readiness.

#### Scenario: A keyed command replays after response loss
- **WHEN** an admitted currently authorized actor retries an identical committed management key/payload
- **THEN** metrics record the new RPC invocation while the original safe result is replayed without another mutation, audit, event or entity-version increment

#### Scenario: Refresh reuse commits revocation then denies
- **WHEN** admitted RefreshSession proves consumed reuse and commits required revocation/audit/event before UNAUTHENTICATED
- **THEN** metrics report the final denial without rolling back committed security state, returning credentials or generating another attempt audit

#### Scenario: Permission changes and credential invalidation remain separate
- **WHEN** an admitted role/application mutation changes permissions or a password/global revocation/status command invalidates credentials
- **THEN** the former preserves sessions/auth_version and the latter retains its existing invalidation scope without metrics adding side effects

## Phase 5 traceability

Scope/owner: grpc-auth backend. Prerequisites: published dependency pins and
the secure37-method pipeline. P5-01..07 map to M-10/M-11 and the relevant AC
criteria without replacing their meaning. Implementation and required normal/
race PostgreSQL/workspace/standalone verification are PASS; actual test names,
source identity and command receipts are recorded in the
[Phase 5 acceptance ledger](../../changes/archive/2026-10-20-auth-phase-5-hardening-metrics/evidence/progress.md).
Actual consumer/deployment readiness remains Phase6 DEP-01; pending outbox
observations do not prove broker delivery.
