# Shared Web product

This React application runs in Browser and Wails. Runtime owns durable business
semantics; Web owns inputs, view projections, navigation and presentation.
[ARCHITECTURE.md](ARCHITECTURE.md) records feature, extension and lifetime owners.
The [native host](../desktop/README.md) supplies macOS capabilities without owning
another UI application.

From the repository root:

```sh
make setup
make dev-web
npm run check --workspace @flame/web
```

Root npm workspaces and lock select all dependencies. Shared packages must be
built by `make setup`/`make generate`; do not create a package-local install or
lock. `make stage-web` builds the same production output used by native embed.

`src/app` composes product policy. `src/features` contains actual feature owners
and their formal public capabilities. `src/extensions` contains generic host,
registration and lifetime mechanisms; it never owns business extension points.
`src/platform` translates Browser/Wails effects. `src/ui` contains product
presentation composites over shared `@flame/ui` controls and tokens.

The composition root creates each feature instance and its backing state once.
React consumers use that owner's Provider; commands and plugin factories capture
explicit dependencies. Tests construct independent instances in `test/renderClient.tsx`,
and visual fixtures compose them in `visual/client.tsx`. No default feature-instance
registry or module-global port installer is part of the product API.

Appearance settings use section headings and direct label/control rows. Existing
control labels, selected values and error feedback carry the task; the page does
not repeat explanatory introductions or one-line descriptions. Section headings
remain settings search terms through their existing catalog keys.

`SettingRow` marks the label and controls as one shared focus unit. `Panel` owns
revealing that complete unit inside the clipped host when it fits; taller rows
retain the focused control as the reading target.
Pointer activation keeps the control at its pressed position; keyboard and
programmatic focus retain the complete reading context.

The existing appearance preference owns optional Xuan paper, cotton paper and
fine-grain backgrounds with a retained strength from 1 to 5. The document painter
projects a static, seamless SVG image through `--surface-texture`; shell, reading
and settings planes consume it behind their content. Paper is off by default,
keeps the theme's page and pane colors, and leaves control, menu and card fills intact.
Palette themes include the material's opacity bound in their existing text and
focus contrast calculations. Forced colors suppress the image. No texture layer
intercepts input or selection.

Palette-derived semantic text accounts for its translucent feedback wash on every
reading plane, including paper bounds. The same palette owner publishes the wash
opacity and computes its painted backgrounds; the shared sRGB compositor also
bounds paper. Composite channels stay fractional through the contrast check,
so rounding cannot turn a failing pair into a pass. Status fills, the selected
accent and paper artwork retain their existing values.

The workspace host retains mounted view layout when switching Dock tabs. Inactive
views are invisible, inert and excluded from the accessibility tree. The existing
viewport keeps its native reading position without a second scroll-state cache;
Session changes and view retirement retain their existing lifetime boundaries.

Background-task progress keeps complete names, messages and errors in one named
keyboard reader. The popup and its scrolling body fit the available viewport;
progress updates retain the same reader and native position. Task lifecycle
and automatic retirement remain with the producing task store; the existing readout
owns percentage clamping.

Goal stop-reason details use the shared named keyboard reader inside their existing
disclosure. Original line breaks and the viewport cap remain; Tab enters the
explanation, and keyboard scrolling can reach every recorded requirement.
The retained disclosure body preserves native reading position through close and
reopen; the primitive excludes it from the closed keyboard and accessible scope.

Recorded Tool failures retain their original line breaks, blank lines and indentation
in both standalone and grouped rows, with long values wrapping inside the reading
column. Captured text, status, disclosure, actions and source-return ownership stay
with their existing owners.

The generic Tool inspector preserves whitespace-only results and argument text as
literal source, including their reading and copy controls. Only missing or
zero-length result bodies use its existing empty-result notice.

Recorded Skill resources and stored-output reads retain their complete captured
results. Only file reads unwrap the file content and line-range envelope; the
shared read category does not imply one result format. Skill pages retain their
continuation beside content, and historical text remains literal. Stored-output
summaries identify the Runtime's result ID, while its original byte-window header
remains in the body.

Recorded Tool arguments retain the complete accepted object independently of the
compact summary. Live starts leave raw text accumulation to argument deltas;
history and settled Items project their captured arguments directly. Rejected
argument text remains literal. Result extensions replace only the result body,
so the host's secondary Arguments disclosure survives enhanced presentation,
failure and withdrawal without losing its open state. Copy arguments uses the
same complete capture; no Tool is replayed to inspect it.

