# Flame Runtime and Apache Maka

Analysis date: 2026-09-08.

This document records the requested comparison of Flame Runtime with Apache
Maka's agent design documents and local source checkout. It distinguishes
observed implementation behavior from recommendations. It is an analysis
snapshot, not a replacement for Flame's [design philosophy](../DESIGN_PHILOSOPHY.md)
or [Runtime architecture](../runtime/doc/ARCHITECTURE.md).

## Scope and evidence

The review began against the Flame worktree at HEAD `7ed9b366`. The Maka
reference was `/Users/tangerg/Desktop/maka`, commit `8b3655a1`, dated
2026-09-08; the Scope reference was `/Users/tangerg/Desktop/scope`, commit
`179de3ff`. Source references identify inspected implementations; paths in
either worktree may change as development continues.

The primary Maka evidence is four architecture chapters, each carrying its own
`last_verified` date: Chapter 1 runtime core (2026-08-23), Chapter 3 compaction
projection (2026-08-28), Chapter 7 graph scheduling (2026-08-23), and Chapter 8
resume (2026-09-02). Three `docs/blogs/` essays restate the same arguments.
Those chapters are marked `document_status: draft` and separate implemented
behavior from planned phases; planned work must not be read as current
behavior. Maka's `DESIGN.md` was inspected and excluded — it is a UI design
system, not agent design.

The comparison covers fact ownership, persistence, compaction, tool boundaries,
recovery, and multi-agent scheduling. It is not a security audit, a performance
benchmark, or a review of every Maka subsystem. Maka's `docs/` contains 139
files; subsystems outside the areas above were not examined.

## Assessment

Maka and Flame have independently converged on the same layering: one hosted
execution authority, one framework boundary beneath it, and clients that own
presentation only. That convergence is the reason the remaining difference is
informative rather than incidental.

The difference is concentrated in one place. Maka's canonical record is an
append-only log of typed semantic facts, and model context, UI transcript, Run
terminal state, recovery, and context budget are five projections over it.
Flame's canonical record is current Domain and Application state plus Scope
executor checkpoints, and its compaction path replaces stored history outright.

Flame should retain its Run/Segment distinction, its compaction **trigger**
discipline, its bounded non-persistent live journal, and its claim and T1/T2
mechanisms. All four are already the stricter answer. The useful Maka lessons
concern what happens to a fact after it is committed: whether compaction may
destroy its source, whether an invocation record survives its Run, and what a
continuation is allowed to assume about the prefix it replays.

Flame's multi-agent direction should not change. Scope's process tree with
per-process mailboxes is one of the two paths Maka's own material identifies,
and Scope's kernel is the stricter of the two.

## Compare equivalent responsibilities

| Concern | Maka | Flame |
| --- | --- | --- |
| Sole execution authority | Runtime Host | Runtime |
| Framework beneath it | in-repo `packages/runtime` | released Scope modules |
| Clients | Desktop, TUI, CLI, bots, eval | CLI, Desktop |
| Client authority | presentation and triggers only | presentation and interaction only |
| Evaluation semantics | `packages/eval` | `scope/eval` |
| Canonical fact record | append-only RuntimeEvent log | current Domain/Application state |
| Compatibility stance | replace the wrong shape outright | replace the wrong shape outright |

## Convergent answers that need no change

These were reached separately and are recorded so a future reader does not
mistake them for gaps.

| Concern | Maka mechanism | Flame mechanism |
| --- | --- | --- |
| Tool schema cost | Deferred Tools plus `tool_search` | `adapter/toolset` discovery with deferred names |
| Oversized tool results | archive first, placeholder second | `tool_result_blobs`, `history_items.offload_id` |
| Side-effect boundary | T1 dispatch commit, T2 outcome commit | `tool_invocations` `started` → `completed`/`incomplete` |
| Duplicate admission | conditional claim before execution | `child_run_start_reservations` |
| Subagent context | no automatic parent-history inheritance | Delegate with its own child session |
| Unproven external effects | park, never guess | unknown external effects fail closed |
| Compaction trigger | real provider usage against declared capacity | imminent-call request footprint |
| Provider identity | exact pair, credentials at the boundary | exact provider/model pair plus model-owned options |

