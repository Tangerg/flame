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
8. Inspect the full diff and stage explicit paths only.

Reference repositories provide evidence, not layouts to copy:

| Reference | Useful evidence |
| --- | --- |
| `/Users/tangerg/Desktop/scope` | Framework and provider boundaries, public Go API discipline |
| `/Users/tangerg/Desktop/study/codex-server` | Protocol lifecycle, interruption, compaction, and recovery |
| `/Users/tangerg/Desktop/grok-build` | Terminal interaction and presentation density |
| `/Users/tangerg/Desktop/opencode` | Provider discovery, credential precedence, and endpoints |

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

Use targeted race tests only for changed concurrent ownership. Use real PTY tests only for terminal mode, input decoding, resize, focus, or restoration. The default test suite must remain offline and credential-independent.

`runtime/config/config.yaml` may be loaded through production configuration for an explicitly requested bounded live DeepSeek check. Never print or copy its credential.
