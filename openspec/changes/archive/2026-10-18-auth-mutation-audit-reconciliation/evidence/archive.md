# Archive verification

Archived `auth-mutation-audit-reconciliation` on 2026-10-18 using schema `spec-driven`, with all four artifacts and 23/23 tasks complete.

## Spec synchronization

The [main command-persistence spec](../../../../specs/command-persistence/spec.md) contains the five modified requirements and all 34 delta scenarios. All 29 original scenarios, six other requirements and the Purpose remain intact; 21 scenarios were added, giving 11 requirements and 50 scenarios overall. The historical foundation follow-up note is identified as historical and current status links to [implementation evidence](progress.md).

## Evidence integrity

All historical JSON receipts/manifests and `.openspec.yaml` were preserved byte-for-byte. Existing Markdown navigation was adjusted for the extra archive directory; the source plan and main spec now point to this archive. Historical source manifests and finalization receipts describe the implementation verification snapshot, before these documentation navigation changes, and were not regenerated.

The source plan SHA-256 after its navigation update is `4602e6d0d1dd73d1003a6764142278cfaa2211b8446377c515f5ca4da5a1e2a5`. The synchronized main spec SHA-256 is `dc6600e394f7218919ebc65f5f2eb108792263d24f6ac1a6c742756ffd7dcfd5`. All 467 Go files still match [the verified code snapshot](code-snapshot.json), whose aggregate SHA-256 remains `e4e69276962d8f4b19ff453976c1d5d55940e7eebf88710a3bc3d7d51b81a2ed`.

## Archive checks

Strict main-spec validation passed for all 24 specifications. Local Markdown files and anchors were checked after relocation. No active change remains. Go/PostgreSQL tests were not rerun for this documentation-only archive; their current implementation receipts remain in [backend checks](backend-checks.json) and [PostgreSQL checks](postgres-checks.json). Phase 5 broader hardening, Phase 6 DEP-01, REST/UI and broker delivery remain separate.
