# Flame design philosophy

This document explains the design choices shared by the Flame Runtime and CLI. [`AGENTS.md`](AGENTS.md) contains short rules, [`REFACTORING.md`](REFACTORING.md) defines the structural method, and [`DEVELOPMENT.md`](DEVELOPMENT.md) owns execution details. Module architecture documents own module-specific boundaries.

## Build one product model

Flame has one durable product model. Runtime owns Sessions, Runs, Segments, Items, Goals, Plans, Interrupts, provider selection, recovery, and persistence. CLI and Desktop project that model for their users; they do not create parallel execution or recovery models.

The design should make the owner of every fact obvious. If the same state can advance in Runtime Domain, SQLite records, protocol objects, and CLI state, the system has more than one product model. Keep one authoritative transition and make every other form a projection.

## Repair the owner

Two laws govern repairs:

1. Replace a known-wrong design instead of preserving it for convenience
2. Fix the layer that can produce the invalid state instead of teaching every consumer to tolerate it

A repair is complete when the invalid state can no longer be produced. Logging it, retrying it, coercing it, or adding a fallback only changes the symptom.

Breaking changes still need a blast-radius analysis. Once the scope is known, migrate every in-workspace consumer and delete the former shape in the same batch.

## Use rich models without Java ceremony

Object-oriented Go means state and behavior live on the concrete type that owns the semantics. It does not mean inheritance, base classes, getters, factories for every value, or one package per noun.

Entities and value objects own:

- construction invariants
- legal state transitions
- derived values and classifications
- normalization that changes semantic identity
- pure policies that describe that value

Data remains data when it represents configuration, a request, a response, a wire value, a storage record, or an observed fact. Decorative methods do not make a data transfer object a domain model.

Input and output mark the boundary. Domain objects do not open stores, start processes, call providers, or own cross-aggregate transactions. Application use cases coordinate those effects around behavior-rich values.

## Apply DDD and clean architecture by responsibility

Domain-driven design gives Flame a stable language and clear ownership. Clean architecture keeps product policy independent from frameworks, storage, transports, and user interfaces. Neither is a directory template.

Use these boundaries:

| Boundary | Responsibility |
| --- | --- |
| Domain | Product invariants, entities, values, aggregates, and pure policy |
| Application | Use-case ordering, cross-aggregate consistency, workflow lifecycle, and consumer-owned ports |
| Adapter | Translation between product semantics and an external framework, provider, store, or protocol |
| Infra | Reusable technical mechanisms such as SQLite, Git, process execution, and filesystem confinement |
| Delivery | Protocol validation and projection, command routing, and user-facing presentation |
| Bootstrap | Concrete construction, ownership transfer, startup, and shutdown |

Create a package when it owns a coherent vocabulary, invariant, lifecycle, translation boundary, or reusable mechanism. Keep related files together when a package split would only create more imports and forwarding APIs.

Physical hierarchy follows the context map after ownership is proven. Large rings may use one non-package namespace level to group several related packages; the namespace is navigation, not another abstraction. Keep ring-wide mechanisms and context-root aggregates direct, and do not create single-child or facade namespaces for visual symmetry.

Use domain names, not layer-role suffixes. `goal`, `session`, `workspace`, and `toolset` describe ownership. `service`, `manager`, `impl`, `common`, and `helpers` hide it.

## Discover abstractions from consumers

Start with a concrete implementation. Extract an interface at the package that consumes a replaceable capability or needs to reverse a dependency. Include only the methods that consumer invokes.

An abstraction earns its cost when it does at least one of these jobs:

- protects a stable invariant
- cuts an invalid dependency
- isolates an external type or protocol
- owns a resource lifecycle
- supports multiple real implementations
- gives a consumer a materially smaller contract

A forwarding wrapper, one-method interface beside its only implementation, generic repository, configuration facade, or mirrored state object is not neutral. It adds another concept that must remain coherent.

## Prefer one representation

One semantic value should have one internal representation and one stable name. A protocol or storage record may encode it, but the encoding does not become another semantic owner.

Use named text values for stable external vocabularies. Use numbers for counts, positions, bit masks, and quantities where numbers are the domain. Use presence or a closed union for optionality; do not make `0`, empty text, or a boolean combination mean several states.

Anonymous maps stop at open boundaries. Convert configuration, JSON, YAML, provider metadata, and tool arguments to typed values before product logic uses them.

## Keep execution and presentation separate

Runtime owns execution, durability, recovery, provider calls, and protocol semantics. CLI owns terminal navigation, draft composition, local authoring state, rendering, and process exit behavior.

Streaming previews can improve latency, but durable completed facts remain authoritative. A CLI may fold a preview into its current view; cold recovery must reconstruct the same result without that preview.

TUI polish is part of product correctness. Layout, focus, selection, keyboard behavior, mouse release semantics, resize, Unicode, and terminal restoration need explicit owners. Oolong provides terminal primitives; Flame provides product interaction.

## Treat providers as boundary adapters

Provider identity is exact and durable. A provider/model pair and model-owned options remain together across Session defaults, Run admission, Goal execution, recovery, and rendering.

The provider boundary owns credential resolution, endpoint selection, SDK construction, request mapping, response mapping, and provider-specific errors. Product code consumes provider-neutral model contracts and catalog facts. It does not switch on provider names to recreate SDK behavior.

Configuration precedence must be visible and deterministic. Missing credentials, malformed custom endpoints, unsupported capabilities, duplicate model identifiers, and catalog drift fail at admission instead of surfacing as unrelated execution failures.

## Design protocols from product semantics

Runtime Protocol methods express product operations, not internal function calls. A binding can use HTTP, streaming events, or a direct embedded call without changing business semantics.

Protocol DTOs remain serializable data. Domain and Application do not depend on them. Delivery validates them, maps them to product values, calls one use case, and maps the result or typed failure back.

Use one current wire shape. Version changes reject older shapes rather than maintaining fallback decoding during active development.

## Test the workload Flame actually has

Flame normally has one CLI and one logical Runtime. Optimize design and tests for that product, while preserving the real shared-directory and process-restart contracts already present.

High-value tests exercise complete user stories: Goal and Plan evolution, steering, human input, tool calls, long conversations, compaction, provider failure, restart, recovery, and TUI behavior. Low-value tests construct speculative client matrices, duplicate implementation tables, or prove scheduling accidents.

Concurrency mechanisms still require deterministic tests when production owns concurrent state. Use explicit synchronization and owner-visible outcomes. Do not use sleeps, stress loops, or the race detector as a substitute for a contract.

## Let references provide evidence

Scope demonstrates framework boundaries, rich Go models, one-owner APIs, and complete breaking replacement. Codex Server demonstrates protocol and execution-control mechanisms. Grok Build demonstrates terminal interaction and visual density. OpenCode demonstrates provider catalogs, credential sources, and custom endpoints.

Flame adopts an idea only after identifying the local owner and proving the product need. Similar code in a reference is not evidence that its package layout, plugin system, daemon topology, compatibility policy, or multi-client model belongs here.
