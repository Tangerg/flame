# Code Refinement Log

## Round 1 — complete

### Audit scope and evidence

- Scope: `runtime/internal/infra/filesystem/fileobservation`, including its production consumers and focused tests.
- The exact-file `watch` and tree `treeWatch` independently own the same fsnotify lifecycle: watcher handle, watched-directory set, debounce/retry loop, stop and join channels, close-once state, lock, closed flag, directory replacement, and shutdown.
- `dupl -t 120 runtime cli` independently identified the duplicated directory-replacement bodies. Direct inspection shows the surrounding run and close lifecycles are also equivalent. Git blame traces both copies to the initial implementation rather than to a current requirement for independent policies.
- The two scanners remain intentionally different: exact paths own one fingerprint each, while trees own content-derived snapshots and selective acceptance. This round will not merge those representations.
- Baseline: `GOWORK=off go test ./...` passed in Runtime and CLI; focused `GOWORK=off go test ./internal/infra/filesystem/fileobservation -count=1` passed in 3.428s. `staticcheck ./...` passed in both modules. The installed `deadcode -test ./...` crashed inside `golang.org/x/tools/go/callgraph/rta` for both modules and produced no candidate evidence.

### Root cause

Two observation modes were added as separate concrete watchers even though their variability is limited to reconciliation. The shared fsnotify resource lifecycle was copied along with the distinct scan policies, creating two owners for one technical mechanism.

### Impact and acceptance criteria

- Production behavior and public APIs remain unchanged; no breaking change is intended.
- Exactly one implementation owns the fsnotify event loop, retry timer, watched-directory set, and close/join sequence.
- Both existing exact-file and tree observation suites continue to pass, including missing paths, symlink identities, selective acceptance, dynamic tree changes, non-regular paths, and close joining.
- Runtime builds, vets, and tests cleanly; a real CLI/server lifecycle exits without leaving watcher processes or temporary state behind.

### Plan

- **Completed:** introduced one package-private observer lifecycle that owns fsnotify construction, debounce/retry execution, watched-directory replacement, selective baseline acceptance, and close/join behavior.
- **Completed:** migrated exact-file and tree observers to that owner without changing their target validation, fingerprinting, acceptance, or notification semantics.
- **Completed:** removed the duplicated lifecycle fields, event loops, acceptance parsing, directory replacement, and close methods.
- **Completed:** ran formatting, focused tests, Runtime and CLI static/build/test gates, the real Runtime binding authored-file lifecycle, isolated real CLI discovery/catalog commands, and a bounded live DeepSeek Run.
- **Completed:** recorded results, resource cleanup, remaining risk, and the next audit direction.

### Validation

- Focused package tests passed normally (3.334s) and with `-race` (4.344s).
- Runtime format check passed in 0.01s; full `GOWORK=off go vet ./...` passed in 1.83s; full `GOWORK=off go build ./...` passed in 9.68s; uncached full `GOWORK=off go test -count=1 ./...` passed in 54.61s.
- With the current workspace Runtime selected, CLI full `go vet ./...` passed in 7.78s, `go build ./...` passed in 14.92s, and uncached `go test -count=1 ./...` passed in 53.61s.
- The uncached CLI Runtime binding lifecycle test, including external authored-file invalidations, passed in 1.213s.
- The first isolated real-CLI attempt built successfully in 9.40s, then failed before discovery because the intentionally empty environment omitted Runtime's required default provider: `config: provider is required`. The temporary directory cleanup trap ran. The rerun injected `deepseek/deepseek-chat`: `runtime info --json` completed in 0.99s and advertised `fileWatch`; `sessions ls --json` completed in 0.07s with an empty catalog.
- After the user authorized the development DeepSeek configuration, the current-source CLI completed the bounded prompt `Respond with exactly FLAME_LIVE_OK. Do not call tools.` through production bootstrap and the public binding. It returned `FLAME_LIVE_OK`, status `completed`, one step, 7,887 input tokens, 5 output tokens, and 561ms model duration; the complete CLI command took 2.35s. No credential value was printed or copied.
- `git diff --check` passed. A post-change `dupl` scan confirms the fsnotify event loop, directory replacement, acceptance parsing, and close/join copies are gone. It retains one constructor-shaped match between `Watch` and `WatchTrees`; that wiring is intentionally separate because the functions validate and instantiate different target/scanner owners.

### Changes and compatibility

- Added one package-private `observerLifecycle` for fsnotify construction, event debounce/retry, selective acceptance parsing, watched-directory replacement, initialization failure cleanup, and close/join.
- Exact-file and tree observers now supply only their reconciliation policies and retain their distinct fingerprint/snapshot facts.
- The three production files total 758 lines, down from 836 lines across the former two implementations: a net reduction of 78 lines while removing one lifecycle representation.
- Breaking changes: none. Public APIs, wire values, persistence, target validation, notifications, and error categories are unchanged. Initialization cleanup now uses the correct `observe trees` context for tree watcher close failures.

### Resource cleanup

- Every test-owned watcher was closed by its existing cleanup.
- Both isolated CLI attempts used validated `/tmp/flame-round1.*` or `/tmp/flame-live-round1.*` directories and moved them to the system Trash after completion or failure.
- No Flame CLI/server process or matching temporary directory remained after validation. No shared cache, global dependency, user Runtime data, or user configuration was removed.

### Remaining risk and next direction

- Existing tests exercise the shared lifecycle through both observer modes, including a focused race run. The residual risk is limited to platform-specific fsnotify behavior already behind the same tested abstraction.
- Round 2 will audit other production duplicate groups from the scanner, starting with MCP server mutation flows and strict decode paths. Small symmetric boundary adapters will be kept unless consumer and ownership evidence proves a real duplicate fact or lifecycle.
