# Flame development workflow

Repository rules live in [`AGENTS.md`](AGENTS.md), design rationale in [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md), and the structural method in [`REFACTORING.md`](REFACTORING.md). Module-specific boundaries live in [`cli/ARCHITECTURE.md`](cli/ARCHITECTURE.md) and [`runtime/doc/ARCHITECTURE.md`](runtime/doc/ARCHITECTURE.md).

## Active boundary

Current Runtime and CLI refactoring may change `cli`, `runtime`, `runtime/localruntime`, and the repository documents that describe them. Treat all Desktop changes as unrelated user work unless the user explicitly expands the scope.

Breaking changes are authorized. Migrate all in-scope consumers and leave one current shape.

## Workflow

1. Inspect `git status` before each batch.
2. Trace the real call path and identify the semantic owner.
3. Search source, dynamic entrypoints, storage, protocol catalogs, generated artifacts, tests, and docs.
4. Run a focused baseline.
5. Repair one ownership boundary and remove the obsolete contract completely.
6. Search again for retired names and paths.
7. Run focused checks, then proportionate module checks.
8. Inspect the full diff, stage explicit paths only, and commit and push the verified batch before starting another risky batch.

Reference repositories provide evidence, not layouts to copy:

| Reference | Useful evidence |
| --- | --- |
| `/Users/tangerg/Desktop/scope` | Framework and provider ownership, released contracts, strict construction, and public Go API discipline |
| `/Users/tangerg/Desktop/study/codex-server` | Protocol lifecycle, interruption, steering, compaction bounds, recovery, and integration-test evidence |
| `/Users/tangerg/Desktop/grok-build` | Terminal hierarchy, interaction feedback, streaming stability, and presentation density |
| `/Users/tangerg/Desktop/opencode` | Provider discovery, exact identity, credential precedence, endpoint policy, and request lowering |

Reference code is read-only evidence. Do not copy its directory tree, private protocol, compatibility burden, or framework abstractions into Flame without a Flame-owned requirement.

## Dependency discipline

Use released Scope modules and provider libraries. Do not copy their implementations into Flame, add a local `replace`, or preserve a removed Scope API behind a compatibility wrapper. Upgrade the direct module graph first, migrate every breaking contract in the owning batch, run `go mod tidy`, and inspect the selected transitive graph instead of pinning indirect versions without evidence.

An optional provider capability remains separate from the ordinary chat contract. Advertise it only when the concrete provider implements the exact behavior; do not approximate a missing capability in a generic Flame layer.

## Verification

Run checks from the owning module. For standalone module behavior, disable the workspace:

```sh
cd runtime
GOWORK=off go test ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...

cd ../cli
GOWORK=off go test ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
```

Run `go generate ./...` in Runtime when the protocol catalog changes. Run `go mod tidy` only when imports or dependencies change, and inspect any `go.mod`, `go.sum`, or `go.work.sum` changes before keeping them. Always run `git diff --check`.

Use targeted race tests only for changed concurrent ownership. Do not invent multi-Runtime, multi-client, or multi-server coverage when the product path has one Runtime and one CLI. Use real PTY tests only for terminal mode, input decoding, resize, focus, or restoration. The default test suite must remain offline and credential-independent.

Use focused tests for owner invariants and deterministic failure ordering, then exercise real lifecycle slices through the public binding or protocol. The high-value matrix is Goal creation and completion, Plan replacement and update, steer while running, interruption and resume, context compaction, long context, long execution, restart, and recovery. Test hard bounds and malformed inputs at their owner rather than padding the suite with implementation inventories.

`runtime/config/config.yaml` may be loaded through production configuration for an explicitly requested bounded live DeepSeek check. Use the production bootstrap and public binding or protocol, cover both success and provider-error paths, and never print or copy its credential. A scheduled build-cache cleanup is not a product failure; rebuild and continue without investigating it.

Commits and tests are the progress record. Do not add temporary audit reports, completed-plan documents, capability ledgers, or generated inventories to the repository.
