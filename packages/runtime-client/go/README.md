# Remote Go Runtime client

Package `runtimeclient` consumes Runtime's public protocol over HTTP/SSE. It
contains no host bootstrap, server transport, Domain or Application dependency.
It implements typed methods, stream framing, callback calls and original-command
uncertainty/reconciliation. Runtime's delivery registry generates both these
methods/metadata and the local binding; edit the registry rather than generated
operations.

Import `github.com/Tangerg/flame/packages/runtime-client/go` and use
`runtime/protocol` call options and wire values. `runtime/protocol/jsonrpc`
owns the shared envelope codec. The server's admission and lifecycle handlers
remain private. `runtimeclient.ErrClosed` describes a closed transport owner;
`runtime.ErrClosed` describes a closed in-process host.

Backend implementations use `packages/plugin-sdk/go`. Add this client only when
they need a Runtime callback; the backend SDK does not depend on the client.
Run `go test ./packages/runtime-client/go` and the root boundary checks.