Two deserve emphasis. Flame's compaction trigger discipline is already the
stricter half of that design: deciding only at an imminent model call from that
call's complete footprint, and refusing message counts and Run completion as
pressure signals, is the conclusion Maka reaches after rejecting
`finishReason: length` as actionable evidence. And `tool_invocations` already
carries the T1/T2 shape that Maka's Chapter 8 spends its length justifying.

## Fact ownership is the structural difference

Maka's relationship is explicit:

```text
State(t) = Project(RuntimeEvents[0..t], policy, configuration)
```

A projection may be rebuilt, versioned, or discarded. It may never advance a
fact on its own.

Flame's durable stores are:

| Store | Table | Mutation | Role |
| --- | --- | --- | --- |
| Transcript | `history_items` | append, CAS replace, delete by Run/Session | durable observable history |
| Conversation | `messages` | atomic whole-history replace | model context |
| Invocation ledger | `model_invocations`, `tool_invocations` | insert, state transition, trigger delete at Run terminal | open-attempt reconciliation |
| Executor checkpoint | `executor_checkpoints` | framework snapshot | recovery |
| Live stream | in-process `journal` | bounded window, per Segment, process-scoped | client streaming and reconnect |

`history_items` is closer to a log than the surrounding model implies: monotonic
`seq`, unique `item_id`, and its own offload column. The missing piece is not a
substrate. It is the statement that one store is canonical and the others are
derived.

Today none is. `domain/run/conversation` documents its own independence from
Runs, transcript observations, and executor working state.
[Runtime architecture](../runtime/doc/ARCHITECTURE.md) then needs an arbitration
rule: a completed durable Item or snapshot wins over a missing or duplicated
preview event. A rule about which representation wins is evidence that two
representations can advance independently. Under one owner with pure
projections the rule is unnecessary, because a projection has nothing to win
with.

This is a gap between Flame's stated principle and Flame's persistence layer,
not an imported opinion. [Design philosophy](../DESIGN_PHILOSOPHY.md) already
requires one fact, one owner, and requires every other representation to
encode, cache, or render that fact without creating a competing transition.

## Compaction destroys the history it compacts

Flame's compaction replaces a Session's whole message history with the compacted
set in one transaction, then rebases every Run watermark in the same
transaction. The store method is honest about its shape: delete the existing
rows, insert the new ones, roll back together.

Maka's compaction instead produces a checkpoint carrying coverage — event count,
Turn count, a `through` boundary, and a SHA-256 digest over the covered prefix.
The next request materializes `checkpoint + uncovered raw tail`; the log is
untouched. A checkpoint that no longer matches its prefix is rejected as
`coverage_miss` or `source_hash_mismatch` rather than trusted because it
resembles the current history.

Three consequences follow from Flame's current shape:

1. **Compacted history is unrecoverable.** Re-reading a task under a stronger
   model, auditing what the model was actually shown, and reproducing a
   historical defect all require the pre-compaction messages.
2. **Row order is the coordinate system**, so rewriting history forces every Run
   watermark to be rebased. The rebase is a symptom of that coupling rather than
   necessary complexity: watermarks anchored to a monotonic event sequence would
   not move when a projection changes.
3. **A lossy artifact becomes the only record.** The summary influences every
   later decision with no source left to correct it against.

The substrate for the alternative exists. `history_items.seq` is already
monotonic and ordered; a compaction checkpoint would bind to a prefix of it, and
`Conversation` would become a value projected from checkpoint plus uncovered
tail rather than a separately stored sequence.

## The summary gate is weaker than the rewrite it authorizes

Flame's compactor rejects one condition: an empty summary. Maka validates the
generated summary against the same section template its prompt requests,
rejects text ending inside an open fence or other truncation marker, requires
proportional output for a large fold, spends exactly one stricter repair
attempt, and remembers malformed-input fingerprints per Session so an unchanged
doomed input is not dispatched again.

The asymmetry matters less than its interaction with the previous section. Maka
can afford a permissive gate because failure is recoverable: it fails open to
the raw source-derived projection and lets the provider decide. Flame cannot
fail open, because by the time a summary is written the source is gone. Weak
validation and destructive rewrite are each manageable alone and compound badly
together. Addressing either one reduces the other's blast radius.

