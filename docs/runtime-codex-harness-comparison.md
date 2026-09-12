# Flame Runtime and the Codex Harness

Analysis date: 2026-09-08.

This document records the requested comparison of Flame Runtime with OpenAI's [Unlocking the Codex harness](https://openai.com/zh-Hans-CN/index/unlocking-the-codex-harness/) article and the local Codex source checkout. It distinguishes observed implementation behavior from recommendations. It is an analysis snapshot, not a replacement for Flame's [design philosophy](../DESIGN_PHILOSOPHY.md) or [Runtime architecture](../runtime/doc/ARCHITECTURE.md).

## Scope and evidence

The review began against the Flame worktree at HEAD `695a920b`. The Codex reference was `/Users/tangerg/Desktop/study/codex-server`, commit `d6489472f3c15e87d2d7763a5fde033545c530f8`, dated 2026-09-08. Source links identify the inspected implementations; links into either worktree may change as development continues.

The article explains the integration architecture at its publication time. The local checkout contains later changes, including a shared in-process App Server client used by TUI and exec. Article-era descriptions must not be treated as the current implementation.

The comparison covers execution ownership, delivery, lifecycle primitives, streaming, persistence, recovery, context compaction, tool presentation, and provider boundaries. It is not a complete security audit, performance benchmark, or review of every Codex subsystem.

## Assessment

Flame and current Codex have converged on the same central principle: clients consume product behavior from an authoritative runtime. Neither a transport connection nor a UI should own the durable agent lifecycle.

Flame should retain its shared delivery endpoint, Run/Segment distinction, durable Interrupts, bounded event replay, and separation between transcript history and model context. The most useful Codex lessons concern complete client contracts, concrete failure boundaries, and tests of the requests and state that real clients actually observe.

Flame's explicit durable continuation model is a meaningful design choice. It is not evidence that Flame is more mature overall. Its additional guarantees create corresponding obligations around checkpoint validation, transaction ordering, tool identity, and recovery.

## Compare equivalent responsibilities

Codex's harness includes the agent loop, tools, policies, model interaction, context management, and thread persistence. Flame intentionally delegates framework execution and provider libraries to Scope.

The useful comparison is therefore approximately:

| Responsibility | Flame | Codex reference |
| --- | --- | --- |
| Complete agent product backend | Runtime plus released Scope libraries | Core plus App Server and supporting crates |
| Client operation contract | Runtime Protocol and delivery Endpoint | App Server protocol and request processing |
| Execution engine | Scope, adapted through `agentexec` | Core session and task execution |
| Durable product semantics | Runtime Domain and Application owners | Core, ThreadManager, ThreadStore, and product extensions |
| Presentation | CLI and Desktop | TUI, IDE, desktop, and other clients |

These are responsibility mappings, not one-to-one package correspondences. A capability delegated to Scope is not missing merely because Runtime does not implement its engine.

## One semantic entrypoint across bindings

Flame's [Endpoint.Invoke](../runtime/internal/delivery/endpoint.go) centralizes invocation attachment, parameter validation, idempotency handling, capability enforcement, Application dispatch, and response/event validation. The [public Runtime binding](../runtime/runtime.go) obtains that endpoint from the same composition root used by the external binding.

The critical property is that an in-process caller does not bypass the product contract. Avoiding HTTP encoding must not introduce different admission, idempotency, error, or lifecycle semantics.

The article described a TUI that still called the Rust core directly and a planned migration toward App Server. The inspected checkout now contains [InProcessAppServerClient](/Users/tangerg/Desktop/study/codex-server/codex-rs/app-server-client/src/lib.rs), used by [TUI startup](/Users/tangerg/Desktop/study/codex-server/codex-rs/tui/src/lib.rs) and [exec](/Users/tangerg/Desktop/study/codex-server/codex-rs/exec/src/lib.rs). It preserves App Server request and event behavior over typed channels rather than exposing core execution handles to those clients.

This supports Flame's existing choice. A subprocess or stdio connection is not required to achieve semantic consistency. A future remote binding should enter the same endpoint, while process placement remains a host decision.

## Lifecycle vocabulary and recovery

