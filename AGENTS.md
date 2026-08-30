# Repository instructions

Flame is a Go workspace for a local agent product. `runtime` owns durable product semantics and the Runtime Protocol. `cli` owns the command surface and terminal experience. `desktop` owns the Wails and React client. Scope provides released framework and provider libraries; Flame must not duplicate those libraries inside the product.

Read [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md) before changing architecture and [`REFACTORING.md`](REFACTORING.md) before a structural refactor. Read the nearest module `AGENTS.md` before editing that module.

## Current work boundary

- Change only `cli`, `runtime`, `runtime/localruntime`, and repository documents required by those modules
- Treat every `desktop` change as user-owned concurrent work: do not edit, format, stage, revert, generate into, or include it in a commit
- Use `/Users/tangerg/Desktop/flame/runtime/config/config.yaml` for authorized live DeepSeek verification; never print or copy its credential into logs, fixtures, diffs, or documentation
- If the scheduled build-cache cleanup removes artifacts, rebuild them and continue; do not investigate the cleanup as a product failure

## Design laws

- Fix the cause at its semantic owner. A consumer-side condition, retry, coercion, fallback, or compatibility wrapper that leaves the invalid state producible is not a fix
- Do not refactor clear working code for stylistic uniformity. Require a real consumer, reproducible defect, boundary violation, or measurable maintenance cost before changing structure
- Breaking changes are allowed. Replace a wrong API, wire shape, persisted shape, package, or name completely; remove old callers, codecs, aliases, fallbacks, tests, and docs in the same batch
- Give each fact one owner, one representation, and one primary call path. Derived projections may cache or serialize an owner-provided value, but they do not advance it independently
- Prefer object-oriented, behavior-rich domain models: entities and value objects own invariants, validation, derived values, and pure state transitions. Configuration, requests, responses, wire values, and persisted records remain data models
- Use domain-driven design and clean dependency direction to express real ownership. Do not create `service`, `repository`, `manager`, `impl`, or one-concept packages for visual symmetry
- Prefer several responsibility-named files in one cohesive package before creating another package. A package must own a vocabulary, invariant, lifecycle, replaceable boundary, external translation, or reusable technical mechanism
- Define interfaces at the consumer. Keep them narrow. Use a concrete type when one cohesive implementation has no substitution or dependency-boundary need
- Keep composition roots explicit. Do not introduce ambient globals, service locators, reflection registries, dependency-injection containers, or configuration bags that leak across layers
- Make invalid states unconstructable where practical. Use named value types and closed states instead of primitive sentinels, boolean combinations, magic strings, or maps
- Prefer the standard library, then an existing mature dependency. A wrapper must own policy, translation, lifecycle, or authority; a forwarding wrapper is deletion material
- Preserve one vocabulary across Go types, errors, storage, protocol, CLI output, tests, and docs. Do not keep old names as aliases

## Module boundaries

- Runtime Domain owns pure product rules. Application owns use-case ordering, cross-aggregate transactions, and process-local workflow lifetimes. Adapters translate external frameworks and providers. Infra owns technical mechanisms. Delivery projects the protocol. Bootstrap is the only composition root
- CLI commands parse arguments, flags, and configuration, then call CLI-owned use cases. Cobra, Viper, Oolong, Runtime Protocol DTOs, and provider SDK types must not leak into the CLI domain
- Runtime is the authority for Session, Run, Segment, Item, Goal, Plan, Interrupt, execution, persistence, recovery, provider/model selection, and compaction. CLI never rebuilds those state machines
- Provider identity is always the exact provider/model pair plus any model-owned options. Do not infer a provider from a model name or merge provider SDK semantics into the domain
- `runtime/contract` is the Runtime Protocol machine truth. HTTP and embedded bindings use the same operation and Application path. Do not add a second business path for a binding

## Reference projects

Reference projects supply evidence, not dependencies or structures to copy:

- `/Users/tangerg/Desktop/scope`: repository rules, rich domain models, released framework/provider contracts, and refactoring discipline
- `/Users/tangerg/Desktop/study/codex-server`: protocol lifecycle, turn control, interruption, steering, compaction, and recovery mechanisms
- `/Users/tangerg/Desktop/grok-build`: CLI/TUI layout, interaction density, Goal/Plan surfaces, composer behavior, and terminal details
- `/Users/tangerg/Desktop/opencode`: provider discovery, exact model identity, credential precedence, custom endpoints, and capability projection

Record what evidence a reference supplies and which Flame invariant owns the final design. Never copy a multi-client, cloud, daemon, plugin, or compatibility architecture that Flame does not need.

## Testing

- Test observable contracts and architecture boundaries, not implementation trivia
- Prioritize real end-to-end scenarios: Goal start/continue/stop, Plan replacement, steer, HITL answer and resume, context compaction, long runs, long context, restart and recovery, provider/tool failure, CLI rendering, and terminal input
- Use the real DeepSeek configuration for targeted live tests. Keep live tests explicit and bounded so the ordinary offline suite remains deterministic
- Assume one logical Runtime and one CLI actor. Do not add multi-client, multi-server, contention, or race suites unless a changed production boundary genuinely owns concurrent state
- Run race tests only when a batch changes goroutine ownership, shared mutable state, cancellation, streams, or resource shutdown. Do not use race coverage as a ritual gate for unrelated work
- Use a real pseudo-terminal when terminal escape sequences, resize, input decoding, focus, or mode restoration are the contract. Use in-memory tests for ordinary application state and rendering
- A regression test must fail when the protected behavior is removed. Do not add count-only, non-empty, duplicated fixture, or mock-heavy tests that cannot distinguish the bug

## Work discipline

- Inspect `git status` before each batch and preserve unrelated work
- Work in one ownership boundary per commit. Keep a behavioral fix and an unrelated cleanup separate unless the cleanup removes the root cause
- Start structural work from caller, owner, dynamic-entrypoint, persistence, and protocol evidence. Directory count and static-tool output only produce candidates
- Delete an obsolete contract end to end. Search again for its symbols, serialized names, config keys, docs, and generated artifacts
- Update the owning document when a stable rule or current contract changes. Do not append command logs or per-commit diaries to architecture documents; Git owns that history
- Stage explicit paths. Before committing, inspect the staged diff and verify it contains no `desktop` path
- Run the narrow decisive test first, then the affected module's test, vet, build, tidy, generation, and architecture gates in proportion to risk
- Keep every commit independently revertible. Push each verified batch; never force-push or rewrite user commits

Reply to the user in Chinese. Keep code, identifiers, comments, errors, and new repository documentation in English unless an existing document deliberately uses Chinese.
