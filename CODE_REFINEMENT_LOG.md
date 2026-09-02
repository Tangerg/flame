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

## Round 3 — complete

### Audit scope and evidence

- Audited the remaining text/reasoning streaming clone together with open-item identity, authoritative completion, delta types, and reducer tests.
- Text and reasoning deliberately use distinct closed `ItemDelta` variants, completion constructors, mutable slots, and transcript kinds. A shared append helper would accept independently variable kind, delta, and state-slot parameters, making mismatched states constructable. The symmetric concrete paths are retained.
- Measured production cognitive and cyclomatic complexity with `golangci-lint` (`gocognit` and `gocyclo`, tests excluded). Runtime reported six functions above the default cognitive threshold; CLI reported one. No cyclomatic threshold violation was reported.
- `InterruptAnswer.validateResolution` measured 38 because it contains the complete Approval and Question validation bodies under one kind switch. These are already a closed two-variant family with concrete `transcript.Approval` and `transcript.Question` payloads, so each branch can own its invariant without a new abstraction or dynamic parameterization.
- The remaining measured functions coordinate sequential rollback, bootstrap, reflection-boundary, generated-contract, or UI settlement lifecycles; this round will not split those until evidence identifies a coherent owner rather than merely a long function.
- Baseline: `GOWORK=off go test -count=1 ./internal/application/agent/runs` passed in 1.81s.

### Root cause

Kind-specific answer rules were centralized into one validation entry point, but the implementation stopped at the dispatch switch. Approval and Question rules continued to contribute nested branches to one function even though their payload types and invariants are independent after dispatch.

### Impact and acceptance criteria

- Preserve every existing validation rule, error string, wrapping category, answer index, and resolution call path.
- Keep `validateResolution` as the single kind dispatcher while moving each variant's rules to a payload-typed private method.
- Eliminate the measured cognitive-complexity finding without adding interfaces, configuration values, or package boundaries.
- Pass focused answer/reducer tests normally and under the race detector, Runtime and current-workspace CLI full gates, plus one bounded live Run.

### Plan

- **Completed:** extracted Approval and Question resolution validation into private methods accepting their concrete prompt types; left the unknown-kind branch at the dispatcher.
- **Completed:** formatted, remeasured complexity, ran focused and full verification, exercised the authorized DeepSeek path, cleaned resources, inspected the diff, and recorded compatibility.

### Validation

