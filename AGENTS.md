# Repository instructions

Flame is a local agent product. `runtime` owns durable product semantics and exposes them through one in-process Go binding and one Runtime Protocol. `cli` and `desktop` are consumers that own command, terminal, and graphical presentation. Scope supplies released framework and provider libraries; Flame does not rebuild them.

Stable design rationale lives in [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md). Structural changes follow [`REFACTORING.md`](REFACTORING.md). Repository workflow, verification, active scope, and reference-project rules live in [`DEVELOPMENT.md`](DEVELOPMENT.md). Read the nearest module `AGENTS.md` before changing that module.

- Do not preserve backward compatibility for a wrong design. Fix the semantic owner, migrate every in-scope consumer, and remove obsolete APIs, packages, schemas, aliases, fallbacks, tests, and documentation in the same batch.
- Apply Occam's razor. Prefer the smallest complete design that explains all proven requirements; every abstraction, representation, state, dependency, package, and call path must justify its existence.
- Give each fact one owner, one representation, and one primary call path. Projections may encode or cache an owner-provided fact but never advance it independently.
- Runtime is the sole authority for Session, Run, Segment, Item, Goal, Plan, Interrupt, execution, persistence, recovery, provider/model selection, and compaction. CLI and Desktop do not rebuild those state machines.
- The Runtime Go binding and Runtime Protocol are two projections of one semantic core. Both enter the same delivery endpoint before capability checks, idempotency, lifecycle control, Application invocation, and error or event projection.
- Use domain-driven design and clean dependency direction to express ownership, not to generate a directory matrix. Domain models own invariants and pure transitions; Application owns use-case ordering; external adapters own translation; delivery owns bindings; bootstrap owns composition and shutdown.
- Prefer explicit, readable, shallow, sparse Go. A package must own a coherent vocabulary, aggregate invariant, use-case lifecycle, external translation, or reusable technical mechanism. Prefer several responsibility-named files in one package over a package per concept. When a ring has several proven contexts, use one non-package namespace level to expose that context map; reserve direct ring packages for ring-wide mechanisms or an aggregate that names its context.
- Namespace directories never contain facade Go files. Do not add a single-child namespace, nest beyond `ring/context/package`, or move a cross-context mechanism under one consumer merely to make the tree symmetrical.
- Do not create `service`, `repository`, `manager`, `impl`, `common`, `helpers`, or umbrella packages to make a diagram look complete. A forwarding wrapper or cycle-avoidance package is a refactoring signal, not an architectural boundary.
- Start with concrete types. Define narrow interfaces in consuming packages only when they reverse a dependency, isolate an external boundary, or support real substitution. Return concrete types from constructors.
- Keep composition explicit. Do not use ambient globals, service locators, reflection registries, dependency-injection containers, optional service bags, or configuration objects that leak across boundaries.
- Make invalid states unconstructable where practical. Use closed states and named values instead of primitive sentinels, boolean combinations, magic strings, anonymous maps, or typed-nil ambiguity.
- Preserve one vocabulary across domain types, protocol values, storage, errors, CLI output, tests, and documentation. Do not retain former names as aliases.
- Provider identity is the exact provider/model pair plus model-owned options. Credential precedence, endpoints, SDK construction, and provider-specific translation remain at the provider boundary.
- Prefer the current Go standard library, then an existing mature dependency. A wrapper must own policy, translation, lifecycle, or authority; otherwise delete it.
- Do not guess where performance matters. Measure first, fix the data model before adding machinery, and optimize only a demonstrated dominant bottleneck.
- Test observable contracts, owner invariants, dependency direction, and real product lifecycles. Do not freeze file paths, private fields, package counts, implementation inventories, or speculative topologies.
- Preserve unrelated user changes. Never edit, format, stage, revert, or generate into a user-owned path outside the active scope.
- Reply to the user in Chinese. Keep code, identifiers, comments, errors, and repository documentation in English. Comments explain why, not what.
