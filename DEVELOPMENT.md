# Flame development workflow

Repository rules live in [`AGENTS.md`](AGENTS.md), design rationale in [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md), and the structural method in [`REFACTORING.md`](REFACTORING.md). Module-specific boundaries live in [`cli/ARCHITECTURE.md`](cli/ARCHITECTURE.md), [`runtime/doc/ARCHITECTURE.md`](runtime/doc/ARCHITECTURE.md), and [`desktop/frontend/ARCHITECTURE.md`](desktop/frontend/ARCHITECTURE.md).

## Active boundary

The task defines the active boundary. A Runtime/CLI-only task does not authorize Desktop or IDE edits. Repository-wide ownership or directory work includes the requested modules and every affected in-repository consumer of a replaced contract; preserve unrelated product work.

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

Application development uses the repository's `go.work`. Run checks from each owning
module with the workspace enabled:

```sh
go test ./...
go vet ./...
go build ./...
```

Coordinated changes use the participating modules' local sources; publishing a Runtime
module is not a prerequisite for workspace verification. For an independent module
release, disable the workspace and verify the published dependency graph:

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

The graphical client has one required gate, run from `desktop/frontend`:

```sh
npm run check
```

Its TypeScript dependencies are installed after `runtime/contract/typescript` and after `icons` has been installed and built. The desktop task graph performs both prerequisites. For direct frontend commands, run `npm ci && npm run build` from `icons` first. Run the shared client's `npm run check` when changing that boundary, and `npm run check` from `ide` when its consumers are affected. Wails native builds follow the supported targets in `desktop/Taskfile.yml`; a browser build does not prove a native package works.

Run `go generate ./...` in Runtime when the protocol catalog changes. Run `go mod tidy` only when imports or dependencies change, and inspect any `go.mod`, `go.sum`, or `go.work.sum` changes before keeping them. Always run `git diff --check`.

Use targeted race tests only for changed concurrent ownership. Do not invent multi-Runtime, multi-client, or multi-server coverage when the product path has one Runtime and one CLI. Use real PTY tests only for terminal mode, input decoding, resize, focus, or restoration. The default test suite must remain offline and credential-independent.

Use focused tests for owner invariants and deterministic failure ordering, then exercise real lifecycle slices through the public binding or protocol. The high-value matrix is Goal creation and completion, Plan replacement and update, steer while running, interruption and resume, context compaction, long context, long execution, restart, and recovery. Test hard bounds and malformed inputs at their owner rather than padding the suite with implementation inventories.

`runtime/config/config.yaml` may be loaded through production configuration for an explicitly requested bounded live DeepSeek check. Use the production bootstrap and public binding or protocol, cover both success and provider-error paths, and never print or copy its credential. A scheduled build-cache cleanup is not a product failure; rebuild and continue without investigating it.

Commits and tests are the progress record. Do not add temporary audit reports, completed-plan documents, capability ledgers, or generated inventories to the repository.

## Pre-release data policy

Breaking pre-release storage changes replace the schema and all in-scope consumers in
one batch. Runtime refuses an incompatible data directory; opening it does not convert
installation declarations, tool policy, OAuth grants or waiting checkpoints. Use a fresh
data directory and retain the old directory separately. Do not add compatibility readers,
dual persistence, implicit migration or fallback decoding to make old state appear valid.
Completed historical content remains readable when its current owner can decode it.