- The post-change `gocognit`/`gocyclo` scan no longer reports `validateResolution`; Runtime findings fell from six to five, with no new finding and no cyclomatic threshold violation.
- Focused Approval/Question tests passed in 4.11s; the full runs package passed in 4.57s and with `-race` in 8.50s. Focused `go vet` and `staticcheck` passed.
- Runtime `GOWORK=off go vet ./...` passed in 4.83s, `GOWORK=off go build ./...` passed in 11.51s, and uncached `GOWORK=off go test -count=1 ./...` passed in 52.10s.
- Current-workspace CLI `go vet ./...` passed in 3.72s, `go build ./...` passed in 6.02s, and uncached `go test -count=1 ./...` passed in 41.06s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND3_OK`, status `completed`, one step, 9,185 input tokens, 8 output tokens, 9,088 cache-read tokens, and 568ms model duration; the complete command took 4.19s. No credential value was printed or copied.
- `git diff --check` passed.

### Changes and compatibility

- `validateResolution` now performs only closed-kind dispatch and keeps the unknown-kind error at the common boundary.
- `validateApprovalResolution` accepts only an Approval prompt and owns approval-only fields, remember policy, edited arguments, and denial-reason rules.
- `validateQuestionResolution` accepts only a Question prompt and owns acknowledgement, field exclusion, answer cardinality, and indexed field validation.
- Breaking changes: none. All methods are package-private; validation order inside each variant, errors, wrapping, answer indices, protocol values, and persistence are unchanged.

### Resource cleanup

- The bounded live Run used a validated `/tmp/flame-live-round3.*` directory and moved it to the system Trash on exit.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- The remaining risk is limited to accidental rule movement during extraction; existing kind-shape tables, ordered answer tests, claim revalidation, race coverage, and full lifecycle tests passed.
- Round 4 will audit the measured Session rollback orchestration and its boot-recovery counterpart for duplicated workspace-transition ownership. It will preserve the explicit temporal sequence unless a coherent shared phase can be proven.

## Round 4 — complete

### Audit scope and evidence

- Traced `sessions.rollback` from protocol validation through delivery translation, the Session application command, workspace admission, recoverable mutation log, checkpoint restore, history truncation, boot recovery, and CLI consumers.
- The protocol already owns a closed three-value vocabulary: `history`, `files`, and `both`. Delivery expands that value into a private `rollbackIntent` pair of booleans, then copies the pair into `sessions.RollbackSpec` as two more booleans.
- Those stored booleans admit an unrequested no-op state and make `both` a convention that two independently writable fields happen to be true. The use case has no validation for either condition.
- `WorkspaceMutation.RestoreHistory` is not the same defect: a workspace mutation exists only for file restore, so its one boolean selects the optional second durable phase without encoding a multi-axis state.
- The live and recovery workspace sequences differ legitimately in guards and failure cleanup, but direct inspection proves two identical ordered phases: quiescing Session/workspace state before file restore, and invalidating workspace evidence plus discarding the sandbox after a successful restore.
- After the scope migration, `Rollback`'s measured cognitive complexity rose from 34 to 35 because command validation added a branch while the existing procedural phases remained inline. The refinement must not trade a valid state model for a harder use case.
- Baseline: `GOWORK=off go test -count=1 ./internal/application/agent/sessions ./internal/delivery` passed in 8.09s.

### Root cause

Delivery decoded a closed wire enum into procedural booleans instead of preserving the same vocabulary at the application boundary. `RollbackSpec` then exposed those booleans as writable command state, so the semantic owner could neither validate nor exhaustively classify the requested rollback mode.

### Impact and acceptance criteria

- Introduce one closed Session-application restore scope with exactly the history, files, and both values and intention-revealing queries.
- Make `RollbackSpec` store only that scope; reject invalid scopes and missing file targets before reading or mutating Session state.
- Translate the wire enum directly to the application scope, delete `rollbackIntent`, and migrate delivery projection plus all application branches.
- Preserve protocol JSON, defaults, capability constraints, mutation records, rollback ordering, error mapping, and CLI contracts.
- Pass scope invariants, focused history/files/both and recovery tests, Runtime and current-workspace CLI full gates, and one bounded live Run.

### Plan

- **Completed:** added `RestoreScope`, validation, and derived queries; replaced the two `RollbackSpec` booleans throughout the use case.
- **Completed:** removed the delivery boolean representation and migrated the handler to direct vocabulary-preserving scope translation.
- **Completed:** centralized the proven pre-restore quiesce, mutation-log admission, and post-restore workspace-retirement phases while keeping their order visible in live and recovery orchestration.
- **Completed:** added owner-invariant tests, formatted, remeasured complexity, ran focused/full/live verification, cleaned resources, and inspected compatibility.

### Validation

- The final `gocognit`/`gocyclo` scan no longer reports `Rollback`; Runtime findings fell from five to four, with no new finding and no cyclomatic threshold violation.
- Final focused Session+delivery tests passed in 18.25s; targeted rollback integration tests passed in 12.55s; the same Session+delivery packages passed with `-race` in 33.37s. Focused `go vet` and `staticcheck` passed.
- Runtime `GOWORK=off go vet ./...` passed in 3.13s, `GOWORK=off go build ./...` passed in 6.93s, and uncached `GOWORK=off go test -count=1 ./...` passed in 54.99s.
- Current-workspace CLI `go vet ./...` passed in 2.17s, `go build ./...` passed in 4.03s, and uncached `go test -count=1 ./...` passed in 39.86s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND4_OK`, status `completed`, one step, 9,185 input tokens, 8 output tokens, 9,088 cache-read tokens, and 839ms model duration; the complete command took 4.08s. No credential value was printed or copied.
- `git diff --check` passed. Protocol generation was not applicable because no protocol type, schema, constraint, or published contract changed.

### Changes and compatibility

- `RestoreScope` now owns the closed history/files/both application vocabulary, validity, and resource queries.
- `RollbackSpec` stores one scope and rejects invalid scope values or file restores without a target before reading Session state.
- Delivery now validates/defaults the wire value and converts it directly to the application scope; the `rollbackIntent` boolean representation was deleted.
- Live and recovery rollback now share one pre-restore quiesce phase and one post-restore workspace-retirement phase. The recoverable mutation-log admission is one named operation.
- Breaking changes: the internal `sessions.RollbackSpec` command shape intentionally replaced `RestoreFiles`/`RestoreHistory` with `Scope`; its only production consumer was migrated and the obsolete fields and delivery type were removed. Public Go binding requests, Runtime Protocol JSON, defaults, constraints, persistence, CLI contracts, and error categories are unchanged.

