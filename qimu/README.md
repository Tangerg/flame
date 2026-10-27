# Qimu

Qimu is a general-purpose plugin framework with a Go Host, typed native contracts,
portable process contracts, and a TypeScript browser execution runtime. A consumer
defines its services, contribution Points, Events, selection rules, permissions and
business records. Qimu owns installation admission, exact implementation bindings,
execution lifetimes and the evidence required to release their protections.

The Go module is `github.com/Tangerg/flame/qimu`. It builds independently of Flame's
Runtime and SDK modules. Flame is a consumer of these public APIs. Qimu does not
require Dougong. The portable schema and MCP adapters use the dependencies declared
in this module's `go.mod`.

## Contracts and ownership

`Contract[T]` declares a service value, `Point[M, V]` declares metadata and a
contribution value, and `Event[T]` declares a public event payload. Each declaration
has an exact name, version, kind, execution mode and descriptor digest. Register
declarations through `Host.Register` before installing Units that reference them.
The same language type or display name does not authorize substituting another
contract.

Use `NewContract`, `NewPoint` and `NewEvent` for native values. Their concrete Go
types are checked at the owning Host. Native JavaScript values remain in their
original Realm. Neither path implicitly serializes functions, DOM nodes or other
objects. Cross-process values require an explicit portable descriptor, codec and
driver. `BindContract`, `BindPoint` and `BindEvent` attach typed projections to a
published descriptor; `NewPortablePoint` keeps the Point's metadata schema separate
from the service contract of its value.

`portable.ProfileLimits()` publishes the fixed descriptor and validation profile;
the Node SDK's `PORTABLE_LIMITS` is generated from that same source. Schema
compilation remains in Scope. Portable admission checks bytes, depth, value nodes,
method count, numeric literal/exponent size and local reference expansion first.
`portable.AdmitSchema` applies the same boundary to Point metadata. Validation
checks instance depth, a separate payload node budget and a conservative schema
traversal bound before invoking the existing engine. Payload byte limits still
belong to the transport or calling application. Local structural recursion is
supported; non-consuming reference cycles, external schema resources and patterns
with author-controlled backtracking are rejected. See the
[Node profile](node/README.md) for the shared authoring constraints.

A `Definition` contains an immutable release revision and its independent Units.
Each `UnitDeclaration` declares named import slots, exports and an execution
profile. `Define` validates that declaration. `Host.Install`, `Update`, `SetEnabled`
and `Uninstall` are the installation transition authority. A configured
`InstallationStore` persists those admitted transitions; it does not make its own
selection or installation decision.

| Fact                                                                     | Owner                                               |
| ------------------------------------------------------------------------ | --------------------------------------------------- |
| Contract meaning, ordering, selection policy and business result         | Consumer                                            |
| Installed release, configuration and enabled state                       | Qimu Host, persisted through its installation store |
| Public contribution membership and exact binding admission               | Qimu Host                                           |
| Actual process, JavaScript object or native resource                     | Its execution driver and original Lifetime          |
| Queued work, document revision, application receipt and retained binding | Original consumer business owner                    |
| Stable transaction conclusion and execution-protection record            | SQLite adapter when that profile is used            |

## Select once, use the admitted binding

For an explicit service graph, `Host.Plan`, `PlanWith` and `PlanUnit` construct a
`BindingPlan`. `PlanWith(ctx, realm, choices, roots...)` accepts the consumer's
explicit choices and execution Realm; `PlanUnit` supports an entry with no public
export. Each import slot has its own contract and exact provider set. The plan
includes transitive setup dependencies, configuration identities and the candidate
reads behind unresolved slots, including optional absence. `ActivatePlan` and
`Acquire` validate those original reads under admission. Slow setup retains its
premises until activation and publication are approved.

Once an instance is ready, its actual setup imports are fixed. A later optional
provider does not change an accepted absence. An explicit retirement and fresh
plan are required to adopt another binding. Imports with providers fixed in their
declaration do not read the unrelated candidate set.

For contribution Points, the public workflow is:

