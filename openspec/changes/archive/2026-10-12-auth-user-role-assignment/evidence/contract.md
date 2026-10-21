# Contract C verification

Baseline: fd1147049d52, feat/role-management,16 descriptor files/27 methods.
Only pre-existing dirty path: proto/response.json; SHA256 retained in contract-inventory.json and verified unchanged.

12-Oct-2026: additive generated contract30 methods,8 new messages,17 item fields.
All legacy messages/options/methods compared against frozen baseline; dedicated field/presence/wire tests PASS.
English/Indonesian new errors, locale fallback, identity/cause/redaction and existing mappings PASS.

- make proto twice: PASS; all22 generated hashes identical between runs.
- GOCACHE=/tmp/auth-role-build-cache go test ./...: PASS.
- GOCACHE=/tmp/auth-role-build-cache go vet ./...: PASS.
- GOWORK=off with same cache, go test ./... and go vet ./...: PASS.
- git diff --check: PASS.

Initial default-cache test could not write macOS Go cache under sandbox; rerun with controlled writable cache passed.
Publication and standalone module download evidence:

Published commit bfb073390ea97e3f63750e46747b79238c3c6d79 on origin/feat/role-management.
Standalone download PASS: v0.0.0-20261012102712-bfb073390ea9
Sum h1:u9T8xCpKPDHTj1CxsOQA7uSJ7qX7MXUBdWXAZgcaa9Q=
GoModSum h1:Jb5bNxnX/aW6YWghT/pXorXlmIDRRxXltY8gL7Udzg4=

Backend published pin baseline: workspace and GOWORK=off go test ./... and make build PASS. Initial assertion27 failed as expected with additive descriptor; updated explicit inventory30 and all30 service boundaries pass. New domain files compiled during this baseline; no assignment handler wired yet.
