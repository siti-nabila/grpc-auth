# Phase 5 runtime metrics

Status: implementation complete; final verification is tracked in the
[Phase 5 evidence](../openspec/changes/archive/2026-10-20-auth-phase-5-hardening-metrics/evidence/progress.md).
Metrics are local diagnostics, never
authentication/authorization state or delivery evidence. Counter values reset
at process restart; they are cumulative observations rather than durable audit.

## Schema and finite dimensions

One store is created by Login/foundation composition and passed to the unary
observer and runtime sampler. `telemetry.MethodNames()` returns a copy of the
fixed sorted 37-method descriptor inventory plus index37 `unknown`.
`snapshot.rpc[index]` contains numeric arrays only:

| Field | Index/meaning |
| --- | --- |
| codes | 17 standard gRPC codes, code0 OK through code16 UNAUTHENTICATED |
| auth | failure, denial, dependency; no category for unrelated outcomes |
| duration_ns | Cumulative whole-pipeline duration, including attempt auditing |
| histogram | Cumulative upper bounds1/5/10/50/100/500/1000/5000ms and overflow |

There are38×30 RPC numeric fields. Method/code/outcome indices are finite;
arbitrary strings cannot allocate series. Snapshot copies contain no user,
email, IP, session/caller identity, credential/digest, request/permission input,
correlation, DSN or private error text. Invalid code maps to UNKNOWN; negative
elapsed time clamps to zero; counter/duration accumulation saturates.
Mean duration is not a percentile. Unknown routes/TLS handshake failures that
do not enter the unary pipeline are not counted as unary completions.

Credential/service/session UNAUTHENTICATED maps to auth failure,
PERMISSION_DENIED maps to denial, UNAVAILABLE maps to dependency.
Valid EvaluateAccess allowed=false contributes denial and an OK completion.
Quota rejection contributes RESOURCE_EXHAUSTED without inventing auth failure.

## Service admission

Approved20 October2026: every registered method shares a service-ingress bucket
with **600 requests/minute, burst100 per verified service/process**. TLS/service
credential and method authentication precede admission; Attempt/Session follow
it. Current/previous credentials, methods, user IDs and forwarded IP changes do
not reset a service bucket. This includes Logout/self-admin revoke/TesRPC.

Consumed service tokens are not refunded when downstream validation/session/
identity quota rejects or a request is canceled. The eight existing identity
quota policies still apply, with Login and refresh layered admission atomic
within their own stages. Exhausted service admission returns existing
RATE_LIMITED/RESOURCE_EXHAUSTED, one ErrorInfo and rounded-up positive RetryInfo
before DB/session/attempt/business work. A100ms refill can yield1s guidance.
At the shared10000-entry cap, a new service key receives capacity/one-minute
guidance; existing active keys are not evicted. Idle TTL15m, cleanup1m.

Admission bounds admitted work, including independent audit attempts, by burst
plus refill per configured caller/process. It is not an absolute in-flight
connection/goroutine bound, distributed quota or production capacity claim.
Logout/revoke can be delayed by another method exhausting this shared budget;
rate rejection never claims revocation occurred. Admitted requests retain
required audits; no sampling/drop policy is introduced.

## Audit observations

Fixed origins: required-write, Login-attempt, security-attempt and
finalization-unknown. Required-write means AuditWriter.Create returned an error,
including record validation; it does not assert a SQL query ran. Independent
attempt operation failures are recorded by their one owning callback, without
decorating the same attempt write again. Finalization-unknown identifies commit
failure without asserting that a row was lost or rollback was guaranteed.
These origins have different denominators and are not interchangeable counts
of lost audit rows. Observation never retries a mutation or creates an audit.

## Runtime samples and summary logs

The runtime sampler reads one consistent ORM count/min aggregate of outbox
records with published_at NULL. Future retry schedules remain pending.
Pool usage comes from the existing primary DB.Stats; limiter counts come from
the one shared store. No payloads are selected and no pool is opened per tick.

Runtime fields describe pending count, oldest age, sample time/status, pool
open/in-use/idle/max and cumulative wait count/duration, limiter entries, and
service/identity quota/capacity rejection counts. `sampled_at` is Unix seconds;
`oldest_age_ns`, RPC duration and pool wait duration use nanoseconds. Divide age
by1e9 to display seconds. Pool indices are open/in-use/idle/max/wait-count/
wait-duration; rate indices are service/identity, each quota/capacity.
Status0 unavailable means no
successful sample; status1 available follows success; status2 stale retains
the last successful value/time after a read failure. Empty backlog is zero,
future creation time has age zero, and neither is a substitute for unavailable.

Sampling occurs once per minute, query timeout2s, with no overlapping samples,
catch-up queue or per-request log/DB I/O. One fixed-schema summary event uses
existing formatter/hooks/sinks. The snapshot field is a JSON payload string,
readable in text output and decoded once more when the outer formatter is JSON.
Telemetry payload maximum64KiB is verified at
worst-case numeric values; local snapshots remain available if export fails.
Collection/export failures are bounded counters without recursive raw-error
logging. Pending backlog can increase while no relay is active; it is not
evidence of RabbitMQ publication or consumer delivery.

The telemetry worker is separate from limiter cleanup. Slow log I/O does not
hold the store lock or block request admission/cleanup. Existing synchronous
log writes cannot be canceled: shutdown is allowed to wait for the current
write, then joins workers before pool closure. No detached timeout worker is
used. Actual consumer/deployment readiness remains Phase6 DEP-01.