1. Obtain one `Host.Snapshot` for all Points used in a decision.
2. Read typed `Candidates` and apply consumer policy to their `Metadata`, `Source`
   and stable candidate identity.
3. Call `Select`, `SelectOne` or `SelectMany` with an explicit consumer rule name.
4. `Capture` those selections together into one `BindingPlan`.
5. Open an admission scope, then call `WithPlan` or `Host.Acquire` and `Borrow` the
   selected values within the resulting `Use`.

Selection never grants use by itself. Acquisition validates the original snapshot,
scope generation and exact members under the shared admission gate. An unrelated
consumer cannot fabricate a candidate from wire metadata. A subsequent withdrawal
blocks new acquisitions while an already admitted Use retains its original value.
`Watch` projects membership changes; it does not become a second contribution
directory.

`BindingPlan.Snapshot` returns a data projection only when its selected members
have durable identities. It does not approve a fresh plan or prove a business
commit. Within the original business owner's admission and transaction,
`ValidatePlanUnder` checks the complete read set and returns the exact snapshot
to save with that record. `ActivateBinding` and `AcquireBinding` are trusted
restoration ports for an already accepted binding; passing a fresh plan's JSON
through them would bypass its preparation contract. Restoration never reruns
provider selection, including for an accepted absence that had not yet started
execution. A changed release, configuration or unavailable original dependency
is an explicit failure. An instance cannot simultaneously have two different
setup bindings.

`BorrowExport` supports an exact typed export, and `BorrowInstance` enters the
approved Unit's original work lifetime even when the Unit has no exports. The
caller must not retain an unprotected value after the borrow. `AcquireChild`
admits a child Use under an existing parent; parent closure and child admission
share the same gate.

## Drivers and resource lifetimes

The first-party profile names are `native-go`, `mcp-process`, `browser-js` and
`static-resource`. A driver implements `Profile` and `Prepare`; a native Unit can
instead supply its `SetupFunc`. Drivers receive an approved `Setup`, including
the exact Unit binding, complete snapshot, configuration, Realm, epoch and scope.
They do not perform another global lookup.

Use `Import` or `OptionalImport` for typed approved imports, `Provide` for a
service, and `Contribute` for a Point value. Explicit same-Unit local references
remain part of the frozen binding but require the driver to resolve its own
collaborators; they are not represented as usable nil values. Native import
helpers refuse such unresolved local references.

Preparation registers resources with `Setup.Own` or `Cleanup`. An `OnActivate`
callback starts work only after final Host admission. Failed preparation or stale
publication closes the same original resources. Runtime publication uses
`Host.Publish` with an immutable operation identity and an already declared Point
contract; `Publication` observes the original operation, and `Withdraw` removes
membership without recreating or prematurely freeing its object.

`Lifetime` owns admitted work and resource dependencies. Enter work before
starting it and finish that original work when it ends. Closing first seals new
work and broadcasts cancellation, then joins admitted work and releases dependents
before their providers. Resource cleanup runs once. A pending close remains
unconfirmed until the original owner supplies actual release evidence through
`ConfirmReleased`; cleanup is not replayed to manufacture certainty.

`CloseOutcome` distinguishes release from diagnostics. Cancelling a caller's wait
does not cancel the original exit or release its protections. `Use.Close` waits for
its original resource exit, durable protection release and Host reference removal.
`RetireInstance` and `WaitInstanceRetired` address an exact instance; installation
retirement is a separate, broader operation. Public Event routes are issued and
validated by the Host against their original accepted Uses.

`Host.Observations` projects each live instance's preparation failure and original
`CloseOutcome`, including its Close identity, pending resources and bounded cleanup
diagnostics. The returned report is detached from the mutable exit owner. Released
instances leave the live observations; keeping a report does not retain their
resources. `Admission.Scope` reads the original scope generation and fence without
reopening it, including while recovery blocks new admission. Recovery itself never
reopens a fenced generation; only an explicit `OpenScope` creates a new grant.

