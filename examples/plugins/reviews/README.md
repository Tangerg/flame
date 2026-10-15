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

The backend uses the public Go binding in `github.com/Tangerg/flame/runtime/plugin`.
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
package admits that contract; `runtime/plugin` binds it to MCP without a second
schema or fingerprint owner.
The Status and Outcome types publish their closed vocabularies to Scope's schema
derivation. From `backend`, `go generate ./...` projects the same methods into
`package/io.github.tangerg.flame/contracts/{capability.generated.json,schema.json,methods.generated.json,API_REFERENCE.md}`
and `client/methods.generated.ts` through Runtime's existing `contractgen`.
Do not edit those generated files or maintain a second input/output schema.
The canonical capability descriptor supplies the same contract for live export
resolution, original-binding restoration and the generated TypeScript token.
Schema and method references are presentation projections. Capability calls and
receipt queries carry the original export identity; omitting it is refused before
the business owner. The package declares its generic installed contribution and analyzer point.
The ordinary board is compiled from `client/board.tsx` through the public SDK.
The same Go-owned analyzer point also generates `client/analyzers.generated.ts`
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

Build the executable for the Runtime host's OS and architecture from the repository
workspace, using the Go version declared in `go.work`:

```sh
cd examples/plugins/reviews/backend
mkdir -p ../package/bin
go build -trimpath -ldflags='-s -w' -o ../package/bin/reviews ./cmd/reviews
```

Install the `package` directory, not the source module. Flame accepts the packaged
command `./bin/reviews`, binds `PLUGIN_ROOT` to immutable execution content, and
binds `PLUGIN_DATA` to the installation's private data directory. The stripped
package must fit the existing 128 MiB aggregate admission limit.
Rebuild and admit new bytes when changing the backend. A selected new digest
is an explicit trust decision; it preserves desired enablement when configuration remains valid.

For an already running Runtime, use its endpoint and an absolute source path on
that machine:

