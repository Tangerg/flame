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

## Round 6 — complete

### Audit scope and evidence

- Traced `AllowedValueSet` from shape-registration validation through generated Go validators and conditional JSON Schema branches.
- The registry currently checks the target with `contractshape.Deref`, which unwraps both pointers and slices so nested paths can traverse collection elements. That path-navigation policy incorrectly makes `[]string` look like a scalar string when validating an allowed-value set.
- Generated Go validation reads the actual field and requires `reflect.String`; an array therefore fails every applicable allowed-value rule. Generated JSON Schema likewise attaches a string-valued `enum` to the array node, so no array instance can satisfy it.
- Existing first-party registrations happen to target only scalar fields, but the semantic owner accepts metadata that its two projections cannot implement. Rejecting the malformed declaration at registration keeps invalid contract states unconstructable.
- Baseline: `GOWORK=off go test -count=1 ./internal/delivery/dispatch ./cmd/contractgen` passed in 17.57s.

### Root cause

Allowed-value type validation reused a traversal helper whose broader contract deliberately removes slice layers. Path traversal and leaf cardinality are different facts: a path may cross a collection, but the field narrowed by one allowed-value set must itself be one scalar string value.

### Impact and acceptance criteria

- Accept scalar string and pointer-to-string fields exactly as before, including named string types.
- Reject slice or array fields during shape registration before artifact generation.
- Preserve current first-party generated artifacts, registry order, diagnostics for other malformed specifications, Runtime protocol values, and public bindings.
- Add a regression test proving a string slice cannot be value-restricted, then pass focused dispatch/generator tests, generation drift, Runtime and current-workspace CLI full gates, plus one bounded live Run.

### Plan

- **Completed:** added the failing shape-metadata regression, then narrowed leaf unwrapping to pointers at the metadata owner.
- **Completed:** checked every contract projection, formatted, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- The new regression first failed because the malformed string-slice declaration returned no error, proving the registration gap. It passed after the owner fix in 1.06s; dispatch plus contractgen tests passed in 3.41s.
- Generated-contract drift, registry-to-validator reachability, and cross-artifact value-constraint tests passed in 2.67s. No generated artifact changed.
- Dispatch and contractgen passed with `-race` in 19.29s. Focused `go vet` and `staticcheck` passed.
- Runtime `GOWORK=off go vet ./...` passed in 5.65s, `GOWORK=off go build ./...` passed in 1.88s, and uncached `GOWORK=off go test -count=1 ./...` passed in 45.51s.
- Current-workspace CLI `go vet ./...` passed in 3.07s, `go build ./...` passed in 1.92s, and uncached `go test -count=1 ./...` passed in 39.29s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND6_OK`, status `completed`, one step, 9,188 input tokens, 8 output tokens, 9,088 cache-read tokens, and 670ms model duration; the Run command took 3.80s. No credential value was printed or copied.
- `git diff --check` passed.

### Changes and compatibility

- `AllowedValueSet` registration now removes pointer layers only when deciding whether its target is a scalar string. It no longer imports collection-path traversal semantics into leaf cardinality validation.
- Existing pointer and named-string targets remain valid; slices are rejected alongside the arrays and non-string scalar types that were already invalid.
- Breaking changes: malformed private shape metadata that value-restricts an entire string collection is now rejected at registration. First-party metadata, generated Go/TypeScript validators, JSON Schema, OpenRPC, manifest artifacts, protocol values, persisted data, and public bindings are unchanged.

### Resource cleanup

- The bounded live Run used a validated `/tmp/flame-live-round6.*` directory containing its isolated Runtime home and Go build cache. The exit trap moved it to the system Trash.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- The fix affects only malformed metadata and is guarded at the earliest common owner before either projection runs. Existing registrations and byte-for-byte artifact drift gates cover the accepted path.
- Round 7 will audit bootstrap assembly and configuration authority, the next measured complex boundary, for duplicated provider/model or lifecycle ownership before considering any extraction.

## Round 7 — complete

### Audit scope and evidence

- Traced the default provider/model pair from process settings through `Config`, execution composition, utility-role fallback, Session creation, and Run freezing.
- `buildAssembly` validates adapter presence and paths, then purges unbound tool-result blobs before the default model pair is parsed. A malformed pair therefore mutates durable startup state before construction rejects its configuration.
- The same `runtimeDefaultModelSelection` derivation runs again in `buildAssemblyCore` for Session dependencies. Configuration is immutable during one build, so the second parse is a duplicate representation and a second failure point for one fact.
- The model environment is already the composition capsule shared by interactive execution, auxiliary model work, and Session-facing model state. It can carry the one validated default selection without adding a package, interface, or locator.
- Baseline: `GOWORK=off go test -count=1 ./internal/bootstrap -run 'TestAssembly|TestNewRequiresRuntimeDependencies'` passed in 2.14s.

### Root cause

Default model selection was treated as a local constructor argument at two downstream consumers instead of a validated composition input. As a result, its validation happened after startup reconciliation and its value had no single owner during assembly.

### Impact and acceptance criteria

- Parse and validate the default provider/model pair exactly once, immediately after structural Config validation and before any persistence cleanup or other construction work.
- Carry that same typed selection through the model composition into Session dependencies.
- Prove an invalid selection cannot purge an unbound tool result.
- Preserve valid startup ordering, provider hot updates, utility-role fallback, Session defaults, frozen Run identity, errors, public bindings, and generated contracts.
- Pass focused bootstrap tests normally and under the race detector, Runtime and current-workspace CLI full gates, plus one bounded live Run.

### Plan

- **Completed:** added a durable-side-effect regression, then moved the one selection derivation to the assembly entry and retained it in the model environment.
- **Completed:** formatted, remeasured the assembly complexity finding, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- The new regression first failed after observing that rejected config had already deleted the staged tool result. After the sequencing fix, it and the focused assembly/config set passed in 2.59s.
- The full bootstrap package passed in 3.34s and with `-race` in 43.34s. Focused `go vet` and `staticcheck` passed.
- `buildAssemblyCore` fell from cognitive complexity 32 to 31 because it no longer reparses and branches on the default selection. It remains one point above the configured diagnostic threshold; its remaining branches attach distinct optional capabilities, so no mechanical split was made.
- Runtime `GOWORK=off go vet ./...` passed in 0.73s, `GOWORK=off go build ./...` passed in 1.86s, and uncached `GOWORK=off go test -count=1 ./...` passed in 43.76s.
- Current-workspace CLI `go vet ./...` passed in 0.56s, `go build ./...` passed in 1.99s, and uncached `go test -count=1 ./...` passed in 38.91s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND7_OK`, status `completed`, one step, 9,188 input tokens, 8 output tokens, 9,088 cache-read tokens, and 703ms model duration; the Run command took 3.82s. No credential value was printed or copied.
- `git diff --check` passed. Protocol generation was not applicable because no wire type, constraint, schema, or published contract changed.

### Changes and compatibility

- `buildAssembly` now derives the default model selection once immediately after structural Config validation and before startup reconciliation.
- `modelEnvironment` carries that exact typed value alongside the role state and resolvers it configures; Session construction consumes the same value instead of parsing Config again.
- Breaking changes: none. All changed functions and fields are package-private. Valid startup phase order, provider hot updates, role fallback, Session defaults, frozen Run identity, errors, persistence formats, protocol values, and consumer bindings are unchanged.

### Resource cleanup

- The bounded live Run used a validated `/tmp/flame-live-round7.*` directory containing its isolated Runtime home and Go build cache. The exit trap moved it to the system Trash.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- The selection is immutable construction data and is still revalidated by the Session coordinator at its application boundary; the change removes only duplicate composition parsing and moves rejection before effects.
- Round 8 will audit CLI terminal invalidation refresh, the remaining measured consumer complexity, for stale-projection or duplicate retry ownership before deciding whether its branches should move.

## Round 8 — complete

### Audit scope and evidence