`HostConfig.MaxUses` and `MaxWatches` bound simultaneous admitted Uses and watches;
their defaults are 4096 and 1024. `WatchCapacity` separately bounds each watch's
revision queue. `NewLifetimeWithLimits` configures `LifetimeLimits.MaxWork` and
`MaxResources`; zero fields use 4096, and `NewLifetime` uses that same constructor
with defaults. Capacity refuses new admission with `ErrCapacity`. It never cancels
accepted work or evicts an unconfirmed resource. Only original work completion,
resource release evidence, complete Use retirement or watch closure frees the
corresponding capacity. Each limit applies independently, so a Host's resource
budget can refuse a new Use before its Use-count limit is reached.

`NewAdmissionWithLimits` configures `MaxPreparations`, `MaxPreparationChanges`,
`MaxScopes` and `MaxUseOwners`, with defaults of 4096, 4096, 4096 and 256.
`NewAdmission` uses that same
constructor. `Begin` can refuse a new preparation and returns its error before any
assembly work. Original publication or abandonment frees that preparation's slot.
Each active preparation retains at most `MaxPreparationChanges` distinct changed
installation identities; repeated changes to one identity share an entry. An
additional identity invalidates only that unfinished preparation and stops its
history from growing. It keeps its preparation slot until publication or
abandonment. `Publish` then returns both `ErrStaleBinding` and `ErrCapacity` without
calling the business publication. Already committed changes are retained, and
unrelated changes within the budget do not invalidate a publication.
Fenced scope generations and registered business owners remain authoritative and
are never evicted for capacity. Reopening an existing scope uses its original slot;
fencing a never-opened scope allocates nothing. Installation scope capacity is
checked under the same admission before a durable installation change.

The recovery barrier retains its first original cause for `errors.Is` and
`errors.As`. Later reports increment a fixed-size count instead of retaining
additional error graphs; `RecoveryError` renders at most 2048 bytes of first-cause
detail plus that count. Formatting runs outside the Admission state mutex. Every
report still advances the barrier generation, so a recovery already in progress
cannot clear a newer failure. Successful recovery clears this barrier's diagnostic
state. Unsettled operations and dependency protections remain owned by their
original Store and UseOwners, independently of diagnostic retention.

## Persistence, recovery and business retention

Native in-memory consumers can use the Core without SQLite. Durable process and
browser consumers use one `sqlite.Store`, one Host and the same `Admission`.
The SQLite profile requires a reliable local file, WAL, synchronous FULL and one
exclusive authority. It does not coordinate independent database writers or
transactions across separate databases.

Register original business owners with `Admission.RegisterUseOwner`. Each owner
queries its own queued, running or otherwise retained records; Qimu does not keep
another independently advancing business reference ledger. A transient `Use`
protects active access. A queued business record can retain an exact binding even
when no callback is running. An execution protection preserves resources whose
actual exit has not yet been proved. These are distinct reasons to refuse a
replacement or collection.

`Admission.Retained` reports each dependency alongside its registered `UseOwner`
identity. `Held` projects their distinct dependency set from that same fresh-read
path. Neither query caches retention or invents an alternative business ledger.

A failed or invalid required owner read raises the recovery barrier; it cannot
be interpreted as an empty retained set. A cancellation consisting only of the
current request's cancellation ends that request. A concurrent storage failure
still requires recovery. The composition root restores the original owner and
successfully reads its current facts before clearing the barrier.

`sqlite.Owner.Prepare` allocates a stable submission identity. `Commit` takes the
same admission gate and a `BEGIN IMMEDIATE` transaction. Its trusted mutation
validates the original binding using `Host.ValidateBindingUnder` or
`ValidatePlanUnder` and writes the original business record in that transaction.
The callback uses the provided context and SQL executor; it must not enter another
admission gate, run plugin code or make a network call. Application effects run
after the conclusion is known.

Committed and NotCommitted are stable conclusions. An indeterminate transaction
raises the recovery barrier instead of authorizing replay. On reconstruction,
settle original submissions, restore installation and retained binding projections,
and reconcile old execution evidence before admitting new work. Proof and
monotonic cleanup paths remain available behind the barrier. Only the composition
root calls `Admission.Recover` after those owners have recovered.

