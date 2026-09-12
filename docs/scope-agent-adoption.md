# Flame Runtime and the Scope Agent kernel

Analysis date: 2026-09-08.

This document records which parts of the Scope Agent kernel Flame Runtime
currently uses, which parts it does not, and the order in which the unused
parts should be adopted. It distinguishes observed behavior from
recommendations. It is an analysis snapshot, not a replacement for Flame's
[design philosophy](../DESIGN_PHILOSOPHY.md) or
[Runtime architecture](../runtime/doc/ARCHITECTURE.md).

Read it together with [the Maka comparison](./maka-runtime-comparison.md). That
document argues that Flame keeps parallel representations of one fact; this one
records that the kernel already publishes the port which would remove the
parallel representation.

## Scope and evidence

The review began against the Flame worktree at HEAD `7ed9b366` and the Scope
worktree at `179de3ff`, both dated 2026-09-08. Flame's `runtime/go.mod` pins
every Scope module at `v0.14.0` with no `replace` directive, so the released
contract Flame compiles against is `agent/v0.14.0`, not the Scope worktree.

Version numbers are recorded as observed. A newer Scope release is expected, so
treat the target below as "the next release" rather than a fixed tag.

The comparison covers `EngineConfig` wiring, durable versus ephemeral mode,
snapshot storage, the capability model, strategy packages, and conformance
suites. It does not review the interaction strategy's own contract tests or
Flame's provider adapters.

## Assessment

Flame uses the kernel's execution model completely and its recovery model
partially. The Process state machine, signal and wait semantics, effect
dispatch, child composition, budgets, events, and deltas are all wired. What is
not wired is the durability port: `EngineConfig.TreeDurability` is nil, so the
kernel runs in ephemeral mode and Flame supplies its own recovery substrate
instead.

That is a gap in adoption rather than a design disagreement. `TreeDurability`
is not a new interface — it is present in `agent/v0.14.0`, the release Flame
already depends on, and `doc.go` in that release already documents ephemeral
mode as "the same state machine without calling the durability port". The Scope
worktree refines the protocol; it did not introduce it.

The practical consequence is narrow and specific. Ephemeral mode can only
checkpoint at quiescence, so an Effect that crossed dispatch without settling is
not recoverable from kernel facts, and Flame reconstructs that fact from its own
`tool_invocations` ledger. One fact then has two owners, and the Flame-side
owner is deleted by the terminal-prune trigger.

## The version gap

| Measure | Value |
| --- | --- |
| Flame's pinned Scope modules | `v0.14.0`, uniformly, no `replace` |
| Scope worktree | past `agent/v0.15.0` |
| Commits to `agent/` since `agent/v0.14.0` | 63 |
| Diff | 155 files, +8501 / −4680 |
| Breaking commits | 6 — two in `agent`, one each in `workflow` and `interaction`, two in `planning` |

Five of those commits change semantics that a host implementation of the
durability port depends on:

| Commit | Change |
| --- | --- |
| `e5ed6b9e1` | names committed state and tree runtime ownership (breaking) |
| `a9a464444` | separates initialization acceptance from tree commits |
| `981690fde` | carries durable writer identity through effect dispatch |
| `a10e9be1f` | acknowledges signal admission after durable publication |
| `b1ca5d9a8` | derives settlement presence from the effect boundary kind |

`a9a464444` is the one that changes the port's shape. In `v0.14.0`,
`TreeDurability` embeds `ProcessStartOutcomeAcknowledger` and therefore requires
four methods; after that commit it requires three, and the acknowledger stays an
independent `EngineConfig` field. Flame already supplies that acknowledger, so
the upgrade makes the remaining work smaller rather than larger.

The order follows from this: upgrade first, then implement the port. Writing an
implementation against `v0.14.0` semantics means writing it twice.

## What Flame wires today

`EngineConfig` has ten fields. Flame's engine construction in
`runtime/internal/adapter/agentexec` sets eight:

| Field | Wired | Note |
| --- | --- | --- |
| `DeploymentResolver` | yes | exact local bindings |
| `ProcessAdmitter` | yes | product admission |
| `ProcessStartOutcomeAcknowledger` | yes | already one of the port's methods in `v0.14.0` |
| `EventListeners` | yes | framework event observation |
| `DeltaListeners` | yes | delta projection |
| `DeltaBufferCapacity` | yes | from Runtime policy |
| `Limits` | yes | `DefaultLimits()` |
| `TreeLimits` | yes | from the deployment catalog |
| `TreeDurability` | **no** | nil selects ephemeral mode |
| `Capabilities` | **no** | root authority set stays empty |

Strategy packages:

| Package | Files referencing it in Flame |
| --- | --- |
| `agent` (root kernel) | many |
| `agent/interaction` | 20 |
| `agent/planning`, `agent/planning/goap` | 0 |
| `agent/workflow` | 0 |
| `agent/agenttest` | 0 |

Using only `interaction` is the correct product decision today; Flame's product
model is a conversational agent with delegates, not goal search or deterministic
staged pipelines. This table records the fact, not a recommendation to adopt the
other two.

## Durable mode is the substantive gap

The two modes are mutually exclusive by construction. `CaptureTree` returns
`ErrTreeCaptureUnavailable` when a durability port is installed, and Flame calls
`CaptureTree` and `RestoreTree`. Flame therefore runs ephemeral, and the
`executor_checkpoints` table is the ephemeral substrate: one row per root tree,
keyed by `root_member_id`, holding a `payload` blob replaced in place, with
`build_id` gating restore compatibility.

