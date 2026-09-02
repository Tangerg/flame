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

## Round 2 — complete

### Audit scope and evidence

- Audited the production clone groups in MCP connection mutation, Run projection, memory curation, CLI Runtime ports, and artifact/storage usage decoding.
- The MCP headers and stdio environment resolvers intentionally preserve different transport vocabulary and target-change errors. A shared resolver would require a parameter bag or primitive error-message switches for only two boundary values, so this parallel adapter shape is retained.
- Memory curation's adapter-side `agentMemory` port and application-side `CurationStore` port reverse different dependencies; CLI's command and terminal Runtime ports similarly belong to separate consumers. Their matching method sets are contracts, not duplicate state owners.
- Artifact import and SQLite restoration decode distinct representations at separate trust boundaries. Their usage projections remain local so neither adapter depends on the other.
- The Run reducer has four independent literals for the same base `EventCommit` identity: three lazy durable-attachment paths and ordinary event projection. That identity is one persisted fact owned by the reducer and can drift if the commit shape evolves.
- Baseline: `GOWORK=off go test -count=1 ./internal/application/agent/runs` passed in 1.46s.

### Root cause

The reducer publication split introduced attachment-specific helpers but left base commit construction copied into each path. The different payload append operations are real variability; the Run, Session, and Segment identity construction is not.

### Impact and acceptance criteria

- Keep projection order, park boundaries, cloning behavior, validation errors, and public contracts unchanged.
- Give base `EventCommit` identity exactly one constructor in the reducer.
- Migrate ordinary event projection and every lazy attachment path to that constructor; remove all repeated identity literals in the package.
- Pass focused reducer tests normally and under the race detector, then Runtime and current-workspace CLI build/vet/test gates plus one bounded live Run.

### Plan

- **Completed:** added one package-private base commit constructor and migrated all four reducer-owned construction sites.
- **Completed:** added one lazy last-event commit path and migrated every durable attachment to it.
- **Completed:** formatted, ran focused and full gates, exercised the production binding with the authorized DeepSeek configuration, cleaned resources, inspected the final diff, and recorded risk/compatibility.

### Validation

- Final focused reducer tests passed normally in 2.63s and with `-race` in 5.57s; focused `go vet` and `staticcheck` also passed.
- Final Runtime `GOWORK=off go vet ./...` passed in 2.93s, `GOWORK=off go build ./...` passed in 6.04s, and uncached `GOWORK=off go test -count=1 ./...` passed in 33.98s.
- Final current-workspace CLI `go vet ./...` passed in 2.27s, `go build ./...` passed in 4.45s, and uncached `go test -count=1 ./...` passed in 41.01s.
- An earlier Runtime test run launched concurrently with build and vet observed `TestInteractionExecutorProjectsConcurrentDelegateSiblingsExactlyOnce` settle one sibling as `lost`. The exact test then passed ten consecutive isolated runs in 4.83s, a serial full Runtime rerun passed, and the final full Runtime run passed. No reducer failure reproduced.
- The final current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND2_FINAL_OK`, status `completed`, one step, 9,187 input tokens, 10 output tokens, 9,088 cache-read tokens, and 879ms model duration; the complete command took 4.37s. No credential value was printed or copied.
- `git diff --check` passed. A focused `dupl -t 120` scan no longer reports `reducer_projection.go`; only the separately audited text/reasoning streaming pair remains in the reducer package.

### Changes and compatibility

- `newEventCommit` is now the sole reducer-owned constructor for the base Run, Session, and Segment commit identity.
- `ensureLastEventCommit` is now the sole lazy attachment path for the last ordinary reduction.
- The distinct item, conversation, invocation, progress, and park payload policies remain explicit at their owning call sites.
- Breaking changes: none. Exported APIs, persisted schemas, protocol values, event order, validation categories, and projection payloads are unchanged.

### Resource cleanup

- Both bounded live Runs used validated `/tmp/flame-live-round2.*` directories and moved them to the system Trash on exit.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- The pointer-valued constructor preserves the former escaping local pointer semantics and is covered by focused race tests, full persistence/projection tests, CLI consumption, and a real model Run.
- Round 3 will audit the remaining reducer streaming clone and adjacent open-item lifecycle before deciding whether text and reasoning share a real owner or merely a symmetric event shape.
