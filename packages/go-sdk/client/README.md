# Remote Go Runtime client

Package `client` consumes Runtime's public protocol over HTTP/SSE. It
contains no host bootstrap, server transport, Domain or Application dependency.
It implements typed methods, stream framing, callback calls and original-command
uncertainty/reconciliation. Runtime's delivery registry generates both these
methods/metadata and the local binding; edit the registry rather than generated
operations.

Import `github.com/Tangerg/flame/packages/go-sdk/client` and use
`packages/go-sdk/protocol` call options, wire values and strict envelope codec.
Native message types come from `github.com/Tangerg/go-sdk/jsonrpc`.
The server's admission and lifecycle handlers
remain private. `client.ErrClosed` describes a closed transport owner;
`runtime.ErrClosed` describes a closed in-process host.

Backend implementations use `packages/go-sdk/plugin`. Add this client only when
they need a Runtime callback; the backend SDK does not depend on the client.
Run `go test ./packages/go-sdk/client` and the root boundary checks.
