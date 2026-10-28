# Portable MCP execution

This package binds published Portable descriptors to MCP methods, streams and
authenticated Host callbacks. A method's contract owns input, output, operation
kind and declared business failures. The consumer owns business identity,
authorization, durable results and recovery; transport success cannot settle an
unknown business operation.

`Callbacks` owns one lazily opened loopback endpoint. `AdmitSession` establishes
the exact session's authority, and `Open`, `OpenRPC` or `OpenRPCWithInvocation`
create an original call grant. Keep each returned release handle with its owning
operation. Release withdraws admission, cancels the original context and joins
every admitted handler. `RevokeSession` joins the same calls before removing the
session. Neither cancellation nor a disconnected HTTP request proves that a
handler has stopped.

`NewCallbacksWithLimits` requires positive `MaxSessions`, `MaxCalls` and
`MaxPendingRequests` capacities. `NewCallbacks` uses 256 sessions, 4096 calls and
4096 concurrent requests. These are admission bounds, not eviction targets.
Retiring sessions and calls continue to consume capacity until their original
work joins. Full admission returns `ErrCallbackCapacity`; the HTTP boundary
projects pending-request saturation as 429, which both unary and streaming
clients preserve as that error. Per-call `CallbackLimits` separately bounds
request and response bytes. No unbounded delivery queue or automatic retry is
introduced.

A retained terminal release handle preserves its original join result without
retaining the former callback Host, request context or native handler objects.
Callback references contain ephemeral credentials; never log or persist them,
put them in a release, or reuse them for another operation.

Run `go test -race ./mcp` from the Qimu module. The tests use owned loopback
listeners, real MCP sessions and actual HTTP requests, including nested callbacks,
capacity refusal, cancellation, joined release and terminal-handle collection.
