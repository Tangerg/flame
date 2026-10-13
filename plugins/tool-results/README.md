# Tool results

An optional first-party package for recorded Tool-result presentation. Install and
enable it through the plugin manager or CLI. It has no backend and executes no Tools.

Ordinary components declare `toolResult` scope, `toolResultRenderer` placement and an
exact display-name or known result-shape selector. One exact-name match precedes one
shape match; equally ranked candidates remain ambiguous and use the generic inspector.
Running and absent results stay generic. The declarations share one prebuilt ESM export.

The public SDK supplies `context.toolResult`, a detached frozen settled transcript
projection containing the original call/Run IDs, arguments, result, status, ranges and
errors. This data does not follow navigation or reread a historical producer. File
navigation, copy, approvals and explicit mutation actions retain their existing owners.

Filters, ANSI styles, line numbers and incremental expansion are local presentation.
Authored result bytes, HTML and URLs render as text. The client preserves generic
inspection during loading, failure, connection retirement and withdrawal. Opening
history never downloads or executes the old producer's JavaScript or replays a Tool.

The component is trusted workbench code using shared React, the public SDK and UI
controls/provider context. It uses the common exact-release resource lease and logical
component lifetime; replacement may require a window reload. The HTML guest, channel,
source, resource and dedicated builder are removed. Rebuild through
`node plugins/build-components.mjs`; verify with the same command's `--check` option.
Do not edit generated namespace-local `components/component.js`.
