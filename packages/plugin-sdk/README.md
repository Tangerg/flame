# Plugin author SDK

The public SDK for authoring a plugin's backend, partitioned by implementation
language so each binding is an independent consumer surface, mirroring
[`runtime-client`](../runtime-client).

- [`go/`](go) — the Go backend binding (`github.com/Tangerg/flame/packages/plugin-sdk/go`).

A second-language binding is added as a sibling directory. Every binding implements
the same backend protocol, capability and callback contracts; none is a second host.
React UI components are authored separately through [`component-sdk`](../component-sdk).
