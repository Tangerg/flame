# Flame development workflow

Repository rules live in [`AGENTS.md`](AGENTS.md), design rationale in [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md), and the structural method in [`REFACTORING.md`](REFACTORING.md). Module-specific boundaries live in [`apps/cli/ARCHITECTURE.md`](apps/cli/ARCHITECTURE.md), [`runtime/docs/ARCHITECTURE.md`](runtime/docs/ARCHITECTURE.md), and [`apps/web/ARCHITECTURE.md`](apps/web/ARCHITECTURE.md).

## Active boundary

The task defines the active boundary. A Runtime/CLI-only task does not authorize Desktop edits. Repository-wide ownership or directory work includes the requested modules and every affected in-repository consumer of a replaced contract; preserve unrelated product work.

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

| Reference                                   | Useful evidence                                                                                         |
| ------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `/Users/tangerg/Desktop/scope`              | Framework and provider ownership, released contracts, strict construction, and public Go API discipline |
| `/Users/tangerg/Desktop/study/codex-server` | Protocol lifecycle, interruption, steering, compaction bounds, recovery, and integration-test evidence  |
| `/Users/tangerg/Desktop/grok-build`         | Terminal hierarchy, interaction feedback, streaming stability, and presentation density                 |
| `/Users/tangerg/Desktop/opencode`           | Provider discovery, exact identity, credential precedence, endpoint policy, and request lowering        |

Reference code is read-only evidence. Do not copy its directory tree, private protocol, compatibility burden, or framework abstractions into Flame without a Flame-owned requirement.

## Dependency discipline

Use released Scope modules and provider libraries. Do not copy their implementations into Flame, add a local `replace`, or preserve a removed Scope API behind a compatibility wrapper. Upgrade the direct module graph first, migrate every breaking contract in the owning batch, run `go mod tidy`, and inspect the selected transitive graph instead of pinning indirect versions without evidence.

An optional provider capability remains separate from the ordinary chat contract. Advertise it only when the concrete provider implements the exact behavior; do not approximate a missing capability in a generic Flame layer.

## Verification and toolchains

Use the Go version declared in `go.work`, npm 11 or newer,
and a Node release accepted by the root `engines` constraint. CI pins Node and
npm in [the check workflow](.github/workflows/check.yml). Desktop uses the exact
Wails v3 version selected in its module; macOS
is the supported native verification and distribution platform.

Run from the repository root:

| Command                                 | Responsibility                                                                                                               |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `make setup`                            | Check the toolchain, prepare immutable candidate modules, install root npm dependencies/hooks and shared assets              |
| `make generate`                         | Invoke real Go and npm output owners in dependency order                                                                     |
| `make check`                            | SDK closure/direction guards, clean generated drift, every non-native Go member and npm workspace check                      |
| `make check-native`                     | Stage the same Web build, test/vet/build Desktop and run its Wails macOS build                                               |
| `make check-author`                     | Export independent author namespaces, pack JS dependencies and verify Go vendor execution without network or checkout access |
| `make dev-web` / `make dev-desktop`     | Browser/native development of the same Web product                                                                           |
| `make build-runtime` / `make build-cli` | Headless executable and companion plugin distribution                                                                        |
| `make package-desktop`                  | Complete Web staging and Wails/native packaging                                                                              |

The coordinator discovers Go members from `go.work` and npm members from root
`workspaces`. Native requirements classify the Desktop member explicitly; a
non-native check does not claim that member passed. CI runs clean workspace
installation and checks on macOS and Linux, plus the macOS native job.
Each Go member runs test/vet/build with `GOWORK=off` and read-only manifests, so
workspace substitution cannot hide missing dependencies. Package tests run
serially; process-backed fixtures own their compiler and backend concurrency,
and terminal output checks retain the library's delivery deadlines.
Cross-module development
and generation use the workspace. Independent distribution verification uses
the frozen standard module artifacts described in
[consumer reference](docs/reference/consumers.md).

Owner checks can be run directly, for example `go test ./runtime/...` or
`npm run typecheck --workspace @flame/web`. Use `npm ci` only at the root, then
`make setup`/`make generate` for prerequisites. Do not create package-local
locks or install graphs. `make check-generated` rebuilds in a disposable source
copy and detects added, deleted and changed generated files without rewriting
the working tree. Component builds always include current icons, UI, SDK policy,
contracts and root lock; no incomplete timestamp cache is enabled.

Only edit hand-authored sources. Registry-owned Go methods and metadata,
Runtime wire/schema/methods, SDK sample copies, plugin contracts and installed
component ESM are generated projections. Runtime owns the raw canonical JSON
samples in `runtime/contract/samples`.

Run `go mod tidy` for dependency changes and inspect every manifest/checksum
diff. Run `git diff --check`. Do not change global Go/npm configuration.

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
