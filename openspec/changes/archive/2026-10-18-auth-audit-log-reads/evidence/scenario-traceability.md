# Audit-read scenario traceability

Baseline acceptance IDs retain their original meanings. Implementation/test results and immutable receipts are in [progress](progress.md), [backend checks](backend-checks.json), [contract checks](contract-checks.json) and the PostgreSQL receipts recorded at completion. PostgreSQL scope uses dedicated opt-in test fixtures, two physical backends and explicit barriers. Default suites use controlled dependencies.

| Spec scenario | Ledger | Required evidence |
| --- | --- | --- |
| Auditor reads another user's historical events | ALR-01 | registered audit/historical selector; feature success |
| Caller or current actor lacks authority | ALR-01 | registered audit namespace/JWT/restriction/session negatives; all37 TLS/method boundary |
| Numeric administrator or spoofed actor is presented | ALR-01 | registered audit spoofed metadata + missing permission; original principal/selector checks |
| Exact selectors combine across actor action target and time | ALR-02 | reader exact SQL arguments; PostgreSQL pages AND/presence/time |
| Filters are malformed or target type is missing | ALR-02 | domain invalid-input table; handler timestamp/presence; cursor rejection table |
| Filter matches no historical record | ALR-02 | registered empty; PostgreSQL pages unknown literal and empty range |
| Equal timestamps span pages with large IDs | ALR-02 | PostgreSQL registered tied timestamp/BIGINT pages; domain MaxInt64 cursor |
| Unsupported query capability or cursor is sent | ALR-02 | handler forbidden fields/sort/search; domain cross-method/filter/noncanonical cursor |
| Offset cursor total and terminal metadata are observed | ALR-02 | PostgreSQL fixed offset/keyset/end; reader metadata empty/overflow; domain page-size change |
| Audit insert and permission revoke commit during listing | ALR-04 | PostgreSQL count/page barrier insert/mapping/assignment/role/session/account |
| Historical user or session is inactive | ALR-03 | PostgreSQL inactive historical target/actor; safe nullable row mapping |
| Privilege changes keep the original JWT valid | ALR-01/04 | PostgreSQL same original Login claims before/after mapping/assignment/role revoke; existing full lifecycle regression |
| Nullable actors and snapshots are returned | ALR-03 | contract wire/presence; domain snapshots; handler typed mapping; PostgreSQL user/service/both actors |
| Historical snapshot contains an unexpected secret field | ALR-03 | domain strict snapshot table; registered corrupt snapshot; PostgreSQL null/array/secret/negative snapshot |
| Count page or finalization fails | ALR-04 | registered count/page/commit dependency; feature injected failures; PostgreSQL count/page driver/commit |
| Cancellation deadline or panic interrupts a read | ALR-04 | feature cancellation/deadline/panic; registered localized mapping; PostgreSQL count/page cancellation, deadline, page panic/pool reuse |
| Listing succeeds or a response is lost | ALR-05 | registered success/empty and same-key retry; PostgreSQL success business-state equality |
| Failed list needs an attempt audit | ALR-05 | registered cleanup/error/attempt failure; interceptor once/failure/panic; PostgreSQL failing audit trigger/pool reuse |
| Audit and existing reads share the same exhausted group | ALR-05 | registered audit quota mixed with old reads and malformed inputs; RetryInfo/one bucket/no DB rate audit |
| Quota is available without audit permission | ALR-01/05 | registered missing permission/restricted before data; feature stale authority |
| Prior clients invoke existing methods | ALR-06 | all37 service/method boundary;18 prior full descriptor equality; full normal/race/backend/PostgreSQL regression |
| Backend evidence passes before deployment readiness | ALR-06 | proposal/design ledger; Phase6 separation and completion evidence review |
