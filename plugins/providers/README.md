# Installed Scope model adapters

This optional backend implements `flame.model-provider.v1` with released Scope
SDKs. Its provider profiles own vendor endpoints, catalog facts and exact
construction of chat, native streaming, complete-request counting and embeddings.
No provider is inferred from a model name.

Runtime supplies an immutable credential snapshot when opening a model. One
private handle retains the actual client and its supported capabilities. A
restart reconstructs that same snapshot; changing current credentials cannot
change an existing handle. Embedding space identity includes the exact provider,
model and configured endpoint, with credential rotation excluded.

The backend retains original command input and results in its private database.
An interrupted exchange remains unconfirmed and cannot be sent again under the
same command identity. Streaming stores its final complete response or failure
separately from acknowledged original deltas. Closing a handle fences new work
and joins accepted exchanges. Private connections and receipts contain secrets;
they are never public configuration or model arguments.

Runtime owns model selection, trust, request admission, endpoint evidence,
accounting and execution recovery. Installing this backend grants none of those
authorities to the provider. A missing or disabled selected adapter is a visible
failure; it cannot select a replacement model.

Fresh Runtime databases select this adapter from the shipped catalog. Existing
installations must explicitly install, enable and select its `model-provider`
contribution for `flame.model-provider.v1`. Disabling it refuses new model work;
Session history and plugin management remain available. A waiting Run retains its
exact release and original construction receipt. Finish waiting work with its
matching Runtime build before upgrading; no compatibility checkpoint reader is
provided.

Runtime process configuration uses `FLAME_APIKEY` for the configured default
provider and the canonical provider registry for other credentials. Stored values
win over this process fallback. Former vendor-specific environment aliases are
removed; configure their values through `providers.update` or the default
provider's process setting. An explicit model is required for providers other
than the product default `anthropic/claude-opus-5`.
