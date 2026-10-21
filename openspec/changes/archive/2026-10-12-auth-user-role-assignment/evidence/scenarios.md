# Delta scenario traceability

All30 scenarios map to exercised suites. Gate status and execution counts come
from [verification.json](verification.json); source alone is not PASS evidence.

| # | Exact delta scenario | Exercised test source |
| --- | --- | --- |
| 1 | A valid actor lacks the exact mutation permission | [assignments.go](../../../../../tests/features/user/test_scenarios/assignments.go), [lifecycle_registered.go](../../../../../tests/integration/auth_reads/test_scenarios/lifecycle_registered.go) |
| 2 | Service or principal is invalid or restricted | [lifecycle_registered.go](../../../../../tests/integration/auth_reads/test_scenarios/lifecycle_registered.go), [lifecycle.go](../../../../../tests/interceptors/test_scenarios/lifecycle.go) |
| 3 | Actor authority changes before mutation recheck | [assignment_race.go](../../../../../tests/integration/postgres/test_scenarios/assignment_race.go), [assignments.go](../../../../../tests/features/user/test_scenarios/assignments.go) |
| 4 | A suspended target retains revoked and disabled role assignments | [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go), [assignments.go](../../../../../tests/features/user/test_scenarios/assignments.go) |
| 5 | Target exists with no assignments or is absent | [assignments.go](../../../../../tests/features/user/test_scenarios/assignments.go), [persistence.go](../../../../../tests/repositories/assignment/test_scenarios/persistence.go) |
| 6 | A concurrent commit changes authority and assignment data | [assignment_list.go](../../../../../tests/integration/postgres/test_scenarios/assignment_list.go) |
| 7 | Required read fails after some data was loaded | [assignments.go](../../../../../tests/features/user/test_scenarios/assignments.go), [security_attempt.go](../../../../../tests/interceptors/test_scenarios/security_attempt.go) |
| 8 | Application and status filters combine with literal search | [assignment_list.go](../../../../../tests/integration/postgres/test_scenarios/assignment_list.go), [persistence.go](../../../../../tests/repositories/assignment/test_scenarios/persistence.go) |
| 9 | All public scalars and mixed nullable ties are sorted | [assignment_list.go](../../../../../tests/integration/postgres/test_scenarios/assignment_list.go), [assignment.go](../../../../../tests/repositories/domain/assignment/test_scenarios/assignment.go) |
| 10 | Cursor is reused for another target or query | [assignment.go](../../../../../tests/repositories/domain/assignment/test_scenarios/assignment.go), [assignment_list.go](../../../../../tests/integration/postgres/test_scenarios/assignment_list.go) |
| 11 | Projection hides ordering values or the final page is empty | [persistence.go](../../../../../tests/repositories/assignment/test_scenarios/persistence.go), [assignment_list.go](../../../../../tests/integration/postgres/test_scenarios/assignment_list.go) |
| 12 | Several roles are granted including a revoked pair | [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go), [assignments.go](../../../../../tests/integration/auth_reads/test_scenarios/assignments.go) |
| 13 | One role fails eligibility | [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go), [assignments.go](../../../../../tests/features/user/test_scenarios/assignments.go) |
| 14 | Administrator assigns a system IAM role | [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go) |
| 15 | Batch input or expected version is invalid | [assignment.go](../../../../../tests/handler/test_scenarios/assignment.go), [assignment.go](../../../../../tests/repositories/domain/assignment/test_scenarios/assignment.go), [assignments.go](../../../../../tests/features/user/test_scenarios/assignments.go) |
| 16 | Concurrent requests grant the same pair | [assignment_race.go](../../../../../tests/integration/postgres/test_scenarios/assignment_race.go) |
| 17 | Active revoked and absent pairs are requested together | [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go), [assignments.go](../../../../../tests/integration/auth_reads/test_scenarios/assignments.go) |
| 18 | Revoke cleans up an inactive target and disabled role | [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go) |
| 19 | Unknown role is mixed with valid assignments | [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go), [assignments.go](../../../../../tests/features/user/test_scenarios/assignments.go) |
| 20 | Administrator attempts to revoke their own platform role | [assignment_race.go](../../../../../tests/integration/postgres/test_scenarios/assignment_race.go), [assignments.go](../../../../../tests/features/user/test_scenarios/assignments.go) |
| 21 | Only one active administrator would remain unassigned | [assignment_race.go](../../../../../tests/integration/postgres/test_scenarios/assignment_race.go) |
| 22 | Revoke and status changes remove different administrators in parallel | [assignment_race.go](../../../../../tests/integration/postgres/test_scenarios/assignment_race.go) |
| 23 | Response is lost and role input order changes on retry | [assignment_race.go](../../../../../tests/integration/postgres/test_scenarios/assignment_race.go), [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go) |
| 24 | Conflicting scoped payload different target or expired result is used | [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go), [assignments.go](../../../../../tests/features/user/test_scenarios/assignments.go) |
| 25 | Valid full no-op is recorded and replayed | [assignments.go](../../../../../tests/integration/auth_reads/test_scenarios/assignments.go), [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go) |
| 26 | Required persistence or later guard fails after tentative writes | [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go), [assignments.go](../../../../../tests/features/user/test_scenarios/assignments.go) |
| 27 | Cancellation panic or uncertain commit interrupts a command | [assignment_race.go](../../../../../tests/integration/postgres/test_scenarios/assignment_race.go), [security_attempt.go](../../../../../tests/interceptors/test_scenarios/security_attempt.go) |
| 28 | One overlapping role is revoked with a valid token | [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go) |
| 29 | Shared quota is exhausted across baseline and assignment methods | [role_rate.go](../../../../../tests/interceptors/test_scenarios/role_rate.go), [assignment_management.go](../../../../../tests/integration/postgres/test_scenarios/assignment_management.go) |
| 30 | Additive contract is generated and registered | [lifecycle_registered.go](../../../../../tests/integration/auth_reads/test_scenarios/lifecycle_registered.go) |

Contract generation, compatibility27→30, publication and standalone module
resolution are recorded in [contract.md](contract.md) and the descriptor inventory.
Full regression also covers the previously shipped token/session/user/role flows.
Actual deployment and D/E/CLM are outside C; Phase6 DEP-01 remains OPEN.