- Traced Runtime session/run/plan/goal/interrupt invalidations through changefeed delivery, cold Session reads, operation-slot ownership, queue admission, snapshot installation, and settlement refreshes.
- `refreshInvalidatedSession` clears the coalescing flag before its asynchronous read, as required to detect a newer notification during that read. Ordinary refreshes, however, run under the unrestricted operation policy; the cleared flag means neither admission guard remains active.
- A prompt submitted in that window can start a Run against the stale terminal projection. This is especially material for run, plan, goal, and interrupt changes, and contradicts the existing queue message and `runtimeChangeBlocksRunAdmission` policy intended to place prompts behind Runtime changes.
- Settlement refresh already uses the correct session admission fence. That fence releases before applying its result and deliberately requires the caller to drain only after a successful authoritative install.
- Baseline: `go test -count=1 ./internal/delivery/terminal -run 'TestRuntimeInvalidation|TestInvalidatedSessionRead|TestSessionChangeSettlement'` passed in 4.92s.

### Root cause

The refresh function encoded one semantic distinction twice: `settlementFence` selected both whether an in-flight read could be replaced and whether the read blocked Run admission. Replacement strength differs between ordinary and settlement refreshes; authoritative-read ordering does not.

### Impact and acceptance criteria

- Every in-flight authoritative Session refresh must block new Run admission while preserving ordinary coalescing and settlement replacement.
- A prompt entered during the read remains durably queued and starts only after the successful snapshot application.
- Failed, superseded, deleted-session, live-stream, following, cancellation, and session-change paths retain their existing fail-closed behavior.
- Remove the duplicated operation-policy branch and remeasure the CLI complexity finding without introducing a new state flag or abstraction.
- Pass focused invalidation/queue tests normally and under the race detector, Runtime and current-workspace CLI full gates, plus one bounded live Run.

### Plan

- **Completed:** added a blocked-read integration regression, routed both refresh modes through the admission fence, and explicitly drained an ordinary refresh only after success.
- **Completed:** separated UI-loop reconciliation from asynchronous scheduling, formatted, remeasured complexity, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- The new integration regression first failed while the authoritative read was blocked: instead of showing `queued behind runtime change`, the last frame already showed the submitted Run as `complete`. After the fence and success-drain fix, it passed in 5.28s.
- The final focused invalidation/session-change set passed in 6.75s. The complete terminal package passed in 22.43s and with `-race` in 48.11s. Focused `go vet` and `staticcheck` passed.
- Production-only `gocognit`/`gocyclo` now reports zero CLI terminal findings; `refreshInvalidatedSession` no longer exceeds the threshold. The extracted apply phase owns result reconciliation rather than adding a generic helper or mutable state.
- Runtime `GOWORK=off go vet ./...` passed in 0.75s, `GOWORK=off go build ./...` passed in 3.51s, and uncached `GOWORK=off go test -count=1 ./...` passed in 43.97s.
- Current-workspace CLI `go vet ./...` passed in 0.60s, `go build ./...` passed in 2.31s, and uncached `go test -count=1 ./...` passed in 40.00s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND8_OK`, status `completed`, one step, 9,188 input tokens, 8 output tokens, 9,088 cache-read tokens, and 1,270ms model duration; the Run command took 5.18s. No credential value was printed or copied.
- `git diff --check` passed. Protocol generation was not applicable because no wire type, schema, or public binding changed.

### Changes and compatibility

- Ordinary and settlement Session refreshes now share the session admission-fence policy. `settlementFence` controls only whether a stronger settlement read replaces the current read.
- A successful ordinary refresh explicitly drains the queue after applying the authoritative snapshot. Errors and superseding invalidations leave the invalidated flag asserted, so admission remains fail-closed.
- UI-loop snapshot reconciliation now lives in `applyInvalidatedSessionRefresh`; `refreshInvalidatedSession` owns only read construction, operation policy, and coalescing admission.
- Breaking behavior: a Run submitted during an ordinary Runtime invalidation read is intentionally queued until the read succeeds instead of racing the stale local projection. Queue persistence, Runtime requests, protocol values, errors, and public APIs are unchanged.

### Resource cleanup

- The bounded live Run used a validated `/tmp/flame-live-round8.*` directory containing its isolated Runtime home and Go build cache. The exit trap moved it to the system Trash.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- The new fence cannot deadlock queued work: the operation lease is released before apply, success drains explicitly, a newer invalidation immediately starts its successor, and every failure path leaves a visible blocked state. Integration and race tests cover the blocked-read handoff.
- Round 9 will return to Runtime contract generation and inspect the high-complexity field-constraint compiler for unsupported metadata combinations that currently panic after registration rather than being rejected by the metadata owner.

## Round 9 — complete

### Audit scope and evidence

- Compared `FieldConstraintSpec.validate` with every `constraintCheck` branch in the generated Go validator and the corresponding JSON Schema compiler.
- Registration validates the underlying value kind but discards the field's JSON optionality before checking generator support. It therefore accepts required `*string` prefix/pattern constraints and an `omitempty` non-pointer prefix even though the Go generator explicitly panics for each.
- Registration also unwraps one pointer before validating string-item constraints, accepting pointer-to-string-slice identity/prefix constraints that the Go generator explicitly panics on. Pattern items intentionally support that shape and must remain accepted.
- Schema generation has no equivalent panic, so malformed metadata can succeed in one projection and abort another. No first-party registration uses these unsupported shapes; adding validator machinery would create speculative surface rather than satisfy a product contract.
- Baseline: `GOWORK=off go test -count=1 ./internal/delivery/dispatch ./cmd/contractgen` passed in 3.35s.

### Root cause

The metadata owner reduced a field to its value type before checking it, while Go validator generation depends on both declared pointer shape and JSON optionality. That lost representation allowed the generator's private limitations to become delayed panics instead of registration invariants.

### Impact and acceptance criteria

- Reject all five metadata shapes currently guarded by explicit generator panics, with errors naming owner, field, and constraint.
- Keep supported optional pointer pattern/prefix, optional pointer pattern-items, scalar constraints, and non-pointer string-item constraints unchanged.
- Do not add validator functions, schema keywords, aliases, or compatibility paths for unused shapes.
- Remove the now-unreachable generator panic branches after registration and generation tests prove the support matrix.
- Pass focused metadata/generator/drift tests normally and under the race detector, Runtime and current-workspace CLI full gates, plus one bounded live Run.

### Plan

- **Completed:** added a table-driven registration regression for the five unsupported shapes, preserved the full field descriptor during target validation, then deleted redundant generator panics.
- **Completed:** added positive support-matrix coverage, formatted, remeasured constraint compiler complexity, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- Before the fix, all five unsupported target cases returned no registration error. After the fix, each fails at metadata validation; positive coverage proves optional pointer prefix/pattern and pointer pattern-items remain accepted.
- Dispatch plus contractgen tests passed in 2.84s. Generated-contract drift, registry-to-validator reachability, and cross-artifact value-constraint tests passed in 4.88s; no generated artifact changed.
- Dispatch and contractgen passed with `-race` in 9.77s. Focused `go vet` and `staticcheck` passed.
- Production-only complexity scanning reduced `constraintCheck` from cognitive complexity 49 to 38 by deleting the unreachable guards; no new finding or cyclomatic violation appeared.
- Runtime `GOWORK=off go vet ./...` passed in 0.73s, `GOWORK=off go build ./...` passed in 3.67s, and uncached `GOWORK=off go test -count=1 ./...` passed in 43.11s.
- Current-workspace CLI `go vet ./...` passed in 0.55s, `go build ./...` passed in 3.15s, and uncached `go test -count=1 ./...` passed in 42.95s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND9_OK`, status `completed`, one step, 9,188 input tokens, 8 output tokens, 9,088 cache-read tokens, and 1,450ms model duration; the Run command took 5.18s. No credential value was printed or copied.
- `git diff --check` passed.

### Changes and compatibility

- Field target validation now retains `contractshape.Field`, so it can decide from declared pointer shape and JSON optionality after confirming the underlying text or string-array type.
- The five unsupported shapes now fail in `FieldConstraintSpec.validate`, before any artifact compiler runs. Their five matching panic branches were removed from the Go validator compiler.
- Breaking changes: malformed private metadata using those unsupported shapes is now rejected earlier. All first-party metadata, generated validators/schema/OpenRPC/manifest output, wire values, persistence, and public Runtime/CLI bindings are unchanged.

### Resource cleanup