Selected subagent readers keep the complete task purpose and sibling identity
visible in their header. Overview rows retain their compact summaries; retained
reading state, source navigation and return focus stay with the inspection owner.

Conversation Find follows rendered message changes while its query is open.
Refreshes retain the selected occurrence in its message, rebuild current highlight
ranges and preserve focus and reading position. Only query changes and explicit
navigation request a match reveal; hiding or closing retires the observer with
the existing search lifetime. The Find scope owns Escape and query navigation;
zero-result navigation buttons keep focus while refusing activation.

Image galleries keep the complete captured description in a named, keyboard
scroll region. The header takes its natural height, and long descriptions stay
bounded while the existing image viewport fits the remaining space. Selection
keeps that description reader and its focus; source return and image zoom retain
their existing owners. Save failures lead with the command reason, followed by
the captured image description, so changing the current image cannot misidentify
the failed save.

Generated SVG fences stay inert images inside their code preview and open through
the shared image gallery. The gallery keeps source order within the reply, owns
zoom/pan and source return, and captures the image set for each opening. Code
copying retains the complete original SVG text.
Transcript viewports reveal returned keyboard/programmatic focus against the current
Composer clearance after window or page-zoom changes. Pointer focus keeps its
pressed position; modal completion continues to own focus restoration.
Reveals use absolute instant offsets so WebKit's painted reading position agrees
with layout after native page zoom.

Product object menus reuse their target owner's displayed identity: view and
session titles, message authors, and complete entry paths for files. Nested
submenus keep their primitive's relationship to the controlling item instead of
receiving a separate name.

File previews keep their filename to two visible lines beside persistent navigation,
range and commands. The complete path remains selectable in the same named
scroller as source, rendered content and read/unsupported states. File requests,
line placement, renderer mode and source-return history retain their existing
owners.

The focused file-source reader scopes Ctrl/Cmd+A to its presented source and
copies that complete range from the current accepted content. Source cells carry
literal line separators so native partial selection retains blank lines and
indentation. Rendered previews and editable controls retain native commands;
loading more rows or refreshing does not create a second selection owner.

Plugin provider roles remain available from installation contributions and accepted
selection facts when their independent selection read fails. Until a selection read
succeeds, each role stays unread; a successful empty read confirms it is unselected.
A failed refresh identifies retained selections as the last read. Candidate drafts,
explicit selection and command recovery keep their existing owners.

Loaded client-plugin errors show their complete message before source/time metadata.
Long traces use the shared named keyboard reader; each Copy action describes its
error and reports accepted or failed writes through the existing clipboard owner.
Clearing a plugin's transient errors returns focus to the loaded-plugin list,
including when an unloaded plugin's row disappears.

MCP server rows read their own tools directly, independent of configuration
editing and server origin. Reconnect runs in the same exact-ID serial mutation
generation as other server commands; its acknowledgement only admits a
background dial, so the row shows canonical server status after the reread.
Editors use direct field labels and examples without repeating the list's empty
prompt. Required attributes follow the active transport. Timeout errors project
the existing draft owner's parsed value: blank remains unbounded and a bound
requires positive safe whole seconds. Inline feedback names that field and clears
when corrected. List loading/errors and Retry remain visible during an Add draft;
connection readiness continues to come from the server row's canonical status.
Test and save feedback share the action group. New feedback is revealed with its
focused action when the group fits, without moving focus or returning to a group
the user already left.
Configuration identifiers, commands, paths and connection values explicitly opt
out of native capitalization, autocorrection and spelling changes at their fields.
JSON import uses the labeled format example without a repeated introduction and
applies the same literal-input policy. Only parsing errors mark source invalid;
command failures retain their progress and alert without invalidating valid JSON.
Editing the source retires its previous submission feedback while preserving the
field, text and focus.
The import parser preflights every entry's known source requirements before any
create command, including the active transport's command or URL. Typed source
issues are localized at the UI with literal source names. Timeout bounds remain
with the existing factory, and unexpected failures propagate. Runtime still owns
creation and final validation; external failures can report a committed prefix.

Tests stay beside their owners. Cross-language Runtime/schema/SDK tests live in
`tests/integration`; repository Go cross-product checks live in
`../../tooling/integration`. Visual fixtures live in `visual`. The full check includes
compiler-resolved dependency/contract guards, lifecycle tests, resource drift,
styles, localization and the production bundle.
