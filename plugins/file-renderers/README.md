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
