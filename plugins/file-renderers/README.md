# File renderers

An optional first-party Markdown component for complete Runtime file reads. Install
this directory and enable it through the local application. It declares `md`,
`markdown` and `mdx` extension selectors with file scope and `fileRenderer` placement.
Equally ranked matches remain ambiguous and retain the source view.

Runtime owns file access. The client supplies `context.file`, a frozen projection of
one accepted read: workspace, requested path, content, start line, total lines and
truncation. The component does not read another file or follow current navigation.
Partial and truncated reads use the existing source view before loading the module;
line navigation starts in source mode. Loading, render failure, connection retirement
and disablement preserve the recorded source.

The shared remark pipeline parses Markdown. React renders semantic elements with text content;
authored HTML, MDX expressions, images and URLs remain text. Code fences remain
readable code. This is ordinary trusted workbench code, using shared React, the public
SDK and `@flame/ui`. The declared remark and react-markdown dependencies are bundled
in the ESM; their licenses are retained in `package/THIRD_PARTY_NOTICES.txt`.

Marked parses Markdown tokens. The adapter decodes character references once from
ordinary token source with `entities`; code and raw HTML keep their literal text.
React renders semantic elements with text content;
authored HTML, MDX expressions, images and URLs remain text. Code fences remain
readable code. This is ordinary trusted workbench code, using shared React, the public
SDK and `@flame/ui`. Marked and entities are declared private dependencies bundled
in the ESM; their licenses are retained in the namespace resources.

The preview describes each image with the shared `ImageNotice`: its alternative
text and a short source caption, never a loaded image. Complete payloads remain
intact in the host Source view. Ordinary image URLs remain visible, inert source
text in the caption.
Links retain their formatted labels and native title annotations without becoming
interactive. A destination is shown only when non-empty and different from the
visible label, in the shared muted `link-destination` role.

The host file header owns file identity, range metadata, Source mode and
navigation. The preview body is reserved for authored document content within
the shared content measure.
Code fences render through the shared `CodeBlock`: the captured language info is
a caption outside the literal source, copy writes exactly the captured text, and
wrapping follows the same page-wide preference as transcript code. Syntax color
comes from the host's code highlighter; it paints escaped text and never runs or
loads authored code, and the literal stays visible until it is ready. Code, inline code and tables consume the same shared reading
roles as the transcript while file parsing and content trust stay here.
Code action names and accepted-copy feedback use the same component locale as
the named reading region. Captured fence language, metadata and source remain
literal; shared UI retains wrapping and selection ownership.
Parsed table column alignment is projected on both header and body cells.
Paragraphs, headings, nested lists, quotes and rules use the shared `data-prose`
grammar. A quote that opens with a GitHub alert marker (`[!NOTE]`, `[!TIP]`,
`[!IMPORTANT]`, `[!WARNING]`, `[!CAUTION]`) renders as the shared `Callout`, read
by the same `readCalloutMarker` the transcript uses; other quotes and markers stay
literal. Prose direction follows its content; code remains left to right.
Parsed task items supply one immutable `TaskListItem` marker with the primary visible
task name and captured checked state. Parser checkboxes do not become a
second text prefix; ordinary checkbox notation inside code stays literal.
Ordered lists retain their authored starting number and ordinary items.

Build shared UI dependencies and rebuild or check the component from the repository:

```sh
node plugins/scripts/build-components.mjs
node plugins/scripts/build-components.mjs --check
```

The source owns the generated namespace-local `components/component.js`. The three
extension declarations share that module and export. Do not edit generated resources.
The exact release resource lease and client generation are the same as other installed
components. Code replacement may require a window reload; logical disposal does not
claim JavaScript module unloading. Appearance remains owned by shared UI and the client.
