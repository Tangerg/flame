# Review backend example

The root Flame namespace references io.github.tangerg.flame/declaration.json.
Its review-operations contribution binds the reviews server/export to the existing
generated capability file. Installation admits this metadata without starting the
backend; its publisher receives the same generic declaration path as bundled
Memory/Schedules.

This portable package provides `list_reviews`, `update_review`, and the `review`
Skill. It runs through Flame's existing installation-origin MCP connection,
deferred Tool discovery, approval Interrupt, Run execution and result storage.
It declares an ordinary React board bound to the same operations.

The backend uses the public Go binding in `github.com/Tangerg/flame/packages/plugin-sdk`.
Its typed list and update operations share the same private use cases and receipt
owner as the model Tools. The public installed operation entrance is available
through Runtime Go, Runtime Protocol, `plugins call`, and the TypeScript client.

The exports use the SDK's single invocation transport. Server setup binds the review list, update and analyzer operations
and the read-only receipt query to one Backend, binds the `reviews`
capability export, then publishes it before
starting MCP. Discovery carries the current backend protocol and exact generated
method schemas; the former per-operation MCP endpoints are not registered.

`backend/contracts.go` owns the method descriptions, typed bindings and
the consumer's callable capability contract. The pure public `runtime/capability`
package admits that contract; `packages/plugin-sdk` binds it to MCP without a second
schema or fingerprint owner.
The Status and Outcome types publish their closed vocabularies to Scope's schema
derivation. From `backend`, `go generate ./...` projects the same methods into
`package/io.github.tangerg.flame/contracts/{capability.generated.json,schema.json,methods.generated.json,API_REFERENCE.md}`
and `component/methods.generated.ts` through Runtime's existing `contractgen`.
Do not edit those generated files or maintain a second input/output schema.
The canonical capability descriptor supplies the same contract for live export
resolution, original-binding restoration and the generated TypeScript token.
Schema and method references are presentation projections. Capability calls and
receipt queries carry the original export identity; omitting it is refused before
the business owner. The package declares its generic installed contribution and analyzer point.
The ordinary board is compiled from `component/board.tsx` through the public SDK.
The same Go-owned analyzer point also generates `component/analyzers.generated.ts`
as an exact JSON string; TypeScript does not copy its applicability predicate.
On refresh, the board resolves point metadata against its accepted review status
and publishes that read model with the review. Analyze appears only when that
projection contains an applicable contributor. Constructing controls or opening
the status menu performs no remote work; explicit Analyze invokes the backend.
This action relevance is an accepted snapshot, not an execution grant. Draft
status changes do not advance the accepted context. Metadata failure keeps
the review visible with an explicit error rather than claiming an empty collection.

The backend owns one example review in `PLUGIN_DATA/reviews.sqlite`. Runtime
owns execution and approval; its stored Tool results are historical observations,
not another review database. Read the backend again to observe its current state.

## Build and install

From the repository root, prepare the workspace and regenerate the contracts and
component. Assemble a new release under `out`, using the Runtime host's OS and
architecture:

```sh
make setup
make generate
mkdir -p out/plugins
mkdir out/plugins/reviews
cp -R examples/plugins/reviews/package/. out/plugins/reviews/
mkdir -p out/plugins/reviews/bin
go build -trimpath -ldflags='-s -w' -o out/plugins/reviews/bin/reviews ./examples/plugins/reviews/backend/cmd/reviews
```

Use a new output directory for each build. Install the assembled release rather
than a source directory. Flame binds `PLUGIN_ROOT` to immutable execution content
and `PLUGIN_DATA` to the installation's private data directory. The stripped
package must fit the existing 128 MiB aggregate admission limit. Rebuild and admit
new bytes when changing the backend; selecting a new digest is an explicit trust
decision and preserves desired enablement when configuration remains valid.

For an already running Runtime, use its endpoint and an absolute source path on
that machine:

```sh
flame --runtime-url http://127.0.0.1:17171 plugins install \
  --request '{"source":"/absolute/path/to/flame/out/plugins/reviews"}'
flame --runtime-url http://127.0.0.1:17171 plugins list
flame --runtime-url http://127.0.0.1:17171 plugins set-enablement \
  --request '{"installationId":"<installed-id>","enabled":true}'
```

In a normal agent Session, discover and call `list_reviews`, then request an
explicit status update for the returned ID and revision. The update input has
`id`, `expectedRevision`, and `status` (`open` or `resolved`). Approve through
the existing Tool prompt. Tool definitions come from the backend's frozen Scope
contracts; neither the package manifest nor Runtime copies their schemas.

