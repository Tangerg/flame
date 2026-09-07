# Runtime and CLI ownership convergence

This document records the user-requested audit and implementation contract for
Runtime and CLI convergence. The audit baseline is `be0ac77b`. Breaking changes
are authorized: fix the semantic owner, migrate all in-scope consumers, and
remove obsolete representations, APIs, fallbacks, tests, and documentation in
the same verified batch.

## Scope and completion rule

The implementation scope is `runtime`, `runtime/localruntime`, `cli`, and their
repository documentation. Desktop work is independent and must not be edited
or generated into. Runtime-owned generated protocol artifacts remain in scope.
Any integration implication for an out-of-scope consumer must be recorded.

A finding is complete only when its redundant contract is removed and the
surviving observable behavior is verified. Moving checks into another helper,
adding an alias, or retaining a fallback does not complete a finding. Necessary
trust-boundary checks, resource ownership, and product capabilities remain.

## Audit and target contracts

| ID | Finding and evidence at the baseline | Target contract and deletion scope | State |
| --- | --- | --- | --- |
| R1 | `sessions.MaterialSnapshot` includes Session in one storage read, but `protocol.SessionSnapshot` omits it. CLI combines separate reads with eight stability attempts. | Runtime returns the complete coherent mounted-session projection. Remove CLI metadata pairing, retry count, and equality machinery; update protocol artifacts and consumers. | Complete |
| R2 | `SessionStores`, `WorkingContextComposer`, `InteractionExecutor`, and title finalization allow absent dependencies that the production composition always supplies. | Require complete collaborators at construction. Remove impossible missing-capability execution branches. Retain actual checkpoint, sandbox, and tool-result-offload policies. | Complete |
| R3 | Fixed product policy is represented by optional tuning bags; test-only maintenance and restore-scope overrides create alternate production paths. Shutdown wraps a fixed timeout in repeated validation. | Give fixed policy one owner. Remove replacement paths with no product consumer. Keep narrow test controls only where they isolate an actual external boundary or deterministic lifetime. | Complete |
| R4 | Executor composition repeats BuildID as ImplementationIdentity and adds a hand-maintained configuration identity beside serialized configuration. | Derive deployment identity from the real executable and configuration facts. Remove synonymous identity inputs and wrappers without weakening Scope deployment compatibility. | Complete |
| R5 | Recovery and waiting-subtree cancellation validate constructed immutable write sets again in persistence. Some validators replay planner transitions. | One owner constructs each complete decision. Persistence checks transactional expectations, not a second recovery policy. Remove redundant construction surfaces and repeated proof machinery. | Complete |
| R6 | Session restore, fork, and rollback repeatedly normalize, copy, and validate complete snapshots through write-plan construction and application. | Decode and validate external input at its boundary; acquire mutable ownership once per retained owner. Apply an established immutable write plan without another full reconstruction. | Complete |
| R7 | Restored immutable aggregates are fully revalidated by replacements, queries, snapshots, and persistence. | Aggregate construction owns intrinsic validity. Use cases own cross-aggregate relationships; storage owns decoding and current-state matching. Remove duplicate intrinsic validation while retaining zero-value and external-boundary admission. | Complete |
| R8 | The live registry writes CancelReason but has no production reader. The Run-tree cancellation arbiter owns the consumed reason. | Remove registry cancellation state and writes. Keep the arbiter as the sole cancellation-reason owner and retain observable cancellation coverage. | Complete |
| C1 | CLI mirrors Runtime Run/Session rules, projections, and product error identities. | Consume Runtime protocol values and errors directly. Keep CLI-owned conversation folding, drafts, previews, selection, and rendering state. Remove synonymous models, validators, and error translations. | In progress: product errors complete; projections remain |
| C2 | CLI mutation acknowledgements repeat Session revision/normalization/model rules and MCP/Provider update semantics. | Runtime owns mutation postconditions. CLI retains wire and target-identity checks, local form state, and credential protection. Remove duplicate business-rule validation and its dedicated tests. | Pending |
| C3 | CLI partitions change subscriptions and coordinates several streams although production requests at most 14 topics and one watch against limits of 32 each. | One terminal subscription with normal gap recovery, cancellation, and resynchronization. Remove partitioning, fan-out, and cross-subscription file ownership. | Pending |
| T1 | Small unused or test-only methods remain around the preceding mechanisms. | Delete only after checking direct, interface, generated, platform, and serialized consumers; migrate tests to surviving production contracts. | In progress: Runtime complete; CLI candidates remain |

