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
| R1 | `sessions.MaterialSnapshot` includes Session in one storage read, but `protocol.SessionSnapshot` omits it. CLI combines separate reads with eight stability attempts. | Runtime returns the complete coherent mounted-session projection. Remove CLI metadata pairing, retry count, and equality machinery; update protocol artifacts and consumers. | Pending |
| R2 | `SessionStores`, `WorkingContextComposer`, `InteractionExecutor`, and title finalization allow absent dependencies that the production composition always supplies. | Require complete collaborators at construction. Remove impossible missing-capability execution branches. Retain actual checkpoint, sandbox, and tool-result-offload policies. | In progress: SessionStores, working context, and title finalization complete |
| R3 | Fixed product policy is represented by optional tuning bags; test-only maintenance and restore-scope overrides create alternate production paths. Shutdown wraps a fixed timeout in repeated validation. | Give fixed policy one owner. Remove replacement paths with no product consumer. Keep narrow test controls only where they isolate an actual external boundary or deterministic lifetime. | Pending |
| R4 | Executor composition repeats BuildID as ImplementationIdentity and adds a hand-maintained configuration identity beside serialized configuration. | Derive deployment identity from the real executable and configuration facts. Remove synonymous identity inputs and wrappers without weakening Scope deployment compatibility. | Pending |
| R5 | Recovery and waiting-subtree cancellation validate constructed immutable write sets again in persistence. Some validators replay planner transitions. | One owner constructs each complete decision. Persistence checks transactional expectations, not a second recovery policy. Remove redundant construction surfaces and repeated proof machinery. | Pending |
| R6 | Session restore, fork, and rollback repeatedly normalize, copy, and validate complete snapshots through write-plan construction and application. | Decode and validate external input at its boundary; acquire mutable ownership once per retained owner. Apply an established immutable write plan without another full reconstruction. | Pending |
| R7 | Restored immutable aggregates are fully revalidated by replacements, queries, snapshots, and persistence. | Aggregate construction owns intrinsic validity. Use cases own cross-aggregate relationships; storage owns decoding and current-state matching. Remove duplicate intrinsic validation while retaining zero-value and external-boundary admission. | Pending |
| R8 | The live registry writes CancelReason but has no production reader. The Run-tree cancellation arbiter owns the consumed reason. | Remove registry cancellation state and writes. Keep the arbiter as the sole cancellation-reason owner and retain observable cancellation coverage. | Complete |
| C1 | CLI mirrors Runtime Run/Session rules, projections, and product error identities. | Consume Runtime protocol values and errors directly. Keep CLI-owned conversation folding, drafts, previews, selection, and rendering state. Remove synonymous models, validators, and error translations. | Pending |
| C2 | CLI mutation acknowledgements repeat Session revision/normalization/model rules and MCP/Provider update semantics. | Runtime owns mutation postconditions. CLI retains wire and target-identity checks, local form state, and credential protection. Remove duplicate business-rule validation and its dedicated tests. | Pending |
| C3 | CLI partitions change subscriptions and coordinates several streams although production requests at most 14 topics and one watch against limits of 32 each. | One terminal subscription with normal gap recovery, cancellation, and resynchronization. Remove partitioning, fan-out, and cross-subscription file ownership. | Pending |
| T1 | Small unused or test-only methods remain around the preceding mechanisms. | Delete only after checking direct, interface, generated, platform, and serialized consumers; migrate tests to surviving production contracts. | In progress: Runtime residue removed except compaction result |

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

If deeper consumer evidence invalidates a proposed deletion, record the
surviving requirement here instead of weakening it to satisfy a line-count target.