| Concept | Flame | Codex reference | Consequence |
| --- | --- | --- | --- |
| Durable conversation container | Session | Thread | Broadly equivalent responsibilities |
| Logical work | Run | Turn | Similar at the UI level, but interruption semantics differ |
| Active execution interval | Segment | No direct equivalent in the inspected public Turn contract | Flame explicitly identifies each resumed interval |
| Observable content | Item | ThreadItem | Both support stable identity and structured presentation |
| Awaiting human input | Waiting Run plus durable Interrupt and checkpoint | Active Turn with pending response waiters | Reconnection and process recovery must be compared separately |
| Ongoing objective | Goal | Persisted Goal extension | Goal support is not unique to Flame |
| Plan | Session-owned persisted Plan projection | Plan tooling and notifications | Compare ownership and recovery behavior, not the presence of a checklist |

Flame's [Run state machine](../runtime/internal/domain/run/lifecycle.go) makes waiting non-terminal:

```text
One logical Run:
Segment 1 running -> waiting for input -> Segment 2 running -> completed
```

A resumed Segment does not create a second logical Run. Steering also names the expected Segment, so a stale command cannot silently target a replacement interval. See [steering](../runtime/internal/application/agent/runs/steering.go).

Codex's public [TurnStatus](/Users/tangerg/Desktop/study/codex-server/codex-rs/app-server-protocol/src/protocol/v2/turn.rs) includes `InProgress`, `Completed`, `Interrupted`, and `Failed`. Approval can leave a Turn in progress; a user interruption is a distinct terminal presentation outcome. The inspected core [TurnState](/Users/tangerg/Desktop/study/codex-server/codex-rs/core/src/state/turn.rs) stores pending approvals and input responses in process-local channels.

Two different recovery situations follow:

1. **A client disconnects while the server survives.** Codex can reconstruct a resume response from saved history and the active Turn, then replay pending approval requests to the connection. The [resume implementation](/Users/tangerg/Desktop/study/codex-server/codex-rs/app-server/src/request_processors/thread_lifecycle.rs) explicitly orders these steps.
2. **The execution process disappears.** Replaying a request is not equivalent to restoring its original execution. The inspected waiter implementation does not by itself establish durable continuation across process death.

Flame makes the latter boundary explicit. Its [Recovery use case](../runtime/internal/application/agent/runs/recovery.go) loads durable Runs, Interrupts, open invocations, transcript facts, and opaque executor checkpoints. It probes whether a waiting execution is resumable and applies an ownership-scoped recovery write set. Incompatible or indeterminate execution is not silently replayed as though its external effects were known.

This is a reason to retain Run, Segment, and durable Interrupt as separate concepts. It also limits the promise: Flame does not provide arbitrary instruction-level continuation after every crash. Recoverable waiting boundaries and unconfirmed external effects have different outcomes.

## Streaming is an observation mechanism

Flame separates several facts that are easy to conflate:

- Item identity and completed content belong to the durable transcript.
- Deltas and progress are previews.
- A live Segment journal provides a bounded reconnect window.
- A durable read restores state when the live journal cannot satisfy a cursor.

The [event contract](../runtime/protocol/events.go) distinguishes authoritativeness from replayability. The [journal](../runtime/internal/application/agent/runs/journal.go) enforces both event-count and byte limits. A slow subscriber cannot stall execution indefinitely, and an incomplete authoritative stream is not silently presented as complete.

[SubscribeRun](../runtime/internal/delivery/runs_query.go) supports replay after a retained cursor. A cursorless subscription attaches at the current head, allowing a client to read durable state and fold subsequent events without treating the subscription as full history. Invalid cursors, unavailable replay, waiting Runs, finished Runs, and stale Segments have different remedies.

Codex similarly distinguishes notifications that require delivery from those that may be dropped under pressure in its [in-process transport](/Users/tangerg/Desktop/study/codex-server/codex-rs/app-server/src/in_process.rs). Its client facade continuously drains the runtime so unread notifications do not prevent a pending request response from arriving. That facade uses an unbounded local consumer queue, while underlying command/runtime queues remain bounded.

The transferable lesson is to define independent behavior for control responses, terminal facts, and progress notifications. Flame already has a bounded replay-and-recovery model; adopting an unbounded queue would be a separate tradeoff, not an architectural upgrade.

## Persistence and ownership

Flame uses SQLite for durable product facts and recovery state. Framework objects, goroutines, live contexts, and SDK clients are not persistence models. Application owns write ordering and recovery decisions; storage encodes the resulting facts.

Codex's [ThreadStore boundary](/Users/tangerg/Desktop/study/codex-server/codex-rs/thread-store/README.md) separates raw canonical history append from explicit metadata mutation. Its local implementation uses JSONL history and SQLite metadata, including compatibility behavior for older or SQLite-less local state. [LiveThread](/Users/tangerg/Desktop/study/codex-server/codex-rs/thread-store/src/live_thread.rs) provides the active-session persistence boundary.

