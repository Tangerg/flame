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
ANSI foreground colours and the `error`/`warning` line fallbacks use the host's readable
text roles (`--color-negative`, `--color-success`, `--color-warning`, `--color-accent-text`,
`--color-text-muted`), never the solid fill tones, so coloured output reads on the frame.
Rows and command output use the shared `CodeFrame` and `CodeCaption`, so recognised
results read like the generic inspector's captured code: the caption names the
content and holds Find, expansion, Jump to latest, Collapse and Copy. Copy writes the
recorded text, without ANSI styling for command output. Result filters open on
demand beneath the caption, above one bounded scrolling body. A web fetch names its
format in the caption. Paged results retain the shown and matching row counts in
that caption. Continuing a page reveals its first new row and returns focus to the
named result viewport, including the last page, instead of carrying keyboard focus
past the new evidence or dropping it when the continuation button disappears.
Output search identifies the current matching line with the
shared search treatment; Enter and Shift+Enter cycle matching lines. Clear empties
the query and retains search focus; Close and Escape return to the output region.
Ctrl/Cmd+A in a focused result reader selects its content rather than the workspace.
Paged or filtered rows and collapsed output previews select the presented material.
Expanded output represents the whole captured log: copying its whole-reader range
writes all ANSI-stripped source text, including virtualized lines and trailing
newlines. Ordinary partial selections retain native copy behavior. Search fields
and caption controls remain outside each reading range.
Find is a bounded task: Escape from its query or clear control closes it even over
a query, ahead of the field's own clear-on-Escape. IME composition keys neither
close Find nor cycle matches.
Applied-change receipts stay an unframed table whose verbs
and code-role paths rank below the Tool row. Source lines share a continuous numbered
layout; located source results keep their context above each snippet. Web results
and skill summaries separate the primary title, supporting URL and readable body
using the shared record typography. An absent or repeated web title shows its URL
once. The host Tool disclosure owns identity, status and failure details.
Missing snippets add no body row; recorded empty snippets and source lines retain
their blank line.
Known literal answers, instructions and source results are classified by Tool
identity before inspecting normalized result shapes. JSON-looking text stays
complete rather than being mistaken for a different Tool's result envelope.
Skill summaries decode the producer's XML text once and retain its truncation
notice. Unknown or malformed summary formats remain complete source text.
Result viewports contain horizontal overscroll and let vertical scrolling continue
through the transcript when the result reaches its edge or fits without scrolling.
Explicit expansion transfers focus once to the new output viewport after its
initial positioning, unless the user has already moved elsewhere. One persistent
text toggle owns both expansion and collapse; collapsing restores focus to that
control and clears the local search. Explicit Jump
to latest returns focus to that viewport so keyboard reading can continue.
The enclosing transcript ends a conflicting tail follow when it would hide new
reading focus. Automatic output tail positioning preserves the current focus.
Authored result bytes, HTML and URLs render as text. The client preserves generic
inspection during loading, failure, connection retirement and withdrawal. Opening
history never downloads or executes the old producer's JavaScript or replays a Tool.

The component is trusted workbench code using shared React, the public SDK and UI
controls/provider context. It uses the common exact-release resource lease and logical
component lifetime; replacement may require a window reload. The HTML guest, channel,
source, resource and dedicated builder are removed. Rebuild through
`node plugins/scripts/build-components.mjs`; verify with the same command's `--check` option.
Do not edit generated namespace-local `components/component.js`.
