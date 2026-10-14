# Flame design philosophy

Flame is one product with one durable model. Runtime owns product semantics and exposes them through an in-process Go binding and the Runtime Protocol. CLI and Desktop/Web own presentation and interaction. Scope owns the agent framework and provider libraries.

## Why Runtime owns the durable facts

[`AGENTS.md`](AGENTS.md) states the general rule, and [`PROJECT_RULES.md`](PROJECT_RULES.md) names the exact facts Runtime owns. What that ownership buys is the reason to keep it.

A product with in-process, terminal, graphical, and editor consumers can give each surface its own copy of a Run's state and reconcile them later. Flame does not, because reconciliation is where the cost lands: every added surface multiplies the pairs that can disagree, and a crash during disagreement leaves no representation that can be trusted to say what happened.

Concentrating the transitions in Runtime moves that cost once, into one state machine that persistence and recovery already have to be correct about. The surfaces keep only what they can rebuild by reading: selection, focus, drafts, rendering. Losing a surface then costs nothing a restart cannot restore.

## Repair the semantic source

A repair is complete only when the invalid state can no longer be produced. Fix the owner that admits or creates the state, migrate every in-scope consumer, and remove the former API, schema, fallback, test, and documentation in the same change.

Breaking changes are preferred to compatibility around a wrong design. This does not permit arbitrary churn: the new design must reduce the number of owners, representations, states, dependencies, or call paths.

## DDD without ceremony

Domain models own identity, invariants, legal transitions, and pure policy. Application code owns use-case ordering, transactions, cancellation, and cross-aggregate consistency. Adapters translate external systems. Delivery translates bindings and user interaction. Bootstrap constructs and shuts down the concrete graph.

These boundaries express responsibility, not a required directory matrix. A package exists only when it owns a coherent vocabulary, aggregate invariant, workflow lifecycle, external translation, or reusable technical mechanism. Related responsibilities remain as named files in one package when splitting them would add only imports and forwarding APIs.

Interfaces belong to consumers. Start with concrete types and extract the smallest interface only to reverse a dependency, isolate an external boundary, or support proven substitution.

## Behavior belongs with the fact

Prefer a behavior-rich aggregate or value object when a fact has identity, legal states, validation, or transitions. The owner validates construction, keeps mutation private, answers domain questions, and performs pure transitions through intention-revealing methods. Application code asks the owner to decide; it does not reproduce the same rules with conditionals over exported fields.

Rich does not mean that every struct needs methods. Protocol payloads, persistence records, provider requests and responses, configuration inputs, and rendering projections are data at their boundaries. They should be strict and typed, but they do not acquire fake domain behavior. I/O, clocks, cancellation, retries, transactions, and cross-aggregate ordering stay outside Domain objects.

Encapsulation is successful when invalid intermediate states disappear and callers need fewer facts to make a decision. It is not successful when getters merely hide fields, a generic manager forwards every method, or one aggregate absorbs unrelated workflows.

## One execution path

The Go binding and Runtime Protocol enter the same delivery endpoint before capability checks, idempotency, lifecycle control, Application invocation, and result projection. Runtime does not duplicate Scope's process loop, tool loop, scheduler, provider behavior, or checkpoint interpretation.

CLI commands and the terminal client consume Runtime through narrow ports. They may own drafts, selection, prompt history, queue intent, focus, rendering, and other local interaction state. They do not reconstruct durable Runtime state machines.

## Explicit composition

Construction, ownership transfer, cancellation, joining, and shutdown are explicit. Flame does not use ambient globals, service locators, dependency-injection containers, reflection registries, optional service bags, or parallel lifecycle roots.

Every goroutine and resource has one owner, a stop condition, and a join or close path. Durable state is the recovery source; process-local notifications only wake consumers so they can reread it.

Installing or explicitly selecting plugin release bytes is the trust decision.
The installation owns selected content and desired enablement; backend and client
availability are observations. Enabling the same selected code restores availability
without replacing an execution dependency. Release or configuration replacement still
requires the owning work to be quiescent. Installing code does not create standing
model-tool approval: those decisions retain their source-qualified policy owner.