The relevant lesson is the ownership of history, metadata, and write ordering. JSONL is not inherently more correct than SQLite, and a stream is not automatically an event-sourcing architecture. Flame should retain its current durable owners unless a concrete requirement justifies another storage mechanism.

Codex's compatibility costs also reflect its release and integration obligations. Flame's current pre-release policy deliberately replaces invalid contracts rather than maintaining parallel formats. A future compatibility commitment should follow actual independently released consumers, not imitation of Codex's historical burden.

## Tool presentation contracts

Codex exposes explicit [ThreadItem variants](/Users/tangerg/Desktop/study/codex-server/codex-rs/app-server-protocol/src/protocol/v2/item.rs), including command execution, file changes, and MCP calls. Command items can carry working directory, process identity, output, exit status, and duration. Clients can render those facts without interpreting terminal text.

Flame uses a generic [ToolInvocation envelope](../runtime/protocol/items.go), but this does not mean every result is unstructured. Its [Presenter and PresentationContracts](../runtime/internal/adapter/toolset/presentation.go) share the same built-in descriptors. Command, patch, search, and other supported results have declared presentation shapes. The [contract generator](../runtime/cmd/contractgen/manifest.go) exports them, and [Desktop reads generated result types](../desktop/frontend/src/plugins/sdk/toolResult.ts).

The appropriate improvement is to carry this existing contract through every consumer. Known built-in results should have reliable structured presentation; unknown tools should retain their complete generic result. A new tool does not necessarily require a new top-level Item variant.

One concrete review target is the CLI's [private result decoding](../cli/internal/adapter/runtimebinding/tool_material.go). Presentation-specific extraction is legitimate, but field changes such as `exitCode` or `changes` should be caught by generated types or shared contract examples rather than silently reducing what the CLI displays.

## Context management and compaction

Both systems distinguish conversation history from the next model request. Neither should assume that the visible transcript can be sent directly to the provider.

Flame's [WorkingContextComposer](../runtime/internal/adapter/agentexec/working_context.go) composes instructions, recalled material, current Session state, and conversation input. [Context provenance](../runtime/internal/adapter/agentexec/context_provenance.go) identifies sources and separates replaceable Goal/Plan state from frozen instructions.

Its [model-context budget](../runtime/internal/adapter/run/maintenance/compaction_trigger.go) evaluates the complete imminent request, including instructions, messages, Tools, and Options. It uses provider counting when available and applicable, or calibrated estimates otherwise. The [compaction request](../runtime/internal/adapter/agentexec/model_context_compaction.go) protects the region that must survive rewriting, and the [compaction ladder](../runtime/internal/adapter/run/maintenance/compaction_ladder.go) can trim old oversized tool material before paying for summarization.

These mechanisms already address the main trigger-design lesson. Message counts and end-of-Run maintenance are not substitutes for request capacity.

Codex offers additional evidence at difficult boundaries:

- [Pre-sampling compaction](/Users/tangerg/Desktop/study/codex-server/codex-rs/core/src/session/turn.rs) handles switching to a smaller model context and has specific previous-model fallback rules.
- [WorldState](/Users/tangerg/Desktop/study/codex-server/codex-rs/core/src/context/world_state/mod.rs) maintains a model-visible comparison baseline and supports full or differential reinjection.
- [ContextManager](/Users/tangerg/Desktop/study/codex-server/codex-rs/core/src/context_manager/history.rs) distinguishes retained host facts, model history, and context baselines.
- [Compaction integration tests](/Users/tangerg/Desktop/study/codex-server/codex-rs/core/tests/suite/compact.rs) inspect actual request content across model changes, resume, and repeated compaction.

Flame should absorb these scenarios and the discipline of checking the next request. A successful summary call does not prove that the agent retained the latest user correction, current objective, relevant tool result, or retrieval reference.

Dynamic context deserves an explicit policy before more machinery is added. For AGENTS instructions, permissions, model settings, and tool catalogs, define what is frozen for the current Run, what changes at the next model call, and what changes only for the next Run. Extend the existing provenance and reducer only when a real update requirement calls for it.

Provider-native compaction is a separate optional capability. Codex's [remote compaction request](/Users/tangerg/Desktop/study/codex-server/codex-rs/core/src/compact_remote_request.rs) demonstrates integration with its provider request model. Flame should introduce an equivalent only through the appropriate released Scope/provider contract and preserve supplier-specific state at that boundary. Ordinary chat capability must not imply native compaction support.

## Goal and Plan are shared product concerns

