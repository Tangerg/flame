# Tool results

An optional first-party package for recorded Tool-result presentation. Install and
enable it through the plugin manager or CLI. It has no backend and executes no Tools.

Ordinary components declare `toolResult` scope, `toolResultRenderer` placement and an
exact display-name or known result-shape selector. Result-shape declarations use
`{ "type": "resultShape", "shape": "search" }`; their vocabulary belongs to
`extension.ResultShape` in the Go SDK and is generated for clients. Tool-name and
file-extension selectors retain their independent `name` values. One exact-name match precedes one
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
demand beneath the caption, above one bounded scrolling body. Changing or clearing
the row filter reveals the first matching row without moving input focus; unchanged
queries and ordinary rerenders preserve manual reading position. A web fetch names its
format in the caption. Paged results retain the shown and matching row counts in
that caption. Continuing a page reveals its first new row and returns focus to the
named result viewport, including the last page, instead of carrying keyboard focus
past the new evidence or dropping it when the continuation button disappears.
Output search identifies the current matching line with the
shared search treatment; Enter and Shift+Enter cycle matching lines. Every query or
navigation request reveals the selected line, including a sole match after manual
scrolling and a refined query that still selects the same line. Manual scrolling
alone keeps the current reading position. Clear empties
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
An empty recorded hit or result array states that no results were returned, before,
during and after Find; only a local filter that hides recorded rows offers the Close
find recovery, and clearing or closing it restores every row. An empty applied-change
receipt states that no changes were recorded instead of an empty table; it implies
neither failure nor a write.
Known literal answers, instructions and source results are classified by Tool
identity before inspecting normalized result shapes. JSON-looking text stays
complete rather than being mistaken for a different Tool's result envelope.
Current Skill pages separate summaries or instruction/resource content from their
recorded continuation. A nonfinal page keeps a More pages cue in the caption,
including during Find, and the complete next_cursor as source metadata. Copy
result retains the exact original page; reader selection copies the presented
content. Empty complete catalogs remain empty during Find, and literal
instructions remain literal. Unknown formats, unknown members and malformed page
shapes retain the complete source instead of silently dropping fields. This component never fetches
a page, concatenates separate Tool calls or treats a captured page as the full
Skill catalog or document.
Recorded `web_fetch` and `web_browse` pages retain their URL, snapshot identity,
Unicode range and continuation offset. A nonfinal page keeps the More pages cue
during Find. The renderer never retrieves or joins pages. Unknown members and
inconsistent ranges preserve the complete original source instead of hiding
metadata or presenting a partial capture as a complete document.
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
