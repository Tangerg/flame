# Runtime module instructions

Flame Runtime is the Go application backend for Desktop, Web, CLI, TUI, and in-process hosts. It implements the Flame Runtime Protocol and owns durable product semantics. The root rules in [`../AGENTS.md`](../AGENTS.md), [`../DESIGN_PHILOSOPHY.md`](../DESIGN_PHILOSOPHY.md), and [`../REFACTORING.md`](../REFACTORING.md) apply here.

Before designing, implementing, reviewing, or refactoring Runtime, read these documents in order:

1. [`doc/ARCHITECTURE.md`](doc/ARCHITECTURE.md): stable target architecture and ownership boundaries
2. [`doc/DECISIONS.md`](doc/DECISIONS.md): accepted decisions and superseding rationale
3. [`doc/ENGINEERING_STANDARDS.md`](doc/ENGINEERING_STANDARDS.md): implementation and quality standards
4. [`doc/EXECUTION_PLAN.md`](doc/EXECUTION_PLAN.md): current authorization, workstreams, and next gate
5. [`doc/CAPABILITY_LEDGER.md`](doc/CAPABILITY_LEDGER.md): current capabilities, owners, verdicts, and acceptance evidence
6. [`doc/CONTRACT_BASELINE.md`](doc/CONTRACT_BASELINE.md): Protocol, storage, Agent Framework consumer, and architecture baselines

Read [`doc/DOMAIN_MODEL.md`](doc/DOMAIN_MODEL.md) for Domain behavior, Run and Transcript boundaries, Goal, Plan, or Session model changes. Read [`doc/API.md`](doc/API.md), [`doc/TRANSPORT.md`](doc/TRANSPORT.md), [`doc/AUX_API.md`](doc/AUX_API.md), and the machine truth in [`contract/`](contract/) before a protocol change.

## Runtime laws

- Runtime is a product backend, not a second Agent Framework. Agent Framework is the sole Process, tree, strategy, and effect execution owner
- Run is the product execution center. Conversation, Transcript, WorkingContext, checkpoint, and streaming observations remain distinct facts
- Domain objects own their invariants and pure transitions. Application owns use-case ordering and cross-aggregate transactions. Adapters translate external semantics. Infra owns mechanisms. Delivery projects the protocol. Bootstrap is the only composition root
- Keep the accepted dependency direction. Do not move behavior between rings for directory symmetry or to reduce a metric
- Prefer rich domain models, but keep configuration, protocol values, requests, responses, storage records, and external facts as data
- Fix wrong ownership at the source. Do not add compatibility aliases, fallback decoding, dual persistence, retry layers, or consumer-side coercion
- Do not nitpick or refactor for visual consistency. A structural change needs a reproducible product defect, dependency violation, duplicate truth, leaked abstraction, or proved ownerless surface
- Interfaces belong to consumers and contain only the operations they use. A single implementation inside one cohesive package normally remains concrete
- A package must own a real bounded context, use-case family, external translation, lifecycle, or reusable mechanism. Prefer another file in the owner package over a micro-package
- Public `protocol`, `embedded`, and `localruntime` contracts have one current shape. Regenerate and migrate all in-scope consumers with a breaking change
- Provider/model selection is exact and durable. Provider SDK types and credential rules stay behind model adapters

## Current scope

- This quality program may change Runtime, Local Runtime, CLI consumers, and their documents
- Do not edit or regenerate Desktop. The user is changing it concurrently
- The development configuration in `config/config.yaml` contains an authorized live DeepSeek key. Load it only through production configuration code; never print, copy, snapshot, or commit the credential
- Use Scope's latest published modules. The local Scope checkout is a design and source reference, not a `replace` target
- Reference Codex Server only for protocol and lifecycle mechanisms, Grok Build only for TUI behavior, and OpenCode only for provider behavior

## Testing

- Start with the smallest failing product scenario or architectural proof
- Prioritize real Runtime end-to-end coverage for Goal, Plan, steer, HITL answer/resume, long context, compaction, long-running execution, restart, and recovery
- Use bounded live DeepSeek probes when provider behavior matters; keep the default suite offline and deterministic
- Do not add multi-client or multi-server scenarios unless the changed production owner truly supports that topology
- Do not run or add race/fuzz matrices by ritual. Use targeted race tests for changed shared state, goroutine, cancellation, stream, or shutdown ownership; fuzz strict parsers and codecs only when arbitrary input is the contract
- A structural refactor needs surviving consumer behavior and architecture gates, not a new unit test for every moved helper

## Batch discipline

- Complete one semantic slice per commit. Delete the former owner in the same batch
- Update code, tests, generated contracts, storage shape, and the owning documents together
- Keep mutable progress only in `doc/EXECUTION_PLAN.md`; keep current contract facts only in their owner documents; leave command logs and per-commit diaries to Git
- Run focused tests, then affected module test, vet, build, tidy, generation, architecture, and standalone `GOWORK=off` checks in proportion to risk
- Stage explicit Runtime and CLI paths, verify no Desktop path is staged, commit, and push
