# File renderers

An optional first-party Markdown renderer for complete Runtime file reads. Install this
directory and enable it through the local application.
It supplies `md`, `markdown` and `mdx` file-extension declarations. Equally ranked matches
remain ambiguous and keep the host's source view.

The trusted host reads the file and captures `flame.file.v1`: its Runtime workspace,
requested path, content, start line, total lines and truncation marker. The page cannot
choose another file or invoke a tool. Refresh reauthorizes its renderer and repeats the
same captured read. Truncated and partial reads retain source presentation; line navigation
starts in source mode. Disablement or failure leaves the recorded source available.

Marked parses Markdown into tokens. The page builds a closed DOM with text nodes; authored
HTML, MDX expressions, image embeds and URLs remain text. Code fences remain readable code.
Shared controls are rendered to static markup at build time from `@flame/ui`.
It shares no React instance or host imports at runtime. The generated resource is self-contained,
including its parser and license, and uses the qualified read-only carrier.

After installing the graphical client's locked dependencies, rebuild or check the resource:

```sh
node plugins/file-renderers/build.mjs
node plugins/file-renderers/build.mjs --check
```

The source and build script own the resource. Do not edit generated `renderers/markdown.html`.

Appearance comes from the Host's existing read-only projection. Shared styles,
controls, font scale and focus rules have one owner in `@flame/ui`; Markdown
content structure remains with this renderer. Theme changes never reread the file.