- The bounded live Run used a validated `/tmp/flame-live-round9.*` directory containing its isolated Runtime home and Go build cache. The exit trap moved it to the system Trash.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- Generator helpers remain intentionally narrow, but their supported target matrix is now registered and regression-tested at the metadata authority. Direct compiler calls in tests still receive trusted validated metadata.
- Round 10 will audit `ObjectConstraintSpec.validate` for contradictory condition sets and duplicate logical rules that exact-struct duplicate checks do not detect.

## Round 10 — complete

### Audit scope and evidence

- Traced `FieldCondition` through capability gating, object-constraint registration, runtime reflection matching, Go validator generation, and conditional JSON Schema.
- `OperatorEquals` validates only that its value is non-empty. It accepts numeric, boolean, collection, and object targets, but both runtime matchers require the reflected field to be a string; generated Schema instead applies a string `const` to the non-string node. Such a condition can never hold in any projection.
- `ObjectConstraintSpec.validate` rejects an exact duplicate condition but accepts two different equals values for the same field, producing `field == A && field == B`. It also accepts `present && equals` for one field even though equals already implies presence in all three projections.
- All current first-party equals conditions target strings and no current rule repeats a condition field, so rejecting these malformed declarations changes no published artifact.
- Baseline: `GOWORK=off go test -count=1 ./internal/delivery ./internal/delivery/dispatch` passed in 11.70s.

### Root cause

Shared condition validation checked path existence and operator arguments but discarded the leaf type. Object-rule validation then compared only whole condition structs, not the one-field predicate they collectively represent.

### Impact and acceptance criteria

- Make shared `OperatorEquals` validation accept only declared scalar strings, including named string types; final pointer fields are unsupported by both runtime matchers and must also fail registration.
- Reject conflicting equals values and redundant present-plus-equals predicates on one field inside an object rule.
- Preserve condition order, valid capability gates, all first-party object rules, error wrapping, and generated artifacts.
- Move condition-set validation into one named phase so `ObjectConstraintSpec.validate` becomes shallower instead of accumulating more branches.
- Pass focused delivery/dispatch/generator/drift tests normally and under the race detector, Runtime and current-workspace CLI full gates, plus one bounded live Run.

### Plan

- **Completed:** added regressions at the shared condition boundary and object-rule boundary, retained the resolved leaf type, and extracted condition-set validation.
- **Completed:** separated field-declaration validation from condition validation, formatted, remeasured complexity, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- Before the fix, equals-on-boolean, equals-on-pointer, conflicting equals, and present-plus-equals regressions all returned no error. After the fix, shared and object-owner focused tests passed in 6.28s.
- The final delivery, dispatch, and contractgen set passed in 13.16s. Generated-contract drift, registry-to-validator reachability, and cross-artifact value-constraint tests passed in 4.04s; no generated artifact changed.
- Delivery, dispatch, and contractgen passed with `-race` in 27.82s. Focused `go vet` and `staticcheck` passed.
- Production-only complexity scanning no longer reports `ObjectConstraintSpec.validate`; it fell from 37 to below the threshold. Runtime delivery now has only the strict-null traversal finding, with no cyclomatic violation.
- Runtime `GOWORK=off go vet ./...` passed in 1.28s, `GOWORK=off go build ./...` passed in 4.61s, and uncached `GOWORK=off go test -count=1 ./...` passed in 47.60s.
- Current-workspace CLI `go vet ./...` passed in 0.80s, `go build ./...` passed in 5.23s, and uncached `go test -count=1 ./...` passed in 42.71s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND10_OK`, status `completed`, one step, 9,188 input tokens, 8 output tokens, 9,088 cache-read tokens, and 1,485ms model duration; the Run command took 5.83s. No credential value was printed or copied.
- `git diff --check` passed.

### Changes and compatibility

- `ValidateFieldCondition` now resolves the leaf and admits equals only when that declared field is a scalar string. Named strings remain supported; pointers and non-string kinds fail at the shared capability/object boundary.
- Object rules now validate each condition set for exact duplicates, conflicting equals, and redundant present-plus-equals predicates, then independently validate required/alternative/forbidden/value-restricted fields.
- Breaking changes: malformed private condition metadata is now rejected rather than compiling an unreachable rule. All first-party capability and object rules, generated artifacts, request behavior, wire values, persistence, and public bindings are unchanged.

### Resource cleanup

- The bounded live Run used a validated `/tmp/flame-live-round10.*` directory containing its isolated Runtime home and Go build cache. The exit trap moved it to the system Trash.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- Both runtime reflection matchers already required a final scalar string, so registration now states their existing contract rather than changing evaluation. Cross-artifact drift tests cover every accepted first-party rule.
- Round 11 will revisit strict explicit-null traversal, now the sole measured Runtime delivery complexity finding, and separate type normalization from container recursion only if the existing acceptance rules prove a coherent split.

## Round 11 — complete

### Audit scope and evidence

- Re-audited strict parameter decoding across pointer omission, scalar leaves, structs, sequences, typed maps, byte slices, custom JSON unmarshallers, `json.RawMessage`, and interface-backed opaque JSON.
- No new acceptance defect was found. The ordering is deliberate: opaque JSON owns null; every typed value rejects null before custom decoding; custom decoders own their non-null interior; byte slices remain scalar JSON payloads; typed containers recurse.
- The sole remaining production complexity finding in dispatch comes from placing struct, sequence, and map decoding/traversal inside the same recursive dispatcher. These are independent container policies with distinct ordering and path construction, and none shares mutable state beyond the recursive call.
- Existing tests cover pointer/struct, typed-map ordering, and opaque values but not an explicit null inside a typed slice. That missing case should anchor the sequence policy before extraction.
- Baseline: `GOWORK=off go test -count=1 ./internal/delivery/dispatch -run 'TestDecodeParams|TestDecodeProviderUpdate'` passed in 1.11s.

### Root cause

The null boundary began as one recursive routine and accumulated three complete container walkers. The top-level dispatcher now owns type semantics and container traversal simultaneously, inflating nesting even though each traversal is independently understandable.

### Impact and acceptance criteria

- Preserve exact acceptance and error text for pointers, structs, slices, arrays, maps, bytes, custom decoders, raw messages, and interfaces.
- Keep struct declaration order, sequence index order, and lexicographically sorted map-key order.
- Add typed-slice element-null coverage alongside existing typed-map and opaque-map regressions.
- Give each container one private traversal function without a generic visitor, state object, interface, or new package.
- Eliminate the final dispatch production complexity finding and pass focused dispatch tests normally and under the race detector, Runtime and current-workspace CLI full gates, plus one bounded live Run.

### Plan

- **Completed:** added cross-container null regression coverage and extracted the three container walkers while retaining `rejectExplicitNulls` as the only recursive entry point.
- **Completed:** formatted, remeasured complexity, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- Pointer-field, typed-slice element, typed-map value, and opaque-map cases passed after extraction in 1.22s with exact path diagnostics.
- The complete dispatch package passed in 0.59s and with `-race` in 3.14s. Focused `go vet` and `staticcheck` passed.
- Production-only `gocognit`/`gocyclo` now reports zero dispatch findings; `rejectExplicitNulls` no longer exceeds the threshold, and none of the three container walkers introduces a finding.
- Runtime `GOWORK=off go vet ./...` passed in 0.77s, `GOWORK=off go build ./...` passed in 3.81s, and uncached `GOWORK=off go test -count=1 ./...` passed in 44.73s.
- Current-workspace CLI `go vet ./...` passed in 0.52s, `go build ./...` passed in 2.17s, and uncached `go test -count=1 ./...` passed in 39.72s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND11_OK`, status `completed`, one step, 9,188 input tokens, 8 output tokens, 9,088 cache-read tokens, and 375ms model duration; the Run command took 3.25s. No credential value was printed or copied.
- `git diff --check` passed. Protocol generation was not applicable because request acceptance, wire shapes, and published artifacts did not change.

### Changes and compatibility

- `rejectExplicitNulls` remains the only recursive semantic entry and owns pointer normalization, opaque-value exemption, explicit-null rejection, custom-decoder ownership, and container dispatch.
- Struct, sequence, and map traversal now live in concrete private functions that preserve declaration, index, and sorted-key order respectively.
- Breaking changes: none. Accepted/rejected JSON, exact diagnostic paths and text, protocol errors, generated artifacts, and public bindings are unchanged.

### Resource cleanup

