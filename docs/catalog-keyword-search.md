# Catalog keyword search

Implementation contract for [the source plan](catalog-keyword-search-plan.md).
Database verification and rollout status are tracked in
[change evidence](../openspec/changes/catalog-keyword-search-plan/evidence.md).

| Full RPC | Permission | Projection / source key | Initial document |
| --- | --- | --- | --- |
| `/role.v1.RoleService/ListRoles` | `iam.role.read` | `role_search.role_id` -> `role.id` | Role name/code |
| `/permission.v1.PermissionService/ListPermissions` | `iam.permission.read` | `permission_search.permission_id` -> `permissions.id` | Permission code |
| `/application.v1.ApplicationService/ListApplications` | `iam.permission.read` | `application_search.application_id` -> `applications.id` | Application code/name |
| `/user.v1.UserService/ListUserRoles` | `iam.user.read` | `user_role_search.user_role_id` -> `user_role.id` | Assigned role name |

All methods retain verified service/TLS, ACTIVE nonrestricted user/session,
current permission, primary snapshot and protected-read quota. Assignment target
is separate from actor identity; application/status/target filters remain AND
with the grouped search alternatives. `ListUserApplicationAccess` is unchanged.

## Request and compatibility

Protobuf JSON (not a new REST endpoint):

```json
{"search":{"fields":["keyword"],"keyword":"senior-reviewer","mode":"SEARCH_MODE_FULL_TEXT_TRIGRAM"}}
```

New keyword search requires exactly the logical field `keyword` and explicit
FULL_TEXT or FULL_TEXT_TRIGRAM. FULL_TEXT uses plain token AND in the vector.
Combined mode uses all token prefixes with AND, OR a literal substring in lexeme
text. It does not promise typo correction, relevance ranking or source phrase
ordering. Dot, hyphen and whitespace delimit words in both documents and queries.
Other punctuation cannot introduce operators or wildcard expansion.

Missing, empty and whitespace-only keyword means no search. Nonblank input that
produces no lexemes remains a filter and can return no matches. Both raw and
normalized keyword are bounded to 200 Unicode characters. `keyword` is not a
projection/sort field and cannot be mixed with physical search fields.

Existing contains/prefix, unspecified-mode default contains, explicit source
fields, trim, filters, projection, pagination and serialized legacy cursors stay
compatible. For example legacy permission description and assignment role code
search still work; those fields are absent from the initial keyword documents.
New search uses opaque version2 cursors bound to canonical whitespace input,
mode/field, method, target, filters/sort and `catalog-keyword-v1` semantics.
Numeric cursors remain legacy-only. Reset cursor when changing a query or
revision, including equivalent but differently spelled separators/case.
Page/limit/projection do not change the cursor binding; snapshots do not span
separate requests.

## Ownership and rollout

Runtime maps only feature/table/key/vector/text. Source composition belongs to
database refresh functions. To add a source, update its formula, relevant
triggers and backfill existing documents; the same request and binary then find
the additional content without a Go business-source whitelist.

Apply the reviewed [migration package](sql/catalog-keyword-search/README.md),
complete verification, then opt in consumers. Roll back callers to old modes
first and retain projections during compatibility. Actual rest-orc cutover is
separate Phase6 DEP-01 work; this backend change does not edit rest-orc.

Users retain email/name/phone/digit-only phone baseline and existing pagination.
The wrapper definitions supplied by the user on 23 October 2026 refresh every
profile with `user_id=NEW.id` after an auth email update and refresh `NEW.id`
after profile insert/user_id/name/phone updates. The dedicated fixture copies
those bodies and the supplied refresh formula without changing users source
code, PK/FK or catalog delimiter semantics. Regression verifies full/partial
email (including plus/underscore), formatted/digit-only phone, name, email and
profile changes, ownership reassignment and unchanged user cursor metadata.
The initial regression proves those definitions in an owned fixture. On
23 October 2026 the application database `user.public` was also inspected and
the additive catalog migration/backfill/indexes installed at the user's request.
Existing users functions/triggers were verified unchanged; aggregate projection
checks passed. Consumer cutover remains separate; see the operation record in
the [evidence](../openspec/changes/catalog-keyword-search-plan/evidence.md).
