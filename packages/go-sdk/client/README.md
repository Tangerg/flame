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

Call options use the SDK's compiled protocol version when
`RequestMeta.ProtocolVersion` is nil. A supplied pointer carries that exact
version claim to Runtime, where an empty or unsupported version is refused.

A plugin command requires a caller-retained `PluginCallOptions.IdempotencyKey`
for receipt recovery. Plugin queries omit the command identity.
Compose `CallPluginRequest` with a `PluginCallRoute` and the original input
text. Command lookup returns that same division between route and input.

Typed plugin bindings capture the admitted contract by value. Replacing the
caller's Capability cannot retarget a bound method or its delayed stream.
Point resolution retains that same captured schema across the remote call,
including validation of the returned values.

Closing the client withdraws admission before canceling calls. Concurrent close
callers join the same completion, including response cleanup already started by
request cancellation. A closing response remains owned until its body closes.

HTTP metadata must retain its exact value. Invalid field bytes and surrounding
HTTP whitespace are refused before dispatch, rather than changing a command's
identity or reporting an unknown acknowledgement for a local encoding failure.

A Run's acknowledgement binds both its root Run and Segment. The stream
delivers child events independently and completes when the acknowledged pair
publishes its terminal event. A notification matching just one of the two
identities fails observation before delivery.

Backend implementations use `packages/go-sdk/plugin`. Add this client only when
they need a Runtime callback; the backend SDK does not depend on the client.
Run `go test ./packages/go-sdk/client` and the root boundary checks.