- The bounded live Run used a validated `/tmp/flame-live-round11.*` directory containing its isolated Runtime home and Go build cache. The exit trap moved it to the system Trash.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- The extraction changes only function boundaries; all recursion still re-enters the same semantic gate, so nested opaque and custom-decoder behavior cannot bypass a new path. Focused and full race coverage passed.
- Round 12 will audit the remaining contractgen field compiler against collection/numeric pointer shapes and requiredness, looking for additional registration-to-generator mismatches before any structural split.

## Round 12 — complete

### Audit scope and evidence

- Compared every collection, property-map, and numeric `constraintCheck` branch with its concrete helper signature in `protocol/wire_constraints.go` and the registration-time target checks.
- Registration currently unwraps one pointer and accepts several shapes the Go projection cannot compile: pointer arrays with `nonEmptyItems` or `minItems`, pointer maps with any property constraint, and pointer numbers with `minimum` (whose selected `optionalMinimumNumber` helper does not exist).
- A required non-pointer array with `minItems` selects another nonexistent helper, `requiredMinItems`. A unique-items constraint also accepts slices whose element type cannot instantiate the helper's `comparable` type parameter.
- Required pointer fields form a separate cross-projection mismatch: generated helpers treat pointer absence as optional (and `nonEmpty` cannot compile at all), while the schema marks the property required. No first-party constraint uses a required pointer field.
- Baseline: `GOWORK=off go test -count=1 ./internal/delivery/dispatch ./cmd/contractgen` passed in 2.99s.

### Root cause

Target validation proves only the dereferenced JSON kind. Go generation additionally depends on the declared pointer shape, JSON optionality, collection element comparability, and the exact helper surface, so invalid metadata survives its owner and fails later or projects different requiredness.

### Impact and acceptance criteria

- Reject required pointer fields for field constraints until the Go projection has a required-pointer policy.
- Reject pointer targets for the collection, property-map, and minimum-number constraints whose generated helper call is invalid.
- Admit `minItems` only on optional non-pointer slices, matching the one implemented helper, and reject unique-items element types that cannot instantiate `comparable`.
- Preserve all current first-party metadata and the supported optional pointer paths for numeric maxima, positivity, non-negativity, max-items, unique-items, max-item-length, and pattern-items.
- Keep the support matrix in the metadata owner, generate no new unused helpers, and pass focused, drift, race, static, full Runtime/CLI, and bounded live verification.

### Plan

- **Completed:** added negative and positive support-matrix regressions, then centralized projection validation after JSON-kind validation.
- **Completed:** formatted, regenerated/checked contract artifacts, remeasured complexity, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- Before the fix, all nine new invalid-shape cases returned no registration error: a required pointer, pointer non-empty/min-items, required min-items, three pointer-map constraints, pointer minimum, and non-comparable unique items. After the fix, the full negative matrix fails at registration and the supported optional pointer/comparable-item matrix still passes.
- Dispatch and contractgen tests passed after the final extraction. Generated-contract drift, registry-to-validator reachability, and cross-artifact value-constraint tests passed in 2.78s; `go generate ./...` changed no generated artifact.
- Dispatch and contractgen passed with `-race` in 12.18s. Focused `go vet` and `staticcheck` passed.
- Production-only `gocognit` still reports only the pre-existing `constraintCheck` score of 38. `validateConstraintProjection` and its concrete pointer/unique/text helpers introduce no cognitive or cyclomatic finding above the selected thresholds.
- Runtime `GOWORK=off go vet ./...` plus `GOWORK=off go build ./...` passed in 17.54s, and an isolated uncached `GOWORK=off go test -count=1 ./...` passed in 25.97s.
- The first Runtime suite run, executed concurrently with four other heavy gates, exposed one cold-waiting delegate cancellation timing failure. The exact test then passed ten consecutive isolated runs in 2.78s, and the complete isolated Runtime suite passed, so no stable regression remained.
- Current-workspace CLI `go vet ./...` plus `go build ./...` passed in 2.26s, and uncached `go test -count=1 ./...` passed in 40.63s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND12_OK`, status `completed`, one step, 9,183 input tokens, 8 output tokens, 9,088 cache-read tokens, and 1,013ms total model duration. No credential value was printed or copied.
- `git diff --check` passed.

### Changes and compatibility

- Projection validation now runs after target-kind validation for every field constraint. It owns declared pointer shape, JSON optionality, helper availability, and unique-item comparability instead of leaving those facts implicit in the generator.
- Required pointer constraints and the six pointer shapes without valid Go helpers are rejected at registration. `minItems` is limited to the optional value-slice shape its helper implements, and `uniqueItems` requires an element type that satisfies the generated helper's `comparable` bound.
- Breaking changes: malformed private constraint metadata now fails registration rather than generating uncompilable code or disagreeing with Schema requiredness. All first-party registrations, generated artifacts, wire values, persistence, request behavior, and public Runtime/CLI bindings are unchanged.

### Resource cleanup

- The bounded live Run used a validated `/tmp/flame-live-round12.*` directory containing its isolated Runtime home, Go build cache, and CLI binary. The exit trap moved it to the system Trash.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- The declared-shape matrix now covers pointer presence and the obvious generic bound, but exact Go assignability can still diverge for pointers to named string/slice types and maps keyed by named strings. Current first-party fields use builtin strings and literal slices/maps.
- Round 13 will test those named-type projections directly and then inspect dotted constraints through optional parent pointers, where a generated selector may dereference an absent parent before a leaf helper can apply its omission policy.

## Round 13 — complete

### Audit scope and evidence

- Compiled isolated Go probes against the exact helper signatures. `*NamedString` is not assignable to `*string`, `*NamedSlice` cannot infer the `T` in `*[]T`, and `map[NamedString]T` cannot infer the `T` in `map[string]T`; all three shapes currently pass metadata registration.
- The only first-party dotted value constraint is `ListItemsRequest.scope.type`, whose parent is a required value struct and whose generated selector is safe.
- `contractshape.GoPath` dereferences pointer and slice parents while building a plain selector. Metadata can therefore accept `pointerParent.value`, which compiles but panics when the parent is nil, and `parentItems.value`, which generates an invalid selector on a slice.
- Baseline: `GOWORK=off go test -count=1 ./internal/contractshape ./internal/delivery/dispatch ./cmd/contractgen` passed in 2.70s.

### Root cause

Registration validates the leaf's JSON kind but does not validate that the complete declared path can be expressed by the generator's direct Go selector or that the declared named container is assignable to the selected helper parameter.

### Impact and acceptance criteria

- Reject optional named string pointers for the four helpers that accept only `*string`, while preserving generic optional non-empty text and numeric pointer helpers.
- Reject pointers to named slices for the four collection helpers that accept only `*[]T`, while preserving pointers to literal slices, including named string element types.
- Require builtin string map keys for the three property-map helpers; named map types with builtin string keys remain valid.
- Reject dotted field constraints whose intermediate path crosses a pointer or slice; preserve dotted paths through value structs.
- Keep all rules at metadata registration, change no first-party artifact, and pass focused, drift, race, static, full Runtime/CLI, and bounded live verification.

### Plan

- **Completed:** added assignability/path regressions, exposed path fields from the wire-shape owner, and validated projection before generator entry.
- **Completed:** formatted, regenerated/checked artifacts, remeasured complexity, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- Before the fix, all three representative helper-assignability cases and both unsafe-parent paths returned no registration error. After the fix, eleven exact named-type helper combinations and both unsafe parent kinds fail at registration.
- Positive coverage preserves generic optional named-string non-empty validation, pointers to literal slices with named string elements, named map types with builtin string keys, and dotted constraints through value structs.
- Contractshape, dispatch, and contractgen tests passed after the final change. Generated-contract drift, registry-to-validator reachability, and cross-artifact value-constraint tests passed in 2.52s; `go generate ./...` changed no generated artifact.
- Contractshape, dispatch, and contractgen passed with `-race` in 18.43s. Focused `go vet` and `staticcheck` passed in 1.84s.
- Production-only `gocognit` still reports only the pre-existing `constraintCheck` score of 38. None of the new path or helper-assignment functions appears above the cognitive or cyclomatic thresholds.
- Runtime `GOWORK=off go vet ./...` plus `GOWORK=off go build ./...` passed in 8.74s, and uncached `GOWORK=off go test -count=1 ./...` passed in 47.13s.
- Current-workspace CLI `go vet ./...` plus `go build ./...` passed in 8.96s, and uncached `go test -count=1 ./...` passed in 43.13s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND13_OK`, status `completed`, one step, 9,183 input tokens, 8 output tokens, 9,088 cache-read tokens, and 867ms total model duration. No credential value was printed or copied.
- `git diff --check` passed.

