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

Marked parses Markdown tokens. React renders semantic elements with text content;
authored HTML, MDX expressions, images and URLs remain text. Code fences remain
readable code. This is ordinary trusted workbench code, using shared React, the public
SDK and `@flame/ui`. Marked is a declared private dependency bundled in the ESM;
its license is retained in the namespace resources.

The preview describes embedded images using their alternative text and a short
source caption. Complete payloads remain intact in the host Source view.
Ordinary image URLs remain visible, inert source text.
Links retain their formatted labels and native title annotations without becoming
interactive. A destination is shown only when non-empty and different from the
visible label.

The host file header owns file identity, range metadata, Source mode and
navigation. The preview body is reserved for authored document content within
the shared content measure.
Code fences retain their captured language info as a text caption outside the
literal source. Code, inline code and tables consume the same shared reading
roles as the transcript while file parsing and content trust stay here.
Parsed table column alignment is projected on both header and body cells.
Paragraphs, headings, nested lists, quotes and rules use the shared `data-prose`
grammar. Prose direction follows its content; code remains left to right.
Task tokens supply one immutable `TaskListItem` marker with the primary visible
task name and captured checked state. Parser checkbox tokens do not become a
second text prefix; ordinary checkbox notation inside code stays literal.
Ordered lists retain their authored starting number and ordinary items.

Build shared UI dependencies and rebuild or check the component from the repository:

```sh
node plugins/build-components.mjs
node plugins/build-components.mjs --check
```

The source owns the generated namespace-local `components/component.js`. The three
extension declarations share that module and export. Do not edit generated resources.
The exact release resource lease and client generation are the same as other installed
components. Code replacement may require a window reload; logical disposal does not
claim JavaScript module unloading. Appearance remains owned by shared UI and the client.