### Resource cleanup

- The bounded live Run used a validated `/tmp/flame-live-round4.*` directory and moved it to the system Trash on exit.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- Delivery's direct string conversion intentionally depends on the shared history/files/both vocabulary; wire constraint tests, scope owner tests, three-mode integration tests, recovery tests, and full binding consumers passed.
- Shared rollback phase errors now use the same file-rollback context in live and recovery paths while preserving wrapped causes and protocol categories.
- Round 5 will audit the remaining measured complexity at strict JSON-null rejection and contract-shape validation, prioritizing a concrete boundary invariant over mechanical function splitting.

## Round 5 — complete

### Audit scope and evidence

- Traced strict parameter decoding from the dispatch router through `encoding/json`, unknown-field rejection, explicit-null traversal, generated wire validation, error projection, and representative protocol map fields.
- Opaque `json.RawMessage` and interface values intentionally admit null. Typed maps such as MCP headers/environment, feature capabilities, and per-model usage do not: a JSON null would otherwise silently become the Go zero value.
- Struct traversal uses declaration-ordered contract fields and arrays use index order, but typed-map traversal ranges directly over a Go map. When several values are null, the same request can therefore report different first failing paths across executions.
- The unstable path is observable boundary behavior and conflicts with stable field diagnostics. It is independent of performance; no parser rewrite or cache is justified.
- Baseline: `GOWORK=off go test -count=1 ./internal/delivery/dispatch` passed in 1.67s.

### Root cause

The null rejector reparses each typed map into `map[string]json.RawMessage` and immediately recurses over its iteration order. Go deliberately does not define that order, while the function returns on the first invalid child and thereby exposes it as error ordering.

### Impact and acceptance criteria

- Sort typed-map keys before recursive null validation so the first field path is deterministic.
- Keep structs, slices, arrays, byte slices, custom JSON decoders, raw messages, and interface/opaque maps unchanged.
- Add a request-level regression test with two null map entries and preserve the existing test that allows null inside open tool arguments.
- Pass focused dispatch tests normally and under the race detector, Runtime and current-workspace CLI full gates, plus one bounded live Run.

### Plan

- **Completed:** made typed-map recursion lexicographic using the current Go standard library and added the deterministic diagnostic test.
- **Completed:** formatted, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- Dispatch tests passed in 3.05s; the deterministic map diagnostic test passed 100 consecutive runs in 3.24s; the full dispatch package passed with `-race` in 6.15s. Focused `go vet` and `staticcheck` passed.
- Runtime `GOWORK=off go vet ./...` passed in 0.89s, `GOWORK=off go build ./...` passed in 3.90s, and uncached `GOWORK=off go test -count=1 ./...` passed in 38.45s.
- Current-workspace CLI `go vet ./...` passed in 0.56s, `go build ./...` passed in 2.63s, and uncached `go test -count=1 ./...` passed in 39.45s.
- The first live-validation build observed a shared Go build-cache file disappear during linking and therefore produced no binary; its trap cleaned the temporary directory. The isolated retry used a task-owned `GOCACHE` and completed successfully.
- The current-source CLI then completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND5_OK`, status `completed`, one step, 9,185 input tokens, 8 output tokens, 9,088 cache-read tokens, and 1,440ms model duration; the Run command took 5.78s. No credential value was printed or copied.
- `git diff --check` passed. Protocol generation was not applicable because the published wire shape did not change.

### Changes and compatibility

- Typed-map null traversal now visits `maps.Keys` through `slices.Sorted`, making the first rejected field lexicographically stable.
- Opaque interfaces and raw JSON still bypass typed traversal, so third-party tool arguments retain explicit null values.
- Breaking changes: none. Request acceptance, error category and text format, protocol values, schemas, and decoded values are unchanged; only competing invalid map children now have deterministic precedence.

### Resource cleanup

- Both live attempts used validated `/tmp/flame-live-round5.*` directories; the successful retry also contained its own Go build cache. Traps moved every directory to the system Trash.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- Typed protocol maps are small control-plane values and sorting changes no accepted payload. Regression coverage includes typed maps, nested pointer fields, and opaque maps.
- Round 6 will inspect contract-shape specification validation and generator constraint compilation for a similarly concrete ordering or ownership defect before considering structural splitting.