### Changes and compatibility

- `contractshape.PathFields` now exposes the already-owned resolved field chain; `GoPath` builds its selector from that one representation rather than maintaining a second traversal.
- Constraint registration rejects pointer/slice intermediate parents before a direct selector reaches code generation. Value-struct parents, including the current `scope.type` contract, remain supported.
- Helper-assignment validation states the exact accepted declared forms for four pointer-string helpers, four pointer-slice helpers, and three property-map helpers. It rejects only named wrappers that Go cannot pass to the selected helper; naturally assignable named values remain accepted.
- Breaking changes: malformed private metadata now fails registration rather than generating uncompilable code or a nil-dereferencing validator. First-party metadata, generated artifacts, wire values, persistence, request behavior, and public Runtime/CLI bindings are unchanged.

### Resource cleanup

- Two isolated `/tmp/flame-typecheck-round13.*` probes established Go assignability, and the bounded live Run used a validated `/tmp/flame-live-round13.*` directory for its Runtime home, build cache, and CLI binary. Each directory was moved to the system Trash after use.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- `uniqueItems` now rejects element types that are statically non-comparable, but Go permits interface and pointer elements that either panic for dynamic non-comparable values or compare identity while JSON Schema compares values. No first-party unique set uses either shape.
- Round 14 will close that semantic equality gap, then audit optional non-pointer scalar constraints whose zero value conflates omission with presence and may still disagree with Schema/TypeScript omission semantics.

## Round 14 — complete

### Audit scope and evidence

- JSON Schema `uniqueItems` compares JSON values. The generated TypeScript helper documents that rule but keys elements with raw `JSON.stringify`, so equal objects with different member order are treated as distinct; a direct Node probe returned zero violations for that case.
- Go uses `map[T]bool`: an interface element containing a decoded object panics as an unhashable dynamic value, while two different pointers to equal JSON values compare as distinct identities.
- `ProblemData.requiredCapabilities` is a current first-party `uniqueItems` object array. Its Go struct happens to be comparable and generated in stable field order, but the TypeScript validator consumes arbitrary member order from JSON and therefore already exposes the projection drift.
- Canonical JSON encoding naturally handles strings/enums and object values with one equality rule. The current comparable-only registration check is an implementation leak and should disappear with the map-key implementation.
- Baseline: `GOWORK=off go test -count=1 ./protocol ./internal/delivery/dispatch ./cmd/contractgen` passed in 4.85s. The direct TypeScript probe demonstrated the object-order miss before changes.

### Root cause

Both validators encode set membership with host-language shortcuts: Go hash equality and JavaScript insertion-order serialization. Neither shortcut owns JSON value equality, which is the published schema contract.

### Impact and acceptance criteria

- Compare Go items by deterministic JSON encoding so maps, interface-backed values, and pointers use value rather than identity semantics without panics.
- Canonicalize TypeScript objects recursively by sorted keys while preserving array order and primitive JSON semantics.
- Remove the metadata `comparable` restriction and accept non-comparable JSON item types again.
- Prove duplicate dynamic objects, equal pointed-to values, object member reordering, and distinct nested values.
- Preserve all first-party generated artifacts and diagnostics, then pass focused, drift, race, static, full Runtime/CLI, and bounded live verification.

### Plan

- **Completed:** added failing Go value-equality regressions, implemented canonical equality in both Runtime contract validators, and removed the obsolete registration restriction.
- **Completed:** verified TypeScript behavior directly, regenerated/checked artifacts, remeasured complexity, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- Before the fix, Go panicked on an interface-backed object and missed two distinct pointers to equal values. The TypeScript probe reported zero violations for equal objects with reversed member order. After the fix, all Go value-equality regressions pass; TypeScript reports one duplicate and zero violations for distinct nested values.
- Protocol, dispatch, and contractgen focused tests passed. Generated-contract drift, registry-to-validator reachability, and cross-artifact value-constraint tests passed in 3.30s; `go generate ./...` changed no generated artifact.
- Protocol, dispatch, and contractgen passed with `-race` in 17.16s. Focused `go vet` and `staticcheck` passed in 1.31s.
- The Runtime contract TypeScript file passed standalone typechecking and linting. The existing Desktop wire-validator consumer suite passed all 29 tests in 0.96s without changing Desktop source.
- Production-only complexity scanning remains unchanged: `constraintCheck` is the sole cognitive finding at 38, and neither canonicalization implementation adds a threshold finding.
- Runtime `GOWORK=off go vet ./...` plus `GOWORK=off go build ./...` passed in 7.26s, and uncached `GOWORK=off go test -count=1 ./...` passed in 47.87s.
- Current-workspace CLI `go vet ./...` plus `go build ./...` passed in 7.83s, and uncached `go test -count=1 ./...` passed in 41.33s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND14_OK`, status `completed`, one step, 9,183 input tokens, 8 output tokens, 9,088 cache-read tokens, and 621ms total model duration. No credential value was printed or copied.
- `git diff --check` passed.

### Changes and compatibility

- Go `uniqueItems` now serializes each element with deterministic `encoding/json` output and hashes that canonical representation. It accepts every JSON-representable element, compares pointers by their values, and returns a normal field violation instead of panicking if a direct Go caller supplies a non-JSON value.
- TypeScript recursively sorts object keys before serialization while preserving array order and primitive JSON semantics. Equal decoded objects now have one key independent of source member order.
- The metadata owner no longer exposes Go's former `comparable` implementation constraint. Non-comparable item types are valid again, while named-slice pointer assignability remains a separate generator constraint.
- Breaking behavior: duplicate object values with different property order are now correctly rejected by the TypeScript validator. Schema, first-party Go behavior, diagnostics, generated artifacts, wire shapes, persistence, and public Runtime/CLI bindings are unchanged.

### Resource cleanup

- The bounded live Run used a validated `/tmp/flame-live-round14.*` directory containing its isolated Runtime home, Go build cache, and CLI binary. The exit trap moved it to the system Trash.
- No matching temporary directory remained. No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- Canonical equality now matches representable JSON values in both validators. Cyclic or otherwise non-JSON direct host values remain outside the wire contract; Go reports them as a field violation and actual TypeScript inputs arrive from JSON decoding.
- Round 15 will audit every optional non-pointer scalar constraint against encoder omission, Go zero values, Schema keywords, and TypeScript property absence, starting with `nonEmpty`, `nonNegative`, `minimum`, `maximum`, and string identity/length rules.

## Round 15 — complete

### Audit scope and evidence

- Enumerated every first-party value constraint whose declared field is an optional non-pointer scalar, then removed the temporary audit test. Max-length, identity, non-negative, maximum, positive, and pattern helpers all accept an omitted zero value or have an explicit optional-scalar path.
- `minimum` is the remaining unsupported shape: an omitted `omitempty` numeric decodes to zero, but the generator selects `minimumNumber` and rejects it against every positive minimum. No first-party metadata uses this combination.
- Optional non-empty strings split into conditionally required union fields and `Goal.provider/model`. Union generation deliberately delegates presence to its branch rule. `Goal` is different: its Go validator and protocol tests require an exact provider/model, while both fields carry `omitempty`, so Schema and generated TypeScript permit omission.
- Removing `omitempty` from `Goal.provider/model` is the correct semantic repair, but it changes the Runtime-generated TypeScript `Goal` type and requires known Desktop fixtures/adapters to migrate. The prompt's highest-priority scope permits only Runtime and CLI changes, so that batch cannot be completed without violating scope; weakening Go/domain requiredness would preserve the wrong design.
- Baseline inventory and the prior full gates are green; focused dispatch/contractgen baseline remains the Round 14 result.

### Root cause

The field compiler infers optional-scalar policy independently per helper. Most helpers state omission explicitly, while `minimum` falls through to its required-value helper. Separately, `Goal`'s Go tag contradicts the already-enforced aggregate projection invariant.

### Impact and acceptance criteria

- Reject `minimum` on an optional non-pointer value at metadata registration; preserve required scalar minimum and the already-rejected pointer minimum.
- Add negative and positive support-matrix coverage, generate no unused optional-minimum helper, and change no first-party artifact.
- Record the Goal tag/consumer conflict as out-of-scope remaining risk rather than weakening or partially migrating the contract.
- Pass focused, drift, race, static, full Runtime/CLI, and bounded live verification.

### Plan

- **Completed:** added the minimum support-matrix regression and stated the missing optional-value projection in the metadata owner.
- **Completed:** formatted, regenerated/checked artifacts, remeasured complexity, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- Before the fix, optional non-pointer `minimum` returned no registration error. After the fix, it fails with an owner/field/constraint diagnostic, while required scalar minimum remains accepted.
- Dispatch and contractgen focused tests passed in 4.00s. Generated-contract drift, registry-to-validator reachability, and cross-artifact value-constraint tests passed in 4.00s; `go generate ./...` changed no generated artifact.
- Dispatch and contractgen passed with `-race`; focused `go vet` and `staticcheck` also passed in the combined 16.54s gate.
- Production-only complexity scanning remains unchanged: `constraintCheck` is the sole cognitive finding at 38, and the new registration branch introduces no threshold finding.
- Runtime `GOWORK=off go vet ./...` plus `GOWORK=off go build ./...` passed in 5.39s, and uncached `GOWORK=off go test -count=1 ./...` passed in 49.58s.
- Current-workspace CLI `go vet ./...` plus `go build ./...` passed in 5.58s, and uncached `go test -count=1 ./...` passed in 42.18s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND15_OK`, status `completed`, one step, 9,183 input tokens, 8 output tokens, 9,088 cache-read tokens, and 884ms total model duration. No credential value was printed or copied.
- `git diff --check` passed.