Maka's failure posture transfers independently of any log work: fail open to a
valid source-derived context, never to an invented summary, and never let a
local size estimate stand in for a provider verdict.

## The invocation ledger is pruned at Run terminal

A storage trigger deletes `model_invocations` and `tool_invocations` when a Run
becomes terminal. As a lifecycle backstop for open attempts this is correct — a
terminal Run owns no external attempt. But the same rows are the only durable
record of what was dispatched and what came back, so the cleanup also discards
the dispatch and outcome facts.

Maka retains them permanently. They support audit, eval attribution, cost
accounting, and the question a defect report actually asks: what did that tool
return at the time.

The concepts are separable without touching the state machine: an open-attempt
set that reconciliation owns and may clear, and a durable invocation fact ledger
that lifecycle never prunes. This is a retention change rather than a redesign.

It also has a cross-repository consequence. `scope/eval` owns experiment, case,
comparison, ranking, and judge semantics, but attribution needs execution facts
that the Flame side currently deletes when a Run finishes.

## Continuation is not anchored to a verified prefix

Flame recovery reconstructs from durable Runtime state and public framework
checkpoints, validating identities, capabilities, budgets, and prompt digests
before restoring execution. Content digests exist in Flame over files,
knowledge, and skills — not over history.

Maka's continuation reads an immutable prefix to a recorded high-water mark,
verifies a digest over it, revalidates every boundary immediately before
execution, and rejects a mismatch as `runtime_offset_mismatch`. Its planner
gates are worth reading as a checklist independently of the mechanism: one
terminal event for the source, no pending permission, no unsettled background or
child operation, matching workspace identity, every historical tool still
available, and provider history that begins and ends at a legal boundary.

Maka is candid that its own version is unfinished here. Its current high-water is
an event count and an in-process claim, with immutable event-seq, a
domain-separated prefix digest, and a database uniqueness claim still ahead of
it. Treat the gates as evidence and the anchoring as a consequence of fact
ownership rather than independent work.

## Where Flame's design differs and should stay

**Segment versus new Run.** Flame models interruption and resume as another
Segment of one logical Run. Maka creates fresh Run, invocation, and Turn
identities and records a continuation-source lineage back to the source
high-water. Maka's choice follows from immutability: a new attempt must not
reuse an identity that already has committed facts. Flame's choice gives the
user one stable execution identity across an interruption, which is the better
product model. Adopting log projection does not require adopting Maka's identity
split; a Segment can name its own prefix boundary.

**Live stream ownership.** Flame's journal is explicitly not a second persistent
store — a bounded per-Segment replay window scoped by process epoch, with
history belonging to transcript reads. That boundary is already the log-first
discipline applied correctly, and it is why a cursor minted by a previous
process is refused rather than silently continued.

**Construction discipline.** Scope's explicit `Config` values, absent builders,
and concrete constructor returns have no counterpart in the Maka material and
need none.

## Directions to reject

**The Agent Graph DAG and its Coordinator.** Maka's Chapter 7 and its
multi-agent essay frame this as one of two paths, with Codex's mailbox model as
the other. Scope already implements the second: `agent` has a process tree,
per-process mailbox signal queues, tree durability, and portable snapshots. Its
kernel is the stricter of the two — a Step is a pure, discardable reduction that
may not perform I/O, and every external operation must be declared as an Effect
and executed outside the Step. A DAG coordinator would be the second runtime and
second scheduler that Scope's design review checklist exists to prevent.

Two mechanisms inside Maka's graph are already present in Flame and should not
be re-imported under new names: only the root agent may mutate topology, and
readiness is a side-effect-free recomputable projection while admission is an
atomic durable claim. `child_run_start_reservations` is that claim.

**Provider-native compaction as a general contract.** Maka's schema V3 carries
opaque provider compaction state bound to a connection slug and model ID, with
a portable text summarizer retained as a liveness fallback. It is a closed
single-variant union around one observed subscription protocol, not a public
API. Flame's rule that an optional provider capability stays a separate narrow
contract already covers this correctly.

## The structural observation