After a fresh backend read in that Session, the CLI reads the same recorded result:

```sh
flame --runtime-url http://127.0.0.1:17171 sessions show '<session-id>' --json
```

## Analyzer composition

Reviews declares a consumer-owned collection point in
`backend/analyzers.go`. The typed value names an analyzer contribution in the
same installed release. The point owns its schema, installation scope, canonical
key ordering, optional absence and applicability to an open review. Its generated
contract is packaged with both the consumer and the independent analyzer.

The separate `analyzer/package` contributes an implementation of Reviews' typed
Analyze capability. It receives only an inspected snapshot and returns findings
with that review ID and revision; it does not open Reviews' private data.
Reviews declares the named optional `analyzers` capability dependency, so Core
checks cross-plugin calls against the entry's frozen setup bindings.

The Reviews analyzer query reads one current snapshot, resolves applicable point
values through its call-scoped Core callback, invokes their exact installed
release/contribution through `BindPluginMethod`, and admits finding evidence
before returning it. An unavailable implementation or invalid result fails the
query; it does not become an empty successful analysis. An optional empty
collection and an inapplicable resolved review return an empty finding list.
The query does not change the review or create another receipt store.

Build and install the analyzer following [its instructions](analyzer/README.md).
Enable it before activating Reviews so it is in Reviews' frozen optional setup
collection. Reload Reviews when deliberately changing that collection.
Call `io.github.tangerg.flame.examples/reviews.analyzers` on
`review-operations` through `plugins call`, with the selected Reviews installation,
digest, generated capability descriptor, an installation target and `{}` input.
This uses the same public invocation path as other installed operations.

The backend tests exercise captured collaborators, disable/reenable and explicit
collection refresh. Component interaction and native resource lifetime remain
separate contracts; a backend test does not execute the board.

## Mutation and recovery

Runtime supplies its calling-owner context, including its canonical logical Tool call ID, in MCP request metadata under
`io.github.tangerg.flame/invocation`. The backend never derives this identity
from the provider's ToolCall ID, review revision, or user arguments. This metadata
is a projection of execution identity, not a credential or approval grant.

The review transition and immutable invocation receipt commit in one SQLite
transaction. The same identity and admitted arguments return the original result,
including after a backend restart. Reusing that identity with different arguments
returns `invocationConflict`. A new call with an old revision returns
`revisionConflict` with the current review; it never silently rebases the update.
Inputs and identities are bounded before storage. Historical receipts do not
consume a permanent mutation quota and are never expired or deleted to make
space. Valid new business decisions, including known refusals, retain their
original receipts in the same transaction.

The Go-owned `Update` schema bounds identifiers to 128 characters. The business
rule retains its stricter 128-byte limit, derived from the same constant; Unicode
input can satisfy structural admission and still receive `invalidUpdate`. Zero,
negative or above-maximum expected revisions and an empty identifier are known
business refusals. They consume the original command identity and save a receipt;
corrected input requires a fresh identity. Malformed types, unknown status values
and structural overflow are rejected before command admission and cannot consume
an identity. Runtime projects such schema rejection as `invalid_params` before
binding/dispatch, for both unary and streaming commands.

Runtime binds a human command key to its complete original operation before
business dispatch. CLI or Go calls that reuse that key with different input get
the typed `idempotency_conflict` problem, including when the changed input is
otherwise valid. The original key and input still recover the original private
receipt after another command advances the review; a new intent uses a new key.
Direct SDK/Tool calls also check the private invocation binding and preserve the
backend's `invocationConflict` outcome. These checks protect their respective
owners and have separate evidence.

The board's unresolved command controls are projections of the existing client
journal, so recreating the component/client can recover the original identity.
`plugins.getCommand` reads original admitted routing/input from Runtime without
backend dispatch; only then does the existing call path query the original private
receipt. The board never uses the newly read revision or current draft for that
recovery. Multiple retained commands remain separately visible and block another
status submission until their outcomes are established. Original-release absence
or input conflict keeps its command unresolved rather than choosing another owner.

The Store compiles the same Go-owned input schema at open and uses it for writes
and receipt integrity checks. Receipt reads do not rerun business admission:
an original `invalidUpdate` remains recoverable after restart or later work.