### Changes and compatibility

- Projection validation now rejects `minimum` when `omitempty` collapses absence into a non-pointer zero value and no optional helper exists. Required scalar minimum retains the existing Go/Schema/TypeScript behavior.
- No generated helper, schema keyword, wire type, public API, persistence value, or first-party registration changed. The only breaking behavior is earlier rejection of malformed private metadata.
- The `Goal.provider/model` tag contradiction was deliberately not partially repaired: Runtime and CLI cannot change the generated type without migrating known Desktop consumers, which the task's priority-0 scope forbids.

### Resource cleanup

- The temporary optional-scalar inventory test was deleted immediately after producing the audit evidence.
- The bounded live Run used a validated `/tmp/flame-live-round15.*` directory containing its isolated Runtime home, Go build cache, and CLI binary. The exit trap moved it to the system Trash; no matching temporary path remained.
- No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- `Goal.provider/model` remains an evidenced cross-artifact inconsistency blocked by the current Runtime/CLI-only scope. A future scope that includes Desktop should remove both `omitempty` tags, regenerate the contract, and migrate every Goal fixture/adapter in one batch.
- Round 16 will address the remaining measured contractgen complexity by separating textual, collection/property, and numeric constraint rendering only where the closed support matrix gives each family a coherent responsibility; output must remain byte-for-byte stable.

## Round 16 — complete

### Audit scope and evidence

- `constraintCheck` is the only production function above the cognitive threshold in the changed contract path, scoring 38. Its switch mixes five text constraints, four numeric constraints, eight array constraints, and three property-map constraints.
- The function has no shared mutation across families. Each family reads the same resolved selector/leaf and emits one independent helper call; family-specific decisions concern only optionality, pointer shape, limit/value arguments, or union-required text.
- Rounds 9–15 moved unsupported declared shapes into registration, so the compiler now consumes a closed, validated matrix rather than needing defensive family-crossing logic.
- Baseline: focused contractgen and generated-drift tests passed in 3.83s; production `gocognit` reported only `constraintCheck` at 38.

### Root cause

One renderer accumulated every constraint vocabulary in a single switch even though text, numeric, array, and property-map helpers evolve independently. The resulting score reflects mixed responsibilities, not an intrinsically complex algorithm.

### Impact and acceptance criteria

- Keep selector resolution and unknown-kind failure at the single `constraintCheck` entry.
- Move rendering into four concrete family functions without a registry, reflection dispatch table, visitor, or configuration object.
- Preserve every emitted validator call byte-for-byte, including union-owned non-empty omission and optional pointer helper selection.
- Reduce every new function below the cognitive threshold and change no generated artifact.
- Pass focused, drift, race, static, full Runtime/CLI, and bounded live verification.

### Plan

- **Completed:** extracted the four rendering families and three text-policy helpers, formatted them, and proved byte-for-byte generator stability.
- **Completed:** remeasured complexity, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- After the initial family split, contractgen tests and generated-contract drift passed. The remaining text renderer scored 16, so its three real optionality decisions were extracted into non-empty, prefix, and pattern helpers; contractgen and drift then passed again.
- Final focused generation and contract checks passed in 2.30s after `go generate ./...`; no generated artifact changed.
- Production-only complexity scanning no longer reports `constraintCheck` or any new Round 16 function above either threshold. Existing cognitive findings begin at `newValidators` (29), and existing cyclomatic findings begin at `newValidators` (27).
- Contractgen passed with `-race`; focused `go vet` and `staticcheck` also passed in the combined 9.65s gate.
- Runtime `GOWORK=off go vet ./...` plus `GOWORK=off go build ./...` passed in 3.70s, and uncached `GOWORK=off go test -count=1 ./...` passed in 22.12s.
- Current-workspace CLI `go vet ./...` plus `go build ./...` passed in 3.81s, and uncached `go test -count=1 ./...` passed in 43.50s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND16_OK`, status `completed`, one step, 9,183 input tokens, 8 output tokens, 9,088 cache-read tokens, and 730ms total model duration. No credential value was printed or copied.
- The first live verifier queried obsolete JSON field names and therefore rejected a successful Run; a shape-only diagnostic identified the verifier defect, and the corrected verifier passed without exposing credentials.
- `git diff --check` passed.

### Changes and compatibility

- Constraint rendering now has one entry point plus concrete text, numeric, item, and property families. Non-empty, prefix, and pattern helpers each own their distinct optionality rule instead of leaving those rules entangled in one switch.
- Numeric minimum and item minimum rendering now state the only shapes admitted by registration directly. This removes unreachable pointer/required helper selection without weakening the metadata owner.
- Generated Go validation, Schema, TypeScript, diagnostics, public APIs, wire shapes, persistence, and first-party behavior are byte-for-byte unchanged. The refactor changes only private generator structure.

### Resource cleanup

- All bounded live attempts used validated `/tmp/flame-live-round16.*` directories containing isolated Runtime homes and temporary CLI binaries. Exit traps moved them to the system Trash.
- No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- `Goal.provider/model` remains an evidenced cross-artifact inconsistency blocked by the current Runtime/CLI-only scope. A future scope that includes Desktop should remove both `omitempty` tags, regenerate the contract, and migrate every Goal fixture/adapter in one batch.
- Round 17 will audit `newValidators`, the highest remaining contractgen complexity finding, and split it only if its import collection, method compilation, and source assembly have independently testable ownership; otherwise the round will move to the next evidence-backed correctness candidate.

## Round 17 — complete

### Audit scope and evidence

- `newValidators` is now the highest measured contractgen cognitive finding at 29. It first builds three rule indexes, then discovers validator targets across four ordered metadata sources plus reachable protocol definitions, and finally renders methods.
- Those phases have distinct contracts: rule indexing merges declarations by owner, target discovery deduplicates while preserving source order, and method rendering omits shapes with no checks. No phase needs to mutate another phase's state.
- Generated method order is observable source output even though runtime behavior is not order-dependent, so extraction must retain request-root priority, declaration order, sorted reachable definitions, and first-occurrence deduplication exactly.
- Baseline focused contractgen and architecture tests passed in 4.46s. A production-only `gocognit`/`gocyclo` scan reports `newValidators` at cognitive complexity 29 and no other finding caused by Round 16.

### Root cause

The file generator remained both a metadata compiler and a source writer after the constraint renderers were separated. Its complexity comes from three sequential responsibilities rather than one irreducible decision tree.

### Impact and acceptance criteria

- Give rule indexing, ordered target discovery, and per-method source emission one concrete private owner each.
- Keep the top-level generator a readable composition path; do not add a registry, generic traversal framework, reflection cache, configuration bag, or package.
- Preserve generated output byte-for-byte, including ordering and omission of empty validators.
- Reduce every new function below the selected cognitive threshold.
- Pass focused, drift, race, static, full Runtime/CLI, and bounded live verification.

### Plan

- **Completed:** extracted the three proven phases, formatted them, compared generated output, and remeasured complexity.
- **Completed:** ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- The first post-change focused contractgen and architecture run passed in 3.69s. `go generate ./...` then produced no diff in the Go validator, Schema, TypeScript types, or TypeScript validator.
- The final focused generated-drift, reachability, union, optionality, and cross-artifact constraint tests passed in 2.46s.
- Production-only complexity scanning no longer reports `newValidators`; none of `newValidatorRuleIndex`, `validatorTargets`, `appendValidatorTarget`, `reachableProtocolStructs`, or `writeValidator` exceeds the selected cognitive or cyclomatic threshold. The remaining findings are pre-existing functions in other generator phases plus `unionChecks`.
- Contractgen passed with `-race`; focused `go vet` and `staticcheck` also passed in the combined 10.95s gate.
- Runtime `GOWORK=off go vet ./...` plus `GOWORK=off go build ./...` passed in 5.29s, and uncached `GOWORK=off go test -count=1 ./...` passed in 58.95s.
- Current-workspace CLI `go vet ./...` plus `go build ./...` passed in 5.38s, and uncached `go test -count=1 ./...` passed in 45.03s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND17_OK`, status `completed`, one step, 9,183 input tokens, 8 output tokens, 9,088 cache-read tokens, and 766ms total model duration. No credential value was printed or copied.
- `git diff --check` passed.

