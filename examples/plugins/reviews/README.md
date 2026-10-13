# Review backend example

This portable package provides `list_reviews`, `update_review`, and the `review`
Skill. It runs through Flame's existing installation-origin MCP connection,
deferred Tool discovery, approval Interrupt, Run execution and result storage.
It declares no graphical view or host action.

The backend uses the public Go binding in `github.com/Tangerg/flame/runtime/plugin`.
Its typed `reviews.list` and `reviews.update` operations share the same
private use cases and receipt owner as the model Tools. These are backend method
bindings; a general installed-provider host operation entrance is still pending.

The exports use the SDK's single invocation transport. Server setup binds both
contracts and the read-only receipt query to one Backend, binds the `reviews`
capability export, then publishes it before
starting MCP. Discovery carries the current backend protocol and exact generated
method schemas; the former per-operation MCP endpoints are not registered.

`backend/contracts.go` owns the two method descriptions, typed bindings and
the consumer's callable capability contract.
The Status and Outcome types publish their closed vocabularies to Scope's schema
derivation. From `backend`, `go generate ./...` projects the same methods into
`package/io.github.tangerg.flame/contracts/{capability.generated.json,schema.json,methods.generated.json,API_REFERENCE.md}`
and `client/methods.generated.ts` through Runtime's existing `contractgen`.
Do not edit those generated files or maintain a second input/output schema.
The canonical capability descriptor supplies the same contract for live export
resolution, original-binding restoration and the generated TypeScript token.
Schema and method references are presentation projections. Capability calls and
receipt queries carry the original export identity; omitting it is refused before
the business owner. The package still lacks the generic installed contribution
declaration and graphical component entry required by the complete proposal.

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

## Mutation and recovery

Runtime supplies its canonical logical Tool call ID in MCP request metadata under
`io.github.tangerg.flame/invocationId`. The backend never derives this identity
from the provider's ToolCall ID, review revision, or user arguments. This metadata
is a projection of execution identity, not a credential or approval grant.

The review transition and immutable invocation receipt commit in one SQLite
transaction. The same identity and admitted arguments return the original result,
including after a backend restart. Reusing that identity with different arguments
returns `invocationConflict`. A new call with an old revision returns
`revisionConflict` with the current review; it never silently rebases the update.
Inputs and identities are bounded before storage. At 4,096 receipts, new calls
return `receiptCapacity`; existing receipts remain recoverable and are never
expired or deleted to make space.

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
offline author build of this source has also been verified with the current SDK's
public files packaged as a temporary module archive, `GOWORK=off`, no local
`replace`, and no Runtime internal source. That fixture is not a published release.

From `runtime`, `go test ./internal/bootstrap -run TestReviewBackend -count=1`
builds and installs the real stdio backend in temporary directories. It exercises
approval, Runtime/backend restart, command replay, stale revisions, withdrawal,
historical results, and a real CLI reading through HTTP. A transport probe drops
an actual committed update's response: Runtime retains a lost Run and unresolved
effect, the restarted backend recovers the original receipt through the public
read-only query, and a replacement call returns the revision conflict. An absent
receipt does not enter the mutation handler. That test also verifies process retirement
and preserves the Run's original uncertainty after restart. Its model is scripted;
the test needs no provider credentials or external service.

This package's MCP tools execute through Session-owned Runs and the existing
approval path. It has no review board, trusted tool-call form, or HTML bridge to
backend operations. Diagnostic `tools.invoke` is not a plugin action entrance.
