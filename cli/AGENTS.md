# CLI module instructions

The CLI is Flame's command-line and terminal presentation module. It consumes the public embedded Runtime and owns only command routing, process behavior, terminal interaction, rendering, and CLI-local authoring state.

Read [`../AGENTS.md`](../AGENTS.md), [`../DESIGN_PHILOSOPHY.md`](../DESIGN_PHILOSOPHY.md), [`../REFACTORING.md`](../REFACTORING.md), and [`ARCHITECTURE.md`](ARCHITECTURE.md) before changing this module.

## Boundaries

- Runtime remains authoritative for Session, Run, Segment, Item, Goal, Plan, Interrupt, provider/model selection, tools, persistence, compaction, and recovery
- `internal/runtimeembedded` is the only production anti-corruption boundary for `runtime/embedded` and `runtime/protocol`; no other CLI package imports Runtime Protocol types
- Cobra and Viper stay in `internal/cmd` and the root composition path. Oolong stays in terminal and rendering adapters. Domain and application packages import neither
- `main.go` constructs one process graph, owns signals and streams, and closes the embedded Runtime. It contains no product policy
- CLI-local durable state contains drafts, prompt history, queue, stash, recent workspace, and command replay facts. It does not mirror Runtime durable state

## Package discipline

- The current `internal` tree is an audit target, not a template. Merge a micro-package when it only forwards one concept to one adjacent owner and protects no independent invariant or boundary
- Do not rewrite clear working code to satisfy a preferred style. Require a real consumer, defect, leaked boundary, duplicate truth, or proved navigation cost
- Prefer multiple responsibility-named files in an existing cohesive package before creating a package
- Do not replace many micro-packages with one god `backend`, `service`, `manager`, `common`, or `utils` package
- Keep behavior on rich CLI values such as selection, queue entry, replay guard, run projection, or interaction state. Do not move I/O onto those values
- Commands build fresh Cobra trees. Avoid package globals, `init` registration, direct `fmt.Printf`, and business logic in `RunE`
- Parse flags and environment into typed CLI values once. Do not pass Viper, Cobra commands, primitive sentinel bags, or protocol DTOs into use cases

## TUI contract

- Use `/Users/tangerg/Desktop/grok-build` as the primary layout and interaction reference. Copy no Rust architecture or daemon assumptions
- Match information hierarchy, spacing, focus, composer behavior, Goal/Plan visibility, streaming stability, and key/mouse semantics before adding Flame-specific decoration
- Oolong owns terminal primitives and editing mechanics. Fix or upgrade Oolong instead of forking a local widget or escape-sequence implementation
- Every interaction has one state owner. Rendering reads state; it does not repair or advance product state
- A mouse action commits only when press and release identify the same control. Dragging text must not trigger actions
- Resize, Unicode, paste, focus, alternate-screen, cursor, mouse mode, and terminal restoration are product contracts. Test terminal protocols with a real PTY

## Provider and Runtime behavior

- Preserve exact provider/model identity and model-owned options. Never infer a provider from a model ID
- Configuration displays mask credentials and never retain secrets in history, frame snapshots, errors, or logs
- Use Runtime discovery and generated contracts as facts. Do not maintain a handwritten method, feature, event, model-capability, or error catalog beside them
- Treat stream events as observations and completed Runtime Items as authoritative. Cold reads and reconnects must converge without relying on preview frames
- Bind run start, resume, replay, and steer to exact identities. Do not repair stale identity with a new read-and-retry loop unless the product command explicitly allows it

## Testing

- Prioritize root-command tests, real embedded Runtime scenarios, and PTY interaction over unit tests for each micro-package
- Cover Goal, Plan, steer, HITL resume, context compaction, long output, reconnect, cancellation, and provider failures through user-visible flows
- Do not create multi-CLI or multi-Runtime tests unless a changed shared-directory contract requires them
- Run `go test`, `go vet`, `go build`, `go mod tidy`, architecture checks, and `git diff --check` for every batch. Run targeted race tests only for changed concurrent ownership
- Use `GOWORK=off` for a standalone module pass before committing dependency changes

Keep new documentation concise and current. Git owns completed batch logs; architecture owns stable boundaries.
