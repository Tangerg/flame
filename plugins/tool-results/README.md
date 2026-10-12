# Tool results

An optional first-party Agent Plugins package for recorded tool-result presentation.
Install this directory through the local application's plugin manager or CLI and enable it. The package has no backend and executes no tools.

The manifest declares display-name and result-shape selectors. These select presentation
only; executable definitions, policy, approval and recorded outcomes remain Runtime-owned.
The host prefers one exact-name match over one shape match. Competing matches use the
generic inspector; installation order never selects a winner.

The page receives `flame.tool-result.v1`: one captured, settled transcript `tool` projection.
Its fields follow the Desktop ToolCall read model, including the original call and Run IDs,
name, argument text, result text, status and any recorded range or error. It receives no
Runtime client, credentials, filesystem access or tool-invocation bridge. Refresh repeats
the same captured snapshot after Runtime reauthorizes the selected release.

Unsupported snapshot formats report a bounded local failure and retire the page.
Rows, ANSI styling, filters and incremental expansion are local presentation. Authored result bytes are
rendered as text, including HTML and URLs. File navigation, external links, copy, approvals
and mutation actions remain trusted host controls. Revocation or replacement destroys the
page and leaves the generic inspector and original recorded outcome available.

The renderer uses the Host's read-only appearance projection and the shared
`@flame/ui` stylesheet and controls. `src/result.html` owns recorded-content layout
and its existing read channel; `build.mjs` renders shared controls into a
self-contained resource. Rebuild through `npm run build --prefix plugins`.
Do not edit generated `renderers/result.html`.