Current Codex includes a persisted [Goal extension runtime](/Users/tangerg/Desktop/study/codex-server/codex-rs/ext/goal/src/runtime.rs), accounting, expected Goal identities, and idle continuation. Goal support therefore cannot be used as a simple differentiator against the article's smaller set of conversation primitives.

Flame's relevant distinction is explicit ownership: Goal admission and outcome attribution use the Goal incarnation associated with the Run, and Plan has a durable latest-value projection with revision semantics. These are contracts to preserve and test, not reasons to copy Codex's extension registry.

Codex's [plan handler](/Users/tangerg/Desktop/study/codex-server/codex-rs/core/src/tools/handlers/plan.rs) also makes a useful vocabulary distinction: a checklist update is not the same thing as a dedicated planning interaction mode. Flame should continue keeping product concepts separate when their invariants and user behavior differ.

## The main architectural cost to watch

Flame's most consequential boundary is between Scope execution and Runtime product state. Scope owns framework execution; Runtime owns admission, durable waiting, observable history, recovery decisions, and the projection of execution into product facts.

That boundary requires careful reconciliation. A completed tool, a suspended child, or a checkpoint may need to become several consistent product projections. If both sides independently decide whether work is complete or should resume, the system acquires competing state machines.

For each new coordination path in `agentexec`, ask whether it owns translation, Flame policy, persistence ordering, or recovery validation. If it instead recreates framework scheduling, tool execution, or private checkpoint interpretation, the missing contract belongs at the Scope boundary.

Codex's package layout does not answer that question for Flame. Preserve the responsibility split and evaluate it through actual continuation and failure behavior.

## Recommended priorities

| Priority | Work | Owner and acceptance evidence |
| --- | --- | --- |
| P1 | Run the same product scenarios through both public bindings | Runtime delivery and integration tests; compare capability rejection, idempotency, errors, event boundaries, and recovered facts |
| P1 | Carry existing tool presentation contracts through CLI and Desktop | Runtime owns result facts and schemas; consumers own rendering; contract changes must surface through types or fixtures |
| P1 | Verify task continuity after compaction | Runtime context/recovery owners; inspect the next request and add representative repeated-compaction task evaluations |
| P2 | Define when changing context sources become effective | Existing context composition and reducer; prove consistent behavior across live execution and restore |
| Demand-driven | Add native provider compaction, remote hosting, or finer execution permissions | Provider/Scope adapters and explicit host policy; implement only the required behavior and its failure boundaries |

For binding verification, reuse existing tests rather than adding another framework. Some current lifecycle tests call [delivery.Handler directly](../runtime/internal/bootstrap/protocol_lifecycle_test.go), which proves important composition behavior but does not simultaneously exercise Endpoint admission and HTTP transport. Shared scenarios can close that particular distinction while retaining one Runtime per test lifecycle.

High-value context scenarios include switching to a smaller model, large tool results, steering near compaction, resumed children contributing results before compaction, and Goal/Plan changes before the next request. Existing coverage should be reused and extended only where observable behavior remains unproven.

For execution permissions, Codex's [tool orchestrator](/Users/tangerg/Desktop/study/codex-server/codex-rs/core/src/tools/orchestrator.rs) is useful evidence for ordering approval, sandbox selection, execution, and permitted retry. Flame already has execution isolation and approval mechanisms; any expansion should preserve a single decision owner and distinguish a rejected attempt from an effect whose outcome is unknown.

## Verification performed

The preceding analysis was read-only. Selected existing Flame tests were run offline from the Runtime module:

```sh
GOWORK=off go test . ./internal/bootstrap ./internal/application/agent/runs \
  -run 'Test(RuntimeOpenCallIdempotencyStreamAndClose|ProtocolLifecycleSurvivesColdRestart|AssemblyPreservesParkedQuestionAcrossCrashLikeRestart|RuntimeCompactsDuringOneLongRunBeforeTheNextMainModelCall|ProtocolSettlesApprovalWhenEditedArgumentsCannotExecute|Journal)' \
  -count=1
```

All three selected package runs passed. The selection exercised public Go binding idempotency and shutdown, steering, waiting, resume, cold reads, crash-like recovery of a parked question, rejected edited approval arguments, compaction within one long Run, and journal behavior.

This was not the full Flame suite, a live-provider quality evaluation, or proof of arbitrary crash continuation. Codex evidence came from reading implementation and tests; its Rust test suite was not executed. Saving this document does not rerun those tests or imply that every recommendation describes a confirmed missing feature.