### T1 consumer-checked candidates

Runtime candidates are `mcp.clonePointer`, `git/process.Command`,
`Schedule.NextRevision`, `ChildRunStartReservationStore.DeleteAll`,
`toolset.fingerprintOf`, `toolset.fingerprintFile`, `Compaction.Cutoff`,
`Compaction.ReplacementPrefix`, `UsageSummaryPeriod.Days`, and
`ModelContextCompactionResult.Changed`.

CLI candidates are `projectUniqueValues`, `newUserMessageBlock`,
`interactionReview.Position`, and `Block.Identity`. A test-only convenience may
move into a test file when it still makes an observable-contract test clearer.

## Boundaries that remain

- Scope owns framework execution, process trees, effects, and checkpoints.
- Runtime retains commit identities, CAS, atomic write sets, waiting barriers,
  strict persisted-state decoding, and recovery ownership.
- External MCP/LSP connections and child processes retain cancellation,
  settlement, reconnect, and shutdown mechanisms.
- Runtime events remain observations; completed Items and coherent snapshots
  remain authoritative after gaps and cold recovery.
- CLI plugin discovery, contribution ownership, unload, and dependency ordering
  have production consumers and remain supported.
- Protocol generation and validation, filesystem confinement, and credential
  redaction retain their actual boundary protections.
- Narrow consuming interfaces and small responsibility-owned packages are not
  deletion candidates solely because they have one implementation or few lines.

## Implementation order and verification

1. Complete Runtime construction, remove duplicate cancellation state and
   confirmed residue, then converge fixed policy and deployment inputs.
   Verify startup, shutdown, execution, and cancellation.
2. Converge execution/recovery and Session write-plan validation. Verify Goal,
   Plan, HITL, subtree cancellation, fork, rollback, compaction, restart, and
   recovery through one Runtime.
3. Complete the Runtime snapshot contract and migrate CLI projections, product
   errors, and mutation acknowledgement semantics. Regenerate Runtime contracts;
   verify binding/protocol parity and live/cold client recovery.
4. Converge terminal subscriptions and remaining presentation residue. Verify
   sequence gaps, file changes, reconnect, cancellation, and terminal shutdown.

Each ownership batch gets focused tests followed by proportionate module
checks, `git diff --check`, explicit-path staging, a commit, and a push before
the next risky batch. Standalone Runtime and CLI validation uses `GOWORK=off`
with the actual released module graph. CLI pins verified Runtime commits through
the module proxy; no local replacement or alternate production binding is added.

## Implementation decisions and evidence

### Complete Session persistence and cancellation ownership

+ `SessionStores` now rejects incomplete construction; all production and test
  callers supply the full durable graph. Session reads, restore, fork, rollback,
  and deletion no longer interpret missing stores as disabled product features.
+ The live registry no longer stores cancellation reasons. The Run-tree arbiter
  supplies the reason for the addressed Run through its existing production query.
+ Removed the consumer-checked Runtime T1 candidates except the compaction-result
  flag, which is handled with its result contract. File-read and cancellation
  tests now exercise the real observation path. Session-owned reservation cleanup
  remains covered; the unused whole-table cleanup operation is gone.
+ Verified focused persistence, bootstrap, cancellation, toolset, SQLite, and
  Domain checks, followed by Runtime `GOWORK=off go test ./... -timeout 3m`,
  `go vet ./...`, `go build ./...`, and `git diff --check`.

### Complete working context and title finalization

`WorkingContextComposer` requires Knowledge, agent-memory content and search,
Plan, Goal, and Hook sources. Empty content remains valid; missing implementations
cannot silently remove context or policy. Initial title finalization requires
Session title operations, a generator, and a lifecycle-owned task launcher.
Workspace checkpoint availability remains an actual host capability.

Narrow tests supply complete context fixtures or substitute the consumed
finalization port. Production no longer offers incomplete construction for
those fixtures. Verified focused context, finalization, and bootstrap checks,
then Runtime tests, vet, build, and whitespace checks.

### Complete execution collaborators

