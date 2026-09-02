# Runtime module instructions

Flame Runtime is the product backend and sole owner of durable agent semantics. It exposes one in-process Go binding and one Runtime Protocol for CLI, Desktop, and other hosts.

Read [`../AGENTS.md`](../AGENTS.md), [`../DEVELOPMENT.md`](../DEVELOPMENT.md), and the ordered Runtime baseline in [`doc/README.md`](doc/README.md) before changing this module.

- Runtime is a product backend, not another Agent Framework. Scope owns Process, strategy, tree, provider, and effect execution contracts; Runtime adapts them to Flame product semantics.
- The module root owns the public in-process `Runtime` lifecycle and typed operation methods. `protocol` owns public request, response, event, error, version, and validation values. Do not define synonymous public models or a forwarding binding package.
- The Go binding and HTTP/JSON-RPC binding must enter the same delivery endpoint before capability admission, idempotency, invocation lifetime, Application use cases, and event or error projection.
- Domain packages own behavior-rich aggregates, value objects, invariants, and pure transitions. Aggregates validate construction, hide mutation, and expose legal domain operations; protocol, storage, configuration, provider, and projection values remain strict data. Application packages own cohesive use-case families and cross-aggregate ordering. Neither depends on protocol, storage, provider SDKs, or delivery types.
- Keep a Domain or Application package only when it protects a real aggregate vocabulary, invariant, or workflow lifecycle. Merge helper, identity, reference, and one-concept packages into their proven owner; do not replace them with one god package.
- Organize each large ring as `ring/context/package`. Context directories are non-package namespaces backed by multiple related packages; direct ring packages are limited to ring-wide mechanisms or aggregate packages that also name their context. Do not create facade parents, single-child namespaces, or deeper decorative nesting.
- Group external code by the system it translates: provider, MCP, SQLite, filesystem, Git, execution, LSP, or transport. Delete adapter-to-infra forwarding layers that own no policy or lifecycle.
- Bootstrap is the only concrete composition and shutdown owner. It constructs one delivery endpoint and one resource graph; do not maintain parallel Assembly, Host, Instance, Server, or optional-service lifecycles.
- Interfaces belong to consuming Application or delivery code and contain only invoked operations. Keep a cohesive single implementation concrete.
- Run is the product execution center. Conversation, Transcript, WorkingContext, checkpoint, stream observation, durable Item, and recovery remain distinct facts with explicit owners.
- Provider/model selection is exact and durable. Provider SDK types, credentials, endpoints, request lowering, and provider-specific errors stay behind provider adapters. Optional provider capabilities use separate narrow consumer contracts; never inflate the ordinary chat client or approximate an unavailable capability in Runtime.
- Protocol operations express product behavior, not internal functions. Catalog metadata is machine truth; generated contracts and typed Go methods are checked projections of it.
- Public API, wire, storage, and generated-contract changes are replaced completely. Migrate all in-scope consumers and delete the former shape without aliases, fallback decoding, dual persistence, or compatibility packages.
- Prefer real single-Runtime end-to-end scenarios for Goal, Plan, steer, HITL, interruption, compaction, long context, long execution, provider failure, restart, and recovery. Test concurrency only where Runtime owns concurrent lifecycle; do not invent multi-client or multi-server scenarios without a product obligation.
- Decide compaction only at an imminent model call from that complete request's token footprint. Do not trigger it from protocol message counts or Run completion. Pre-release SQLite installs the current schema directly; do not add hand-maintained epochs or a migration graph without an explicit migration requirement.