Backend method bindings are a public transport boundary used by every author.
The feature owns structural types, business refusals and private mutation receipts.
An unrecognized storage, commit or transport failure is unconfirmed; the binding
must not turn it into a definite business refusal. For a command without a Run,
Runtime must durably capture the original provider and request binding before
dispatch, derive retention and quiescence from unresolved bindings, and discharge
that dependency only after observing the original owner's valid response. This
routing evidence does not become another business result or receipt store. Existing
Memory/Schedules commands now use this boundary. Once dispatch has been admitted,
receipt lookup is the only reconciliation path; absence cannot authorize another
mutation. Retained identities do not expire into calls on a current provider.
Generic operation descriptors, origin/parent propagation and public installed
proxies still require the same guarantee when they replace the closed services.

Request identity retains one admitted raw JSON value before typed conversion.
The shared pure jsoninput value normalizes object spelling and order while
preserving numeric lexemes. Host routing and private receipts persist that value;
their digests are projections. Re-encoding a DTO cannot replace original evidence.
An incompatible digest-only directory is refused, retained separately and replaced
with a fresh directory rather than silently interpreted under a new algorithm.

Feature authoring keeps Go-owned types and method descriptions authoritative.
Scope derives and compiles their structural schemas; the same immutable contract
admits the backend wire call and publishes build/discovery descriptors. Runtime's
existing generator projects those standard schemas into TypeScript and operation
documentation. Generated files do not become another registration authority, and
shape equivalence never selects a provider or proves behavioral substitutability.

## Evidence over symmetry

Architecture is judged by real consumers, invariants, failure ordering, protocol behavior, persistence, and lifecycle tests. Package counts, file counts, private field inventories, and symmetric trees are not design evidence.

Use the current module Go version and standard library first, then existing mature dependencies. Keep a wrapper only when it owns policy, translation, lifecycle, or authority. Measure before optimizing.

Model-visible media support is admitted at the complete reduced/transformed
request boundary. Command admission owns structurally valid source input, target,
identity and original transcript evidence; it cannot decide whether a later model
projection has an unsupported modality. Start/Resume/Steer acceptance therefore does
not claim model-media success. The selected provider sees only the admitted final
projection, and failure remains the original execution owner's observable outcome.

Independent plugin model exchanges carry their own caller identity, exact selection
proposal, complete Scope request and explicit request/output budget. They enter the
same authenticated callback endpoint as canonical Runtime RPC. The existing observed
provider resolver and physical journal own dispatch fencing, known outcomes, usage,
price and restart uncertainty; a callback acknowledgement is not a second receipt or
permission to replay. No Memory call-count rule applies to another plugin's workflow.

A captured continuation still refers to the remainder of its original frozen chain.
A fresh selected exchange is a different capability and cannot reinterpret that
continuation as another provider call. Each fresh exchange resolves its installed
selection, freezes context/input/observer bindings and admits the complete final
request. Scope remains the durable effect and execution owner. General remote
composition and Run/Goal aggregation of descendant model consumption require their
own explicit execution contracts; physical evidence alone does not complete them.

A physical model exchange obtains its execution association from the host-captured,
admitted logical Model or Tool call. Primary unary/stream and Tool boundaries seed
that immutable context; nested callback dispatch preserves it. The journal freezes
Session/Run/Segment from the original logical record only while its Run and Segment
are active. Wire IDs, parent chains and an ambient current Session cannot choose or
retarget that association. Unattached/Session-only work explicitly has no Run owner.

Fresh model identities cannot claim existing logical Model/Tool IDs or their calling
operation's identity. A genuine captured Model continuation uses its original Model
owner; independently selected exchanges require distinct identities. Waiting or a
changed active Segment refuses new generation under the old captured owner. Known
physical settlement and recovery still retain the original evidence. The association
is a prerequisite for descendant accounting, not proof that aggregate budgets close.