### Changes and compatibility

- A private `validatorRuleIndex` now owns declaration indexing and the lookup that compiles one target's checks. It is behavior-bearing compiler state, not a cross-layer configuration bag.
- Ordered target discovery now owns first-occurrence deduplication and reachable protocol filtering; method emission owns the no-check omission rule and source formatting.
- Request-root priority, registration order, sorted reachable definitions, generated bytes, public APIs, wire shapes, diagnostics, persistence, and Runtime/CLI behavior are unchanged.

### Resource cleanup

- The temporary strict complexity configuration was deleted after the final scan.
- The bounded live Run used a validated `/tmp/flame-live-round17.*` directory containing its isolated Runtime home and current-source CLI binary. The exit trap moved it to the system Trash; no matching temporary path remained.
- No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- `Goal.provider/model` remains an evidenced cross-artifact inconsistency blocked by the current Runtime/CLI-only scope. A future scope that includes Desktop should remove both `omitempty` tags, regenerate the contract, and migrate every Goal fixture/adapter in one batch.
- Round 18 will audit `unionChecks`, now the remaining validator-generator cognitive finding, against union registration and the Schema/TypeScript projections. Extraction will proceed only around proven discriminator, required-field, and forbidden-field policies with byte-stable output.

## Round 18 — complete

### Audit scope and evidence

- `unionChecks` is the remaining Go-validator generator cognitive finding at 22. It combines pattern-discriminator narrowing, literal-branch iteration, required/forbidden field checks, allowed-value checks, and a second copy of required/forbidden rendering for the pattern branch.
- Literal and pattern branches share one exact field policy: require declared fields and forbid every union-owned path not claimed by that branch. Literal branches alone add allowed-value narrowing; a pattern branch alone adds the open-tag matcher.
- `UnionSpec.Forbidden` initially appeared missing from Go generation. The audit proved it deliberately names removed top-level JSON members that do not exist on the Go DTO; JSON parameter decoding already uses `DisallowUnknownFields`, while Schema/TypeScript explicitly project `absent()`. A typed Go value cannot carry such a member, so adding a fabricated typed check would create dead code rather than close a gap.
- The Round 17 generated-drift, reachability, union, and cross-artifact gates are the green baseline; production complexity scanning reports `unionChecks` at 22.

### Root cause

The union renderer duplicates branch field compilation around two tag-selection strategies. The shared policy is obscured by loop nesting even though the registered union metadata already provides a closed, validated shape.

### Impact and acceptance criteria

- Separate pattern tag validation, literal-variant rendering, and common branch field rendering without changing their output order.
- Keep literal allowed-value checks attached to literal variants and keep global removed-field rejection at the raw JSON and generated client boundaries where the field is representable.
- Add no registry, visitor, generic compiler framework, or unrepresentable Go check.
- Preserve generated output byte-for-byte and reduce every new function below the selected complexity threshold.
- Pass focused, drift, race, static, full Runtime/CLI, and bounded live verification.

### Plan

- **Completed:** extracted the proven union rendering policies, formatted them, compared generated output, and remeasured complexity.
- **Completed:** ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- The first post-change focused contractgen and architecture run passed in 5.23s. `go generate ./...` then produced no diff in the Go validator or generated TypeScript artifacts.
- The final focused generated-drift, reachability, union, optionality, and cross-artifact constraint tests passed in 5.11s.
- Production-only complexity scanning no longer reports `unionChecks`; none of the pattern-tag, literal-branch, pattern-branch, or common branch helpers exceeds the selected cognitive or cyclomatic threshold. Five pre-existing findings remain elsewhere in contractgen.
- Contractgen passed with `-race`; focused `go vet` and `staticcheck` also passed in the combined 17.24s gate.
- Runtime `GOWORK=off go vet ./...` plus `GOWORK=off go build ./...` passed in 8.44s, and uncached `GOWORK=off go test -count=1 ./...` passed in 43.42s.
- Current-workspace CLI `go vet ./...` plus `go build ./...` passed in 8.53s, and uncached `go test -count=1 ./...` passed in 45.62s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND18_OK`, status `completed`, one step, 9,183 input tokens, 8 output tokens, 9,088 cache-read tokens, and 761ms total model duration. No credential value was printed or copied.
- `git diff --check` passed.

### Changes and compatibility

- Pattern discriminator narrowing, literal tag selection, and common required/forbidden field rendering now have explicit private functions. Literal allowed-value narrowing remains adjacent to its branch after common field checks, preserving diagnostic order.
- Global removed-field rejection remains at the raw JSON and generated client boundaries. The Go binding cannot construct a removed field, and JSON-RPC parameter decoding rejects it before typed validation.
- Generated bytes, validator ordering, diagnostics, public APIs, wire shapes, persistence, and Runtime/CLI behavior are unchanged.

### Resource cleanup

- The temporary strict complexity configuration was deleted after the final scan.
- The bounded live Run used a validated `/tmp/flame-live-round18.*` directory containing its isolated Runtime home and current-source CLI binary. The exit trap moved it to the system Trash; no matching temporary path remained.
- No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- `Goal.provider/model` remains an evidenced cross-artifact inconsistency blocked by the current Runtime/CLI-only scope. A future scope that includes Desktop should remove both `omitempty` tags, regenerate the contract, and migrate every Goal fixture/adapter in one batch.
- Round 19 will audit `restrictAllowedValues`, the next Schema projection complexity finding, against dotted-path construction, optionality, and TypeScript/Go allowed-value checks before deciding whether a semantic fix or a responsibility split is justified.

## Round 19 — complete

### Audit scope and evidence

- `restrictAllowedValues` is the next Schema generator cognitive finding at 20. It resolves paths, revalidates enum subsets, descends or creates schema nodes, merges with an unconstrained existing node, and diagnoses conflicts.
- Metadata currently rejects an allowed-values field only when it exactly equals a rule's forbidden path. It accepts a nested restriction below a forbidden parent, such as forbidding `error` while restricting `error.type`.
- Schema first writes the forbidden parent as `false`; `descend` then replaces that boolean with a new object schema for the nested allowed-values field. Go emits both checks and still rejects the parent. The generated TypeScript validator therefore weakens a rule that the Go validator preserves.
- A literal union variant likewise may restrict a nested field whose root is claimed only by another variant. Depending on path depth, Schema either panics on the conflict or replaces the implicit forbidden node, while Go retains the branch prohibition.
- First-party registrations do not use either malformed combination, so the generated artifacts and prior gates remain green; the unsupported states are reachable through the private registration API.

### Root cause

The metadata owner validates allowed-value leaf type and exact duplicate/conflict cases, but does not own hierarchical path compatibility or union-branch ownership. The Schema compiler consequently receives states it cannot project consistently and attempts to rediscover conflicts while mutating its tree.

### Impact and acceptance criteria

- Reject allowed-values paths that overlap a conditional rule's forbidden path at any ancestor/descendant boundary.
- Require every literal-variant allowed-values path to be covered by one of that variant's required or optional claims; an ancestor claim covers its nested values.
- Preserve valid first-party nested restrictions such as a required `error` frame narrowed at `error.type`.
- Split Schema enum-subset checking and node application only after metadata closes the structural matrix; add no generic path framework or compatibility fallback.
- Pass focused negative/positive tests, generated drift, race, static, full Runtime/CLI, and bounded live verification.

### Plan

- **Completed:** added failing metadata regressions, closed hierarchical and branch-ownership validation, simplified Schema application, and remeasured complexity.
- **Completed:** regenerated, ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- Before the fix, the conditional parent/child conflict returned no registration error and the cross-variant nested restriction also validated successfully. Both focused regressions failed for the intended reason.
- After metadata validation moved to the owner, both negative regressions and all first-party shape registrations passed. Existing required-parent/nested-value registrations prove that a branch claiming `error` may still narrow `error.type`.
- Focused dispatch, contractgen, and architecture tests passed in 5.91s after the semantic fix and in 5.92s at the final gate. `go generate ./...` changed no Go or TypeScript artifact.
- Production-only complexity scanning no longer reports Schema `restrictAllowedValues` or metadata `validateAllowedValueSets`; none of the new per-set, overlap, value-list, branch-claim, enum-subset, or node-application helpers exceeds the selected threshold.
- Dispatch and contractgen passed with `-race`; focused `go vet` and `staticcheck` also passed in the combined 19.18s gate.
- Runtime `GOWORK=off go vet ./...` plus `GOWORK=off go build ./...` passed in 9.59s, and uncached `GOWORK=off go test -count=1 ./...` passed in 60.73s.
- Current-workspace CLI `go vet ./...` plus `go build ./...` passed in 9.61s, and uncached `go test -count=1 ./...` passed in 49.45s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND19_OK`, status `completed`, one step, 9,183 input tokens, 8 output tokens, 9,088 cache-read tokens, and 722ms total model duration. No credential value was printed or copied.
- `git diff --check` passed.