```sh
flame --runtime-url http://127.0.0.1:17171 plugins install \
  --request '{"source":"/absolute/path/to/flame/examples/plugins/reviews/package"}'
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

The backend acceptance `TestInstalledReviewsPointUsesCapturedCollaboratorsAndCurrentConsumerLifetime`
builds and installs both executables, calls the analyzer through Core callbacks,
checks disable/reenable and refuses newly discovered contributors outside captured
setup authority. Reconnecting the consumer explicitly admits its new collection.
Point declaration availability belongs to the target consumer installation; another
enabled copy cannot keep a disabled consumer's point available. This backend proof
does not execute the board. Actual installed Web qualification separately covers
applicable/inapplicable actions, provider absence, metadata-only menu construction
and explicit analysis. Independent public-module compilation, native packaging
and broader graphical lifetimes retain their own verification scope in the
refactor progress record.

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
The full graphical restart/update/uninstall/provider review retains its own
qualification scope in the refactor progress record.

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

## Verification and current boundary

From `backend`, run `go test ./...`, `go vet ./...`, and `go build ./...`.
`go test -race -run TestConcurrentDuplicate ./...` checks the changed transaction
ownership. Use `GOWORK=off` for the same checks against this module's independent
released dependency graph once its Runtime dependency names a release containing
the new public SDK. Current workspace checks use the branch's SDK. An isolated
offline author build of current product source `6d5e2427d` is verified with Runtime
and companion localruntime module archives, `GOWORK=off`, and no local `replace`.
The archives include the actual implementation required by their public packages,
including internal directories; author source imports only public APIs. A
public-directories-only archive does not contain that implementation closure.
The checked archives use the explicit temporary version
`v0.0.0-unreleased.6d5e2427d557`. Their source staging directory is removed before
workspace-off tidy, module verification, full race, vet and build pass through
an isolated module cache, with no replace. The fixture also compiles the public
Tool-proposal contributor and runs the unchanged executable command-decoration
regression. The offline proxy is not a published release: this module's currently
declared older Runtime version still lacks the new public SDK. Current selected
module metadata also passes offline, including the thirty Scope v0.51.1 modules;
this does not qualify registry availability or publication.

Schema accepts integer spellings such as `0.0` and `1e0`, while the typed Go
decoder rejects them before Store entry. Runtime now asks that same registered
typed input owner for complete structural admission before saving a command route.
The real installed CLI regression first reproduces an unconfirmed command on old
code, then verifies definite invalid-parameters refusal without consuming the key.
Canonical `0` with that key still reaches the business owner, saves `invalidUpdate`
and recovers its receipt; another input cannot reuse the admitted identity.
This is a shared SDK/Runtime admission contract, with no Reviews-specific numeric
coercion or business receipt for a never-admitted request. The current backend
protocol is `2026-10-15.3`; rebuild every package and consumer together. The
broader qualification limits remain in the
[refactor handoff](../../../docs/plugin-refactor-handoff.md).

`TestTypedAndToolEntrancesShareTheBusinessReceipt` compares complete canonical
results and the Tool error flag for revision, invocation and invalid-input refusals.
It also reads the public receipt of invalid revision and Unicode byte-overflow
decisions. It retains
4,096 receipts through real Store updates, admits another successful update and
preserves new business refusals and original recovery beyond that history size.
This SDK/MCP test uses
in-memory transport; it does not execute the CLI or board.

From `runtime`, `go test ./internal/bootstrap -run TestReviewBackend -count=1`
builds and installs the real stdio backend in temporary directories. It exercises
approval, Runtime/backend restart, command replay, stale revisions, withdrawal,
historical results, and a real CLI querying and receiving revision-conflict
results through authenticated HTTP. That CLI case compares the complete canonical
results with the model Tool output, including same-command refusal redelivery;
it does not claim CLI coverage of every refusal class. A transport probe drops
an actual committed update's response: Runtime retains a lost Run and unresolved
effect, the restarted backend recovers the original receipt through the public
read-only query, and a replacement call returns the revision conflict. An absent
receipt does not enter the mutation handler. That test also verifies process retirement
and preserves the Run's original uncertainty after restart. Its model is scripted;
the test needs no provider credentials or external service.

`go test ./internal/bootstrap -run TestReviewReceiptHistory -count=1` instead
starts a separate HTTP backend fixture with 4,096 real historical receipts before
Runtime connects. Separate model Run/executable CLI cases compare the complete
missing-review and invalid-revision refusals. Each first rejects an oversized
public request, then uses that still-available command identity for actual work.
The CLI and public Endpoint recover the new refusal
through the original read-only receipt query. Child-process counters prove one
Tool dispatch, one read-only typed input validation, one typed update dispatch
and two receipt reads; final private
assertions retain the original Review and all 4,098 receipts. The Store regression
also reopens the private database and reads original success/refusal receipts
after an actual successful update beyond the old cutoff. Separate invalid-input
cases verify exact recovery and changed-input conflict across private restart.
These history cases do not execute the board.

`TestReviewExecutableCLIIdentityConflictPreservesOriginalReceipt` builds the real
CLI and stdio backend and checks the human binding boundary through authenticated
HTTP. A successful update is followed by repeated changed-input conflict and
matching Go classification. A fresh key advances the review, while the original
key/input still returns its complete earlier receipt without changing current
state. This qualifies Core's `idempotency_conflict`.

`TestReviewPrivateInvocationConflictMatchesModelToolAndExecutableCLI` separately
compares private `invocationConflict` through a real model Run and executable CLI.
A separate HTTP backend makes real prior private calls with captured identities
before forwarding unchanged public requests to the production handlers. The
controlled collision has original atomic `notFound` receipts, without fake replies
or direct receipt writes. CLI/Go recovery explicitly refuses those different-input
receipts instead of replacing them or replaying the requested mutations. A fresh
CLI identity succeeds once. Child-process assertions preserve both original
outcomes and require exactly three receipts, one Tool, two typed dispatches, two
pure validations and two receipt reads. The graphical entrance remains unexecuted.

This package's MCP tools execute through Session-owned Runs and the existing
approval path. Its ordinary Board calls the same published business operations
through the public component SDK. Diagnostic `tools.invoke` is not a plugin action
entrance.

## Ordinary component source

`client/board.tsx` exports the ordinary React `Board` component through the public
`@flame/plugin-sdk` context. It reads the owner, submits the inspected revision,
shows explicit conflict/refusal, preserves the original mutation identity for
reconciliation, and invokes the installed analyzer query. Refresh failures retain
previously read reviews; an uncertain command disables fresh changes.

The owning generator now publishes the complete `CAPABILITY_JSON` string beside
its typed method maps. Send that string unchanged. Converting schema integers to
JavaScript numbers can change their exact contract identity.

From `examples/plugins/reviews/client`, install the declared public dependencies,
then use `npm run typecheck` and `npm run build`. The shared public SDK build emits
`package/io.github.tangerg.flame/components/component.js` with React, JSX runtime,
SDK and UI imports externalized for the host's actual shared module instances.
The same prebuilt ESM format is used for development and release builds.

The namespace declaration now publishes `board` as an installation-scoped
workspace component with its `review-operations` binding. The workbench uses the
same generic ESM/resource subscription and shared module mapping for this ordinary
installed package. Generation replacement requests a window reload; it cannot
unload evaluated JavaScript. Unmount cancels observation and Dougong joins SDK I/O.

The former HTML carriers are removed. Compilation, production bundles and backend
resource-lifetime checks provide source/build evidence. Actual rendered Web Board
query/refusal comparison and original-command recovery now have matching evidence.
Web/Wails resource lifecycle and native Memory recovery retain their separate
scenario scope in `docs/plugin-refactor-progress.md`; these do not prove every
Reviews workflow or every graphical refusal class.
