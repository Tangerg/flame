# Installed Scope model adapters

This optional backend implements `flame.model-provider.v1` with released Scope
SDKs. Its provider profiles own vendor endpoints, catalog facts and exact
construction of chat, native streaming, complete-request counting and embeddings.
No provider is inferred from a model name.

The installed settings component manages credentials, connection probes and the
utility and embedding roles through the same public Runtime operations used by
other consumers. It declares the `model-providers` settings intent so setup and
Composer navigation find its actual installation entry. The graphical client
retains only its ordinary model and provider catalogs for basic composition.

One explicitly constructed component source owns unsubmitted credential drafts,
read projections and original command observations across view closes. Source or
Runtime retirement clears that state and fences stale submissions. Known command
responses update the accepted projection before refreshing it; a refresh failure
cannot turn that command into a failure. An unconfirmed command blocks another
write to the same resource and retains its original promise for reconciliation.
Later edits survive settlement and cannot receive feedback from an older probe.
No plugin-owned credential registry, role state machine or module-global presenter
is introduced. Role controls fill their grid track, so they share the description's
reading axis when the row stacks.

Runtime supplies an immutable credential snapshot when opening a model. One
private handle retains the actual client and its supported capabilities. A
restart reconstructs that same snapshot; changing current credentials cannot
change an existing handle. Embedding space identity includes the exact provider,
model and configured endpoint, with credential rotation excluded.

Connection probes use the capabilities declared by the released Scope binding.
A binding with model discovery verifies credentials through that read. A
stream-only binding, including DeepSeek, verifies one ordinary exchange with its
default model; Flame does not infer discovery from the underlying wire protocol.
The current Anthropic Messages binding sends its API key as a bearer credential.

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

Fresh Runtime databases grant the shipped adapter the individual provider IDs in
its published scope resource. The backend registry generates that resource with
`go generate ./plugins/providers/backend`. Production and third-party installations
can coexist: select a contribution separately for each `flame.model-provider.v1/<provider-id>`
role in plugin management or with `flame plugins select-provider`. Credentials
travel only to the selected contribution for that exact provider. Disabling one
adapter leaves other providers available. A waiting Run retains its
exact release and original construction receipt. Finish waiting work with its
matching Runtime build before upgrading; no compatibility checkpoint reader is
provided.

Runtime process configuration uses `FLAME_APIKEY` for the configured default
provider and the canonical provider registry for other credentials. Stored values
win over this process fallback. Former vendor-specific environment aliases are
removed; configure their values through `providers.update` or the default
provider's process setting. Runtime process configuration names both its
provider and model explicitly.

The configuration screens render the base URL example and path policy published by the installed provider profile. Preserve the gateway mount path. Anthropic and Google append their API version to that mount. OpenAI-compatible gateways take a versioned API base, and Azure uses the resource-specific `/openai/v1` base with a deployment name.

For Azure, set the optional installation input `azure-deployments` to an object such as `{"https://resource.openai.azure.com/openai/v1":["team-chat"]}`. Discovery lists the declared deployment names for the selected resource. The credential probe still checks the upstream API.