### Changes and compatibility

- Allowed-values metadata now rejects any ancestor/descendant overlap with a conditional forbidden path and requires each literal-variant restriction to live at or below a field claimed by that variant.
- Schema application now receives a closed structural matrix. Its remaining work is split into path resolution, enum-subset validation, and leaf-node narrowing without changing valid output.
- Breaking behavior is limited to earlier rejection of malformed private registrations that could not be projected consistently. First-party metadata, generated bytes, public APIs, valid wire shapes, persistence, diagnostics, and Runtime/CLI behavior are unchanged.

### Resource cleanup

- The temporary strict complexity configuration was deleted after the final scan.
- The bounded live Run used a validated `/tmp/flame-live-round19.*` directory containing its isolated Runtime home and current-source CLI binary. The exit trap moved it to the system Trash; no matching temporary path remained.
- No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- `Goal.provider/model` remains an evidenced cross-artifact inconsistency blocked by the current Runtime/CLI-only scope. A future scope that includes Desktop should remove both `omitempty` tags, regenerate the contract, and migrate every Goal fixture/adapter in one batch.
- Round 20 will audit TypeScript `checkEmitter.compile`, now the next generated-client cognitive finding, against Schema conjunction, union, conditional, primitive, object, array, and reference nodes before deciding whether its node-family boundaries are safe to extract.

## Round 20 — complete

### Audit scope and evidence

- `checkEmitter.compile` is the next generated-client cognitive finding at 20. It resolves references, chooses the base const/enum/type check, appends string and numeric bounds, appends collection/object checks, expands composition keywords, and finally normalizes the conjunction shape.
- The append order is externally visible in generated source and violation ordering. The current sequence is base value, string rules, numeric rules, collection rules, `oneOf`, `anyOf`, flattened `allOf`, then `if/then`.
- `format` and `contentEncoding` were checked as possible omissions. Runtime's `wireCheck.ts` explicitly defines them as JSON Schema annotations rather than assertions; timestamps and byte slices are validated by their typed decoding boundaries. Strengthening them here would change the declared policy rather than repair drift.
- Reference nodes are emitted as pure references by `schemaSet.define`; directly owned constraints are not attached to a reference, while owner-local nested constraints ride separate `allOf` branches. The early reference return therefore preserves the schema owner's representation.
- Baseline focused contractgen and architecture tests passed in 2.32s; production complexity scanning reports `checkEmitter.compile` at 20.

### Root cause

One function linearizes every Schema keyword family even though the order between families is fixed and each family contributes an independent list of checks. The complexity is vocabulary breadth, not a cross-family algorithm.

### Impact and acceptance criteria

- Extract concrete base, string, numeric, collection, and composition renderers while keeping `compile` as the single reference/conjunction entry point.
- Preserve exact check order, recursive compilation, primitive usage tracking, indentation, generated imports, and generated bytes.
- Do not reinterpret annotation keywords or introduce a generic keyword registry, visitor, or mutable compile configuration.
- Reduce every new function below the selected complexity threshold.
- Pass focused, drift, race, static, full Runtime/CLI, and bounded live verification.

### Plan

- **Completed:** extracted the five proven keyword families, formatted them, compared generated output, and remeasured complexity.
- **Completed:** ran focused/full/live verification, cleaned resources, inspected compatibility, and recorded results.

### Validation

- The first post-change focused contractgen and architecture run passed in 3.96s. `go generate ./...` then produced no diff in the TypeScript validator, TypeScript types, or Go validator.
- The final focused generated-drift, TypeScript, union, optionality, and cross-artifact tests passed in 2.62s against the 2.32s green baseline.
- Production-only complexity scanning no longer reports `checkEmitter.compile`; none of the base, string, numeric, collection, composition, or conjunction helpers exceeds the selected threshold. Contractgen now has one pre-existing cognitive finding and two pre-existing cyclomatic findings.
- Contractgen passed with `-race`; focused `go vet` and `staticcheck` also passed in the combined 12.15s gate.
- Runtime `GOWORK=off go vet ./...` plus `GOWORK=off go build ./...` passed in 5.08s, and uncached `GOWORK=off go test -count=1 ./...` passed in 44.66s.
- Current-workspace CLI `go vet ./...` plus `go build ./...` passed in 5.36s, and uncached `go test -count=1 ./...` passed in 47.26s.
- The current-source CLI completed a bounded production-bootstrap Run using the authorized DeepSeek configuration. It returned exactly `FLAME_LIVE_ROUND20_OK`, status `completed`, one step, 9,183 input tokens, 8 output tokens, 9,088 cache-read tokens, and 848ms total model duration. No credential value was printed or copied.
- `git diff --check` passed.

### Changes and compatibility

- TypeScript check compilation now has concrete renderers for the base value, string, numeric, collection/object, and composition keyword families. The entry point still exclusively owns reference resolution and conjunction assembly.
- Check order, recursive expansion, used-import discovery, indentation, generated bytes, diagnostics, public APIs, wire shapes, persistence, and Runtime/CLI behavior are unchanged.
- `format` and `contentEncoding` remain annotations exactly as the Runtime-owned TypeScript vocabulary documents; no new client-side policy was invented.

### Resource cleanup

- The temporary strict complexity configuration was deleted after the final scan.
- The bounded live Run used a validated `/tmp/flame-live-round20.*` directory containing its isolated Runtime home and current-source CLI binary. The exit trap moved it to the system Trash; no matching temporary path remained.
- No shared cache, global dependency, user Runtime data, or configuration was removed.

### Remaining risk and next direction

- `Goal.provider/model` remains an evidenced cross-artifact inconsistency blocked by the current Runtime/CLI-only scope. A future scope that includes Desktop should remove both `omitempty` tags, regenerate the contract, and migrate every Goal fixture/adapter in one batch.
- Round 21 will audit `reference`, contractgen's final cognitive finding, against its generated API-reference sections and manifest ownership. It will split only independently authored document sections while preserving the Markdown bytes exactly.