`InteractionExecutor` requires its Tool catalog, interpretation, presentation,
authorization, MCP approval policy, Hooks, maintenance, compaction, and current
Session-state sources. Removed all absent-collaborator execution branches,
including the unmaintained model-context path and constant configuration flags.
Tests now exercise the same reducer path with substituted external capabilities.
Tool-result offload still requires its store only when enabled; model pricing
availability remains a model capability checked when a monetary limit needs it.

Verified execution, delegation, approval, waiting restoration, Hook failure,
compaction, maintenance, and bootstrap tests, then Runtime tests, vet, build,
and whitespace checks.

### Deployment identity and restore-scope authority

The executor uses the validated executable BuildID directly for Scope's
implementation digest. Its configuration digest encodes the actual dispatcher,
Tool catalog, model identity, instructions, and delegation facts. Removed the
synonymous implementation input, hand-maintained configuration identity, and
their private wrapper and dedicated tests.

Checkpoint restore and probing share the real workspace admission function.
Removed the test-only replacement callback. The recovery test now changes an
actual response-mode policy to prove incompatible deployment rejection;
isolated or unavailable workspaces remain rejected by the same host boundary.
Verified focused executor and bootstrap recovery, then Runtime tests, vet,
build, and whitespace checks.

### Fixed execution policy

Removed optional executor tuning fields and their resolved-policy bag. The
executor owns its production Tool concurrency, buffers, reconciliation periods,
and default model-call allowance directly. A Run's explicit step limit still
controls its model-call allowance. Delegation constructs one fixed framework
structural policy and work allocation without optional-value normalization.

Tests exercise the production concurrency and reconciliation cadence. Streaming
overflow coverage supplies more data than the real buffer can retain. Response
mode remains an external dispatcher boundary needed to verify both complete
and streaming model effects; offload retains its actual product configuration.
Verified focused execution and bootstrap checks, then Runtime tests, vet, build,
and whitespace checks.

### One maintenance pipeline and bounded shutdown wait

Bootstrap always builds the real post-Run maintenance pipeline. Its memory
consolidator is required; Skill workers remain conditional on the user Skill
library. Removed the whole-pipeline override. Long-context test models now
answer memory extraction separately from compaction instead of suppressing
maintenance or counting every Tool-free request as a summary.

Removed the shutdown timeout wrapper, constructors, getter validation, and
wrapper-specific tests. The lifecycle initializes its private caller-wait
duration directly. Narrow tests still shorten this duration to prove that a
caller timeout cannot abandon the owned cleanup graph or start another closer.
Verified maintenance, Goal/Plan compaction, protocol lifecycle, startup rollback,
and shutdown tests, then Runtime tests, vet, build, and whitespace checks.

### Fixed memory and Skill maintenance policy

Memory curation, Skill mining, and idle Skill archival now use their owned
production bounds directly. Removed the three optional policy bags, normalized
copies, default-validation helpers, and tests for unsupported tuning modes.
Tests supply real backlog sizes, model output sizes, completed-Run counts, and
clock values to exercise the actual gates. Skill revision mining requires its
active-Skill source; an empty library remains source-owned data.

Verified curation failure/recovery and watermark preservation, Skill proposals
and revision, archival cadence and concurrent admission, then Runtime tests,
vet, build, and whitespace checks. The compaction threshold override is handled
separately at the model-capacity boundary.

### Model-capacity compaction policy

Compaction derives its threshold from the selected model's context window,
hard input capacity, and requested output reservation. Removed the optional
threshold override and resolved policy wrapper. Small-capacity tests supply
model metadata to the same compaction algorithm; public-entrypoint tests retain
catalog capacity and complete Runtime Goal/Plan lifecycle coverage.

Removed the unused compaction-result change flag. Tests observe effective
messages, durable rewrites, and summary calls directly. Verified focused
compaction, execution, and bootstrap tests, followed by Runtime tests, vet,
build, and whitespace checks.

### Recovery decisions are established before persistence

Recovery and waiting-subtree cancellation commits no longer expose full
revalidation. Their constructors own the complete write-set relationship check;
persistence rejects an unconstructed value before beginning a transaction and
then enforces exact stored-state matching, commit receipts, and atomic effects.
Recovery retains a private owned state, including an explicit distinction
between an empty constructed reconciliation and the zero value.