The durable port instead requires every boundary to carry a complete
`TreeSnapshot` plus a `PreviousTreeDigest` and to compare-and-advance one
authoritative head:

| Method | What it guarantees |
| --- | --- |
| `ActivateTree` | fences the previous incarnation before restored work can publish |
| `CommitEffect` | keeps the Effect fact and the tree head atomic across pending, settled, and resolved boundaries |
| `CommitCheckpoint` | advances the head at a start, child, input, parked, or terminal safe cut |

Three differences matter for Flame.

**Checkpoint frequency.** `CaptureTree` requires the root to be waiting — Flame
raises "Interaction root is not waiting" otherwise — and freezes dispatch while
in-flight effects settle. So Flame can only snapshot at a wait barrier, and the
crash window is the whole interval between barriers. The durable port commits at
each effect boundary instead.

**Recoverability of an in-flight Effect.** In durable mode the pending boundary
commits before the dispatcher job starts, so recovery finds a committed record
for every Effect that could have run. In ephemeral mode no such record exists,
which is why Flame needs `tool_invocations` to answer the same question. That
table already carries the right shape, so this is duplication rather than
absence.

**Head discipline.** A digest-chained CAS head cannot be advanced by a stale
writer, and `EffectBoundary` validation cross-checks the boundary against the
process snapshot inside the very tree it commits: deployment ref, relation,
prepared step sequence, batch index, effect ID, byte-equal effect, and phase
consistency. `executor_checkpoints` has no head to compare, so its protection
against a stale writer is Flame's own admission and lease machinery rather than
the storage contract.

Adopting the port also supplies the prefix anchoring that
[the Maka comparison](./maka-runtime-comparison.md) lists as a separate gap.
`PreviousTreeDigest` is that anchor. The two items are one change.

## The capability model is latent, not bypassed

`EngineConfig.Capabilities` is unset in Flame, and Flame references no
`agent.Capability` or `agent.CapabilitySet` symbol. The kernel checks
`capabilities.Allows(effect.RequiredCapabilities())` during step preparation and
fails a denied effect with `engine.capability.denied`.

This is not a bypass. `interaction` constructs its dispatcher effects with
`NewDispatcherEffect(payload)` and declares no required capabilities, so the
check passes trivially and an empty root set denies nothing. `planning` and
`workflow` are the packages that thread capabilities through child bindings.

So the mechanism is exercised by the two strategies Flame does not use, and the
strategy Flame does use does not declare capabilities. Flame enforces authority
at its own Application layer instead. Recording this matters for one reason: if
Flame ever wants the kernel's guarantee that a child Effect cannot escalate
beyond a subset of root authority, both `interaction` and the Flame wiring have
to opt in. Neither does today.

## Conformance suites are available and unused

`agent/agenttest` ships conformance suites, including tree durability crash
conformance and signal durability conformance. Flame references the package in
zero files.

This is worth correcting independently of the port, and it is the cheapest item
here. The durable protocol has conformance coverage in Scope but no host
implementation, while Flame has a host implementation of the ephemeral path with
no Scope-supplied conformance coverage. The tested path and the used path are
currently different paths.

## Recommended priorities

1. **Upgrade to the next Scope release before writing any durability code.**
   Five of the six breaking or semantic commits since `v0.14.0` change ack
   ordering, writer identity, or the port's own shape. The upgrade also reduces
   the port from four methods to three.
2. **Implement `TreeDurability` over the current storage bundle.** The head must
   be a digest-chained row that is compared and advanced, not a blob replaced in
   place, so this replaces `executor_checkpoints` rather than extending it.
   `ProcessStartOutcomeAcknowledger` already exists and stays independent.
3. **Retire the duplicate dispatch fact.** Once effect boundaries commit
   durably, `tool_invocations` reverts to an open-attempt set and the terminal
   prune stops discarding recovery evidence. This is the same change as item 4
   in the Maka comparison, reached from the other side.
4. **Consume `agent/agenttest` conformance suites** in the Runtime integration
   suite, so the durable path Flame implements is the path Scope tests.
5. **Leave `planning`, `workflow`, and the capability model alone** until a
   product requirement asks for them. They are recorded above as facts, not
   backlog.

Items 1 and 2 are one project. Item 4 can land first and independently.

## Verification performed

Inspected in Flame: `runtime/go.mod` module pins and the absence of `replace`;
engine construction and `CaptureTree`/`RestoreTree` call sites under
`runtime/internal/adapter/agentexec`; the `executor_checkpoints` and
`tool_invocations` schema and the terminal-prune trigger under
`runtime/internal/infra/sqlite`; and a full import inventory of `Tangerg/scope`
packages across `runtime` and `cli`.

Inspected in Scope: `agent/doc.go`, `agent/engine.go`, `agent/effect.go`,
`agent/step_commit.go`, `agent/mailbox.go`, `agent/tree_durability.go`,
`agent/architecture_test.go`, and the `agenttest` suite inventory at the
worktree; plus `agent/tree_durability.go` and `agent/doc.go` at tag
`agent/v0.14.0` to confirm the port predates the current worktree.

Claims about the version delta come from `git diff --shortstat` and
`git log` over `agent/` between `agent/v0.14.0` and the Scope worktree HEAD. No
Flame or Scope code was modified, and no test or build command was run.
