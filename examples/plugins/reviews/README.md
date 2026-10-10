# Review backend example

This portable package provides `list_reviews`, `update_review`, and the `review`
Skill. It runs through Flame's existing installation-origin MCP connection,
deferred Tool discovery, approval Interrupt, Run execution and result storage.
It declares no graphical view or host action.

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
build fits the existing 16 MiB single-file admission limit on macOS arm64.
Rebuild and admit new bytes when changing the backend. A selected new digest
requires approval and enablement again.

For an already running Runtime, use its endpoint and an absolute source path on
that machine:

```sh
flame --runtime-url http://127.0.0.1:17171 plugins install \
  --request '{"source":"/absolute/path/to/flame/examples/plugins/reviews/package"}'
flame --runtime-url http://127.0.0.1:17171 plugins list
flame --runtime-url http://127.0.0.1:17171 plugins approve \
  --request '{"installationId":"<installed-id>","digest":"<selected-digest>"}'
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
released dependency graph.

From `runtime`, `go test ./internal/bootstrap -run TestReviewBackend -count=1`
builds and installs the real stdio backend in temporary directories. It exercises
approval, Runtime/backend restart, command replay, stale revisions, withdrawal,
historical results, and a real CLI reading through HTTP. A transport probe drops
an actual committed update's response: Runtime retains a lost Run and unresolved
effect, the restarted backend recovers the original receipt, and a replacement
call returns the revision conflict. That test also verifies process retirement
and preserves the Run's original uncertainty after restart. Its model is scripted;
the test needs no provider credentials or external service.

This package's MCP tools execute through Session-owned Runs and the existing
approval path. It has no review board, trusted tool-call form, or HTML bridge to
backend operations. Diagnostic `tools.invoke` is not a plugin action entrance.