The private recovery planner already obtains Run and Item transitions from
Domain behavior, so construction no longer calls those transitions again or
compares a second reconstructed aggregate. Cross-aggregate ownership, finish
times, Goal accounting, conversation watermarks, and cleanup scope remain checked.
The parked/resuming cancellation constructors still admit multiple independently
supplied facts; their one-time exact transformation proof protects that boundary
and is retained. Tests cover constructor rejection and isolated accessors rather
than validating an established commit again.

Verified lost-tree recovery, preserved waiting trees, Goal accounting, child
cancellation, stale claims, rollback, replay and lost receipts, including public
bootstrap lifecycle coverage. Runtime tests, vet, build, and whitespace checks
passed.

### Session write-plan construction owns normalization and validity

Restore, fork, rollback, deletion, and parked termination now expose immutable
plans with zero-value admission instead of public full revalidation. The
constructors establish their complete contract once. Rollback parses and checks
uniqueness in one pass; fork validates child ownership and initial revision at
construction. Transaction adapters retain current-state and revision matching,
atomic writes, and durable failure ordering.

Snapshot normalization validates the complete mutable projection once, converts
offloaded results to previews, and gives the plan its owned Items. Conversation
validation borrows messages without constructing a discarded Conversation;
the plan clones mutable content when it retains it. Removed earlier fork and
restore normalization, post-construction snapshot rebuilding, and repeated
Plan-replacement validation in the adapter. External archive decoding and
returned mutable projections remain explicit ownership boundaries.

Verified malformed snapshot and plan rejection, source/accessor isolation,
parent-first restore, forked Run/Item/blob identities, rollback, restore failure
atomicity, and parked termination. Runtime tests, vet, build, and whitespace
checks passed.

### Immutable aggregates establish their own validity

Run, Item, Session, and committed Plan states no longer expose full aggregate
revalidation. Strict restoration validates their complete representation; legal
transitions validate their changed facts. Consumers admit zero values where
needed and check expected identity, ownership, ordering, and cross-aggregate
relationships. Replacements establish their relationship once and storage
matches the current value or revision without repeating aggregate construction.
Removed the EventCommit's temporary watermark rewrite used only for validation.

Plan Current is either absent or constructed from a committed State. Plan
Version derives committed presence from its owned revision, and Session
replacement derives initial insertion from the absence of an expected revision.
Removed the two duplicate flags and the contradictory states they enabled.
Stored-state codecs still use strict constructors. Run restoration explicitly
rejects unknown states, and Session edits validate exact model selection at the
point it changes instead of relying on a later whole-aggregate pass.

Verified aggregate lifecycle, immutable accessors, malformed restoration,
replacement revision and ownership, coherent snapshots, recovery, waiting
cancellation, and SQLite CAS/round trips. Runtime tests, vet, build, and whitespace
checks passed; the final constructor simplification also passed focused Domain
checks.

### Coherent Session snapshot projection

The Runtime snapshot response includes Session metadata and activity derived
from the same transactional material read as Runs, Items, Plan, Goal, and
interrupts. Application shares the existing activity projection and never makes
an independent Session or Run query. Workspace availability remains a live
filesystem observation of the stored workspace path.

Updated the public Go response and generated Runtime contracts. Verified
Session metadata/activity agreement, one-read Application projection, binding
responses, then Runtime tests, vet, build, and whitespace checks. CLI pins the published Runtime commit and consumes this response directly.
Removed the separate metadata query, eight-attempt stability loop, equality
function, and intermediate cold-read DTO. CLI focused adapter and complete
module tests, vet, build, and whitespace checks passed. Desktop is
outside this batch; its owner must regenerate and consume the required `session`
field when updating its Runtime protocol contract.

### Runtime product errors retain their original identity

Removed fourteen CLI-owned product errors and their identity mapping. Command
confirmation, reconnect, cold recovery, deletion, rollback, steering, and
terminal cancellation now branch on Runtime protocol errors directly. Problem
formatting still preserves the underlying error and structured recovery data;
connection closure and incompatible-protocol classification remain CLI concerns.
Test fixtures emit the same Runtime errors as the real binding.

Verified adapter error identity, command replay uncertainty, retries, steering,
recovery, and Session workflows, then complete CLI tests, vet, build, and
whitespace checks.

If deeper consumer evidence invalidates a proposed deletion, record the
surviving requirement here instead of weakening it to satisfy a line-count target.