The former 4,096-entry lifetime cap and `receiptCapacity` outcome are removed.
That branch returned a known refusal without storing its receipt, so a later
read-only reconciliation could only report uncertainty. The current operation
contract, generated TS and component artifact omit that outcome; no alias or
compatibility path accepts it. This contract change does not change the current
private `input_json` storage format or bypass original-release retention.

The public `commands.get` method reads an existing private receipt without
entering the update use case. It checks the original update method and input
fingerprint and returns the original typed outcome, including a stored revision
conflict after the current review has advanced. Missing evidence stays absent;
binding conflicts, corrupt private evidence and storage failures remain protocol
errors. The query adds no second receipt table or host result cache.

The binding retains the original admitted JSON before decoding the Go input.
Object member order does not change identity, while numeric lexemes remain exact.
The private `input_json` field is the immutable request source, and receipt lookup
derives its fingerprint from that source. It does not rehash a decoded DTO. The
former `arguments` format stored re-encoded Go input and is refused at startup
before schema or business writes. Retain the old directory separately and use a
fresh `PLUGIN_DATA` directory for this breaking cutover.

Malformed inputs and known business refusals produce explicit Tool failures.
Unexpected storage or commit errors remain MCP protocol failures, so Runtime
retains execution uncertainty instead of inventing a definite failure. Do not
automatically submit a replacement mutation after an uncertain response.
Delivery command replay continues to use the original Run command identity;
the backend receipt does not replace that owner. Recovering a backend receipt
does not retroactively settle a finished Run whose response was never observed.

## Verification

From the repository root:

```sh
make check
make check-native
make check-author
```

The root check discovers the Reviews backend through `go.work`, runs its
independent dependency graph with `GOWORK=off`, and checks the component through
the npm workspace. `make setup` prepares the immutable current-source module
artifacts. No local `replace` or source snapshot supplies the SDK. Published
consumers must select a release containing these public packages; candidate
artifacts do not claim registry publication.

For focused backend validation, run `go test ./...`, `go vet ./...` and
`go build ./...` from `backend`. Runtime's real installed regressions run from the
repository root with:

```sh
go test ./runtime/internal/bootstrap -run TestReview -count=1
```

These scripted-model tests cover typed/Tool refusal identity, executable CLI
identity conflict, public receipt recovery, backend and Runtime restart, approval,
withdrawal, and retained receipt history. All execution uses temporary data and
local transports. They do not require provider credentials or qualify every
rendered board interaction.

Integer schemas can accept spellings such as `0.0` and `1e0` that the typed Go
input owner refuses. Runtime performs that same typed structural admission before
saving a command route. A malformed request never consumes its key; a later
canonical input with that key can reach the business owner. The current backend
protocol is `2026-10-15.3`; rebuild the backend, contracts and components together.

The backend's own tests preserve more than 4,096 real receipts, recover original
success and refusal after restart, reject changed-input identity reuse, and check
atomic business/receipt commits. The public `commands.get` query never enters a
mutation handler. Model Tools and ordinary Board calls use the same published
business operations; diagnostic `tools.invoke` is not a plugin action entrance.

## Ordinary component source

`component/board.tsx` exports the ordinary React `Board` component through the public
`@flame/component-sdk` context. It reads the owner, submits the inspected revision,
shows explicit conflict/refusal, preserves the original mutation identity for
reconciliation, and invokes the installed analyzer query. Refresh failures retain
previously read reviews; an uncertain command disables fresh changes.

The owning generator now publishes the complete `CAPABILITY_JSON` string beside
its typed method maps. Send that string unchanged. Converting schema integers to
JavaScript numbers can change their exact contract identity.

From the repository root, use `npm run typecheck --workspace @flame/reviews-example`
and `npm run build --workspace @flame/reviews-example`. The shared public SDK build emits
`package/io.github.tangerg.flame/components/component.js` with React, JSX runtime,
SDK and UI imports externalized for the host's actual shared module instances.
The same prebuilt ESM format is used for development and release builds.

The namespace declaration now publishes `board` as an installation-scoped
workspace component with its `review-operations` binding. The workbench uses the
same generic ESM/resource subscription and shared module mapping for this ordinary
installed package. Generation replacement requests a window reload; it cannot
unload evaluated JavaScript. Unmount cancels observation and Dougong joins SDK I/O.

The host consumes this ordinary ESM through the same installed resource and
contribution lifecycle as first-party components. Generation replacement requests
a fresh window; unmount cancels observation without cancelling accepted business
commands. Graphical query/refusal and original-command recovery regressions live
with the Web integration tests. Native staging and host behavior are checked by
`make check-native`.