Detailed evidence has explicit capacity and retention. Compaction advances only a
terminal, synchronized, unreferenced sequence prefix. An expired identity never
becomes a new submission. See [SQLite](sqlite/README.md) for the transaction and
recovery APIs.

## Processes, browser execution and distribution

The [process driver](process/README.md) records a durable attempt before asking
the independent [runner](runner/README.md) to create a process. The runner owns
its actual direct child and wait result. A surviving runner can close that child
after Host reconnection. If the runner itself crashes, unresolved attempts remain
Unknown; a reused PID or new epoch does not prove termination. The default proof
covers that directly owned child, not arbitrary descendants or external effects.
The Unix implementation has an explicit same-user local boundary and fails on
unsupported platforms rather than claiming a weaker proof as equivalent.

The [browser Host adapter](control/host/README.md) uses the same Go Host and Store.
`qimu/control` owns its wire types and generated TypeScript projection. The
[Browser Runtime](browser/README.md) executes immutable approved LocalPlans,
owns actual JavaScript objects and reports their original release evidence. It
does not implement another Host, provider resolver or public directory. Existing
approved same-Realm native accesses need no RPC per call. New Uses, public
contributions and public Event routes require Host admission.

Authenticated Realm reconnect can complete old cleanup. A new Host epoch gives an
old execution cleanup authority only. Permanent loss of an execution owner can
leave its exact protection unresolved. ESM modules cannot be unloaded by deleting
a cache entry; a controlled Realm replacement is required. Native Go and native
JavaScript are trusted code, not security sandboxes.

[Distribution](distribution/README.md) validates bounded neutral archives and
owns immutable content leases. A release may combine process, browser and static
resource Units. Consumers supply package trust and source policies. Content hashes
prove byte identity, not publisher trust. Resource reads do not require inventing
a process, method or business operation.

## Packages and examples

| Package                                | Responsibility                                                                             |
| -------------------------------------- | ------------------------------------------------------------------------------------------ |
| `qimu`                                 | Contracts, exact graphs, admission, installation state, Points, Events, Uses and Lifetimes |
| `portable`, `jsonvalue`, `contractgen` | Portable descriptor validation, exact JSON and generated contract projections              |
| `mcp`, `process`, `runner`             | Author protocol, approved process execution and actual process ownership                   |
| `sqlite`, `distribution`               | Durable settlement/protection and immutable release content                                |
| `control`, `control/host`              | Control protocol and authenticated browser Host adapter                                    |
| `@qimu/sdk`, `@qimu/browser`           | TypeScript contract/resource APIs and approved browser execution                           |
| `@qimu/node`, `@qimu/react`            | Node MCP author adapter and React resource rendering adapter                               |

The [build/check example](examples/buildcheck/README.md) uses four different native
Points without a database or transport. The [document workbench](examples/document/README.md)
combines native execution, original queued task bindings, SQLite settlement,
browser preview and static resources. Its CLI and Web application own their
document revision checks and result semantics.

The paired [author exercises](examples/authoring/README.md) implement document
plugins through public Go and TypeScript APIs. They cover FileReader imports,
Converter and Validator Points, resource-valued results, cancellation and partial
setup failure. The TypeScript exercise uses the real Host and control Bridge;
the author does not implement the admission or exit protocol.

## Build and verify

Use the Go version declared in `go.mod`. From this directory:

```sh
GOWORK=off go test -race ./...
GOWORK=off go build ./cmd/qimu ./runner/cmd/qimu-runner
GOWORK=off go run ./examples/buildcheck/cmd/buildcheck -apply
```

The build/check command reads source text from standard input. Process and HTTP
tests create isolated local children and listeners and need an environment that
permits them. They do not require a Flame Runtime instance.

From the monorepo root, run `npm run check --workspace` with each of `@qimu/sdk`,
`@qimu/browser`, `@qimu/node` and `@qimu/react`. Generate control DTOs from Go with
`go run ./cmd/controlgen --out sdk/src/protocol.generated.ts` in this module. The
generated TypeScript declarations must not be edited independently.