Scope's agent kernel is already shaped for log projection. `Definition` and
`Execution` form a two-interface waist; a Step is a pure candidate reduction
that may not call a model, run a tool, or perform other I/O; external operations
exist only as declared Effect values; and execution state crosses the waist as
bounded, defensively copied JSON. Pure reduction plus externalized effects plus
serializable state is the standard shape of an event-sourced state machine.

Scope then states that persistence is a caller responsibility — correctly, since
it is a framework and not an application platform. Flame is that caller, and
Flame currently supplies current-state persistence with a destructive compaction
rewrite, so the benefits Scope's kernel makes available are not being taken.

This is pending adoption rather than a settled disagreement. Scope already
publishes a `TreeDurability` port whose boundaries commit a full tree snapshot
against a digest-chained head, and it was present in the release Flame pins;
Flame runs the kernel in ephemeral mode instead.
[The Scope Agent adoption record](./scope-agent-adoption.md) covers the version
gap and the order of work.

That is the whole finding. Fact ownership and the compaction rewrite are one
item, not two; the remaining sections are either consequences of it or cheap
enough to do independently.

## The main architectural cost to watch

Maka states the costs of its own model plainly, and they are real:

- Storage does not shrink when the prompt shortens. Savings target inference
  context, not fact storage.
- Coverage, digests, lineage, replay gates, recovery projections, and
  diagnostics all have to be maintained. A bare summary implementation is much
  shorter.
- Source and shape validation do not prove semantic completeness. A
  well-structured summary can still be wrong.
- Rolling summaries accumulate lossy error. Recompaction from an earlier
  boundary stays possible only because the source survives.
- Archived payloads need lifecycle management of their own.

Against Flame's current shape the trade is narrower than it first appears. The
ordered substrate, the offload path, the T1/T2 boundary, and the claim mechanism
all exist. What is missing is naming one store canonical and deriving
`Conversation` from it instead of storing it twice.

The cost that does not appear on Maka's list is Flame-specific: making
`Conversation` a projection changes a Domain value's construction path, and the
watermark rebase it removes is currently load-bearing for fork and rollback.

## Recommended priorities

1. **Decide fact ownership.** Name `history_items` canonical, or state
   explicitly that Flame keeps parallel representations and owns the arbitration
   rule permanently. Everything else follows from this answer, and leaving it
   unanswered is what allows the two to drift.
2. **Stop compaction from destroying its source**, and anchor Run watermarks to
   the monotonic sequence rather than row order. These are one change. Sequence
   this against the `TreeDurability` work in
   [the adoption record](./scope-agent-adoption.md): its `PreviousTreeDigest`
   supplies the prefix anchoring that gap 4 above asks for, so the two should
   not be designed separately.
3. **Strengthen the summary gate** — structure, truncation markers, output
   proportionality, one bounded repair — and adopt fail-open-to-source
   semantics. Cheap, independent, and it reduces the current rewrite's blast
   radius immediately.
4. **Separate the open-attempt set from the invocation fact ledger** so the
   terminal trigger stops discarding audit and attribution evidence.
5. **Treat Maka's planner gates as a recovery checklist** for the existing
   continuation path, independently of prefix digests.

Items 3 and 4 are safe to take on their own. Items 1 and 2 need the blast-radius
discussion that [design philosophy](../DESIGN_PHILOSOPHY.md) requires before a
breaking schema change.

## Verification performed

Read in full: Maka Chapters 1, 3, 7, and 8; `docs/agent-swarm.md`;
`docs/blogs/log-is-the-runtime.md`, `beyond-function-calling.md`, and
`multi-agent-scheduling.md`; root `ARCHITECTURE.md`.

Inspected in Flame and Scope: `runtime/doc/ARCHITECTURE.md`; the SQLite schema,
terminal-prune trigger, whole-history replace, and transcript item store under
`runtime/internal/infra/sqlite`; compaction application and watermark rebase
under `runtime/internal/adapter/persistence`; the summary gate under
`runtime/internal/adapter/run/maintenance`; the live journal and its non-store
boundary under `runtime/internal/application/agent/runs`;
`runtime/internal/domain/run/conversation`; deferred tool discovery under
`runtime/internal/adapter/toolset`; and `scope/agent` package documentation with
its mailbox, tree durability, and checkpoint surfaces.

No Flame or Scope code was modified. Claims about Maka's planned phases are
taken from its own status markers rather than from its source.
