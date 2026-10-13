# Session investigation

This optional package supplies a public Session trajectory view and a contextual
`sessionAction` component for naming an investigation. Both declare Session scope
and share one namespace-local prebuilt ESM module. The package has no backend.

The Session row menu offers the action using local admitted metadata. Opening it
captures that row's accepted ID, revision and title as `context.sessionTarget`.
Navigation and refreshed row data cannot replace the open target. The action does
not fetch another revision while constructing its menu or silently overwrite a conflict.

The component owns its form and calls the canonical public `sessions.update` command,
the same Session owner used by built-in editing and `flame session rename`. It submits
an explicit title only, with the inspected revision. Unknown acknowledgement retains
the original mutation handle/key and offers same-command reconciliation; a new edit
is blocked until settlement. Accepted data is shown only from the canonical response.
Close and reopen for another edit or a fresh inspected revision.

Disablement withdraws menu registration and ends supported component resources;
closing observation does not retract an accepted Core command. Its durable receipt
remains owned by the canonical Session delivery operation. The former closed action
operation/declaration, special Core/CLI wrapper and private Host form are removed.

Rebuild with `node plugins/build-components.mjs`; use `--check` for exact ESM
reproduction. Reinstall/select changed package bytes. React, SDK and UI come from
actual shared Host modules; code replacement can require a window reload. No guest
or historical script installation path exists.
