# CLI module instructions

Flame CLI owns command routing, process behavior, terminal interaction, rendering, and CLI-local authoring state. It consumes Runtime; it is not a second Runtime.

Read [`../AGENTS.md`](../AGENTS.md), [`../DEVELOPMENT.md`](../DEVELOPMENT.md), and [`ARCHITECTURE.md`](ARCHITECTURE.md) before changing this module.

- Runtime remains authoritative for Session, Run, Segment, Item, Goal, Plan, Interrupt, provider/model selection, execution, persistence, compaction, and recovery. Do not mirror their states, validation, errors, feature catalogs, or lifecycle transitions in CLI-owned types.
- Consume the public Runtime Go binding through narrow interfaces defined by CLI consumers. Use Runtime Protocol values at that boundary instead of translating them into a synonymous CLI data model.
- CLI-owned models are limited to presentation and interaction concerns such as Conversation folding, selection, drafts, prompt history, queue intent, stash, rendering, and terminal focus.
- `main.go` opens at most one concrete Runtime, owns signals and streams, and closes it once. Do not pass a service bag or service locator through commands, workflows, or terminal state.
- Cobra and Viper stay in `cmd` and the process composition path. Commands construct fresh trees, parse typed input, call one CLI use case with `cmd.Context()`, and write through Cobra streams.
- Oolong and terminal protocols stay in terminal delivery. Rendering reads state; it does not repair or advance Runtime state.
- Organize packages around cohesive CLI workflows and durable local owners. Prefer several files in `run`, `session`, `command`, `conversation`, or `terminal` over packages for each action or interface.
- Production code has one Runtime path. Keep fakes beside tests or in test-only support; do not ship a mock Runtime selected by environment configuration.
- Runtime events are observations. Completed Runtime Items and authoritative snapshots win after gaps, reconnects, and cold recovery.
- Preserve exact provider/model identity and model-owned options. Never infer a provider from a model name or retain credentials in history, frames, errors, or logs.
- Test user-visible one-shot and terminal flows. Use in-memory root-command tests for routing and a real PTY only when terminal escape sequences, resize, focus, input decoding, or restoration are the contract.
- The current `backend`, `runtimeembedded`, capability-service, and one-action package tree is migration debt. Do not add new consumers or abstractions to it while it is being removed.
