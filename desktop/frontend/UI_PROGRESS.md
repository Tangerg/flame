# Desktop UI progress and handoff

Updated: 2026-10-20. The resumed Memory task review started from clean local and remote `main` at `cd22229a5`. The approved brand, Composer, CLI and earlier reading/Context Dock work remain integrated; Git history retains their chronology.

This user-requested file is the maintained progress and handoff record for the desktop UI work. Update its sections in place as the product changes. Git history retains the chronology; keep progress organized by product area rather than numbered rounds or additional progress files.

## Current position

The latest objective explicitly resumes continuous desktop UI work. Complete each evidence-backed batch with native acceptance, the relevant gates, an explicit-path commit and push to `main`, cleanup and an update to this file before selecting the next batch. Memory authoring and operation feedback are now verified at Actual Size in the Dock and main content view. Independent approval and pin commands remain available while a draft is open. The next observed problem is cross-page native Zoom In cropping the workspace; it is recorded below as a failed check, with its owner still to be established. Do not claim zoom acceptance from the passing font-size and narrow-window scenes.

Task-oriented layout and interaction apply across the conversation, Context Dock, settings and installed views. Preserve the approved brand, shared reading hierarchy, Composer, code/table/selection fixes, sidebar-matching Dock seam and custom-accent theme owner. The earlier `cap`-based inline-icon sizing tip is a suggestion for future typography work, not a global icon redesign.

Recent integrated milestones are `c59e7f470` (scope-first Diff and Usage), `863e4b62f` (Skills catalog/document/return), `565c3eb08` (literal search and local keyboard ownership) and `01af05b5f` (the reported Tab-item Default/Soft overlap). Their behavior and evidence are recorded by product area below. Continue from those owners; do not rebuild them from an older chat.

Schedules now uses a bounded Add/Edit dialog and a separate deletion decision over the retained catalog. The existing command, captured revision, provider/model catalogs and original-key reconciliation remain authoritative. Focus retirement accounts for catalog reads temporarily disabling record actions, while Add/Templates remain available. Actual Wails acceptance qualified both the shared modal stacking repair and persistent command feedback after scrolling the fields. Detailed coverage and its limits are below.

The checkout is `/Users/tangerg/Desktop/flame`, on `main`. All former local and remote branch tips were checked for ancestry before consolidation; their commits are retained in `main`, and only `main` remains locally and on `origin` at this checkpoint. Read current Git state before resuming; this document cannot overwrite newer work. Historical paused or blocked states in other chats do not describe the current checkout.

## Integrated brand and authoring work

The previously protected **76 modified, deleted or untracked files** have now been committed at the user's request and verified together in an isolated checkout. The original working tree is clean at this checkpoint. These features are implemented; do not recreate them from the older handoff inventory.

The user has approved the brand direction. Its production assets, settings pane, translations and source artwork are committed:

- `/Users/tangerg/Desktop/flame/brand` contains the primary mark, dimensional companion, plush character and usage guidance.
- `/Users/tangerg/Desktop/flame/design/branding` contains concept and refinement assets.
- `/Users/tangerg/Desktop/flame/desktop/frontend/src/lib/brand.ts` and `/Users/tangerg/Desktop/flame/desktop/frontend/src/lib/brand.stylex.ts` connect the assets and brand color.
- `/Users/tangerg/Desktop/flame/desktop/frontend/src/plugins/builtin/settings/brand` and `/Users/tangerg/Desktop/flame/desktop/frontend/visual/brandSettings.browser.spec.ts` belong to the brand work.

Composer submission captures the exact immutable draft and Session at the authoring owner. Slash handlers explicitly accept input or admit a send; returning from a handler alone does not clear the draft. Accepted input records history in its captured Session, consumes only the submitted revision, preserves later text and attachment edits, and avoids stealing another control's focus. Replacing Runtime storage retires old callbacks. Goal reports acceptance through this same owner rather than comparing text or clearing independently.

The implementation lives under `/Users/tangerg/Desktop/flame/desktop/frontend/src/plugins/builtin/chat/composer` and `/Users/tangerg/Desktop/flame/desktop/frontend/src/plugins/builtin/chat/goal`, with updated shell and SDK consumers. Focus is owned by the installed Composer state port; the former `application/focus.ts` was intentionally removed. Do not restore the retired clear, text-comparison or global-focus paths.

CLI slash commands preserve the editor, attachments and cursor when command lookup, availability or arguments are rejected. Contributed commands accept only after their cancellable Session operation starts. Approval normalization retains an allow or deny decision when remembered scope becomes unavailable; unknown choices cannot create an answerable form.

Start each new task with the actual status and relevant diffs. Preserve any newer unrelated edits, avoid bulk formatting or staging, and do not treat the old unfinished-file inventory as current work.

## Reference products and example images

Deeply study the two desktop references before choosing the next implementation. Read their actual layout, component and interaction code, and inspect visual evidence. Naming a reference or borrowing a few colors is insufficient.

| Reference                 | Where to look                                                                                                                                                                                                                           | What to compare                                                                                                                                                                                          |
| ------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ZCode                     | `/Users/tangerg/Desktop/ZCode/packages/ui/src/app-shell`, `/Users/tangerg/Desktop/ZCode/packages/ui/src/ToolCallBlocks`, `/Users/tangerg/Desktop/ZCode/packages/ui/src/v4`, and `/Users/tangerg/Desktop/ZCode/packages/ui/src/settings` | Workspace proportions, reading rhythm, side-pane behavior, resizing, tool summary/detail transitions, contextual actions and settings composition.                                                       |
| ChatGPT desktop reference | `/Users/tangerg/Desktop/study/chatgpt/deob/webview` and `/Users/tangerg/Desktop/study/chatgpt/extracted`                                                                                                                                | Conversation composition, typography, spacing, progressive disclosure, contextual surfaces, focus and keyboard behavior. This is an extracted reference; establish what each source or asset represents. |
| HeroUI                    | `/Users/tangerg/Desktop/heroui`                                                                                                                                                                                                         | Shared control surfaces, optical alignment, state treatment and semantic color roles, adapted to desktop density.                                                                                        |

The user also explicitly requested the Ace Design examples:

- [Alpaca](https://acedesign.io/work/alpaca). Cached overview: `/tmp/flame-alpaca-reference-8f0lz7l0/contact-sheet.jpg`.
- [Marketer](https://acedesign.io/work/marketer). Cached overview: `/tmp/flame-marketer-reference-3epzp5i1/contact-sheet.jpg`.

Both cache directories existed at handoff and contain the full PNGs, a manifest and the source page. Open the overview and relevant full-size images. Compare proportions, grouping, whitespace and information hierarchy; if a temporary cache disappears, revisit the source page. Reference repositories and artwork remain read-only material.

This continuation inspected ZCode's actual workspace layout, resizable side pane,
subagent detail, tool summary/layout and Markdown list implementations. It also
inspected ChatGPT's local conversation/subagent pane code, extracted typography,
wide-block and focus styles, and actual visual assets. The Markdown export module
was identified as an exporter, not mistaken for the conversation renderer. Both
Ace overviews and relevant full-size PNGs were opened. The useful common pattern
is a stable reading axis, quiet summaries, and a persistent identity and return
route above detailed material. The current changes apply that pattern to Flame's
existing owners rather than copying reference colors or adding a new shell.

The Diff review additionally traced ZCode's `MessageChangeSummaryPanel.tsx`,
`messageChangeSummaryPreview.ts` and `components/ui/diff-viewer.tsx`: a message's
summary and file-specific review are distinct from a live workspace comparison.
ChatGPT's `thread-side-panel-tab-content-2b3cd260bf24.js`,
`editor-diff-page-7ae498a2f29c.js` and `review-file-tree-side-pane-3e6c9e54ce58.js`
separate review scope, file navigation and code; they retain continuous small
reviews and switch large reviews to one file with previous/next navigation.
Flame adapts the latter navigation to its constrained Dock instead of copying
unsupported staging, reversion, comments or last-turn semantics.

Skills review traced ZCode's actual `settings/SkillsSection.tsx` and ChatGPT's
`details-73eaf369a9c5.js`. Both retain the directory beneath a named document and
make failed document reads distinct from an empty catalog. Flame keeps that
continuity within its persistent Dock rather than adding another dialog.

Search review traced ZCode's `settings/SettingsSearchInput.tsx` for named local
clear actions and ChatGPT's
`keyboard-shortcuts-search-input-3e66fb0894a8.js` for caller-first keyboard handling
and nonempty-query Escape before outer navigation. Flame applies those priorities
to its shared controlled field, with composition protection at the field and host.

Schedules review traced ZCode's `settings/AutomationEditView.tsx` and
`AutomationsSection.tsx`: editing has its own named task context and submission
actions, while deletion is a separate confirmation. ChatGPT's
`automation-dialog-cb9041625c72.js` retains explicit form submission and local
autocomplete ownership; `automation-delete-confirmation-dialog-ac93171503ee.js`
names the affected task and pairs cancellation with the destructive command.
Flame's constrained Dock needs a bounded editor with persistent actions rather
than a long inserted form that displaces the retained catalog.

Memory review traced ZCode's `settings/MemorySettingsViewer.tsx` for explicit
scope, literal search and record actions, and ChatGPT's actual
`saved-memory-history-83b91672ef74.js` dialog for selected-object identity and
separate consequential decisions. This is API-backed UI code, not the Markdown
exporter. Flame retains its existing non-modal draft because independent approval
and pin/unpin commands remain useful during authoring. The [Radix Dialog model](https://www.radix-ui.com/primitives/docs/components/dialog)
would make that catalog inert; its boundary is deliberately not applied here.
HeroUI's local disclosure/form examples and the [Disclosure documentation](https://heroui.com/en/docs/react/components/disclosure)
guide grouping and state cues. The Ace overviews plus full-size Alpaca connection
and Marketer activity scenes were inspected for task grouping and reading rhythm;
they supply no keyboard evidence. Existing semantic text/surface/focus roles
remain authoritative, consistent with [HeroUI color roles](https://heroui.com/en/docs/react/getting-started/colors)
and the [Radix scale use cases](https://www.radix-ui.com/colors/docs/palette-composition/understanding-the-scale).

For color-system work, retain the user's [Radix Colors](https://www.radix-ui.com/colors) and [HeroUI colors](https://heroui.com/en/docs/react/getting-started/colors) references. The default brand color is **Flame Orange `#FB6E30`**; users can choose their own interface accent through the existing theme owner. Approved brand artwork retains its original color. Avoid recreating the palette, appearance store or brand identity.

The older review at `/Users/tangerg/Desktop/flame/desktop/docs/Flame-Desktop-UI-Review-ZCode-2026-09-23.md` contains historical observations and reference pointers. Revalidate its findings against current code and behavior before using any item as a new task.

## Implemented foundation to retain

| Product area                  | Current behavior and ownership                                                                                                                                                                                                                                                                                                                                 |
| ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Shared presentation           | Panels, persistent controls, resource groups, disclosures, forms, modal geometry, metric summaries and semantic badges share presentation owners. The host owns appearance values; `/Users/tangerg/Desktop/flame/ui` owns reusable presentation rules. Pages compose those rules.                                                                              |
| Color and brand               | The default accent is Flame Orange. Content headings, references, list markers, quote rules and inline code use semantic accent roles while body text keeps readable neutral ink. Theme customization and the approved brand assets already exist.                                                                                                             |
| Prose and layout              | Prose takes one reading step above interface text: 16 px with the default 14 px interface, while code keeps its separate preference. Paragraphs use one gap; headings have unitless leading and a visible following gap, including deeper levels. The `65ch` cap and wider code/table/media measures remain intentional.                                       |
| Markdown tables               | Long tables stay within a bounded reading viewport. Compact table actions follow the table edge and wrap when necessary. Expanded previews keep controls and column headings reachable. Do not reintroduce page overflow or remove access to long data.                                                                                                        |
| Code and recorded output      | One stable bounded code-reading region retains literal selection, scroll and focus as syntax highlighting arrives. Shared captions, wrapping and copy behavior are established. Loading messages no longer expose raw Markdown source as a temporary presentation.                                                                                             |
| Selection and file references | Ordinary action-button drags cannot select the surrounding application. Reading text and inline file references remain selectable. Dragging a file reference preserves selection without navigating the Dock; ordinary clicks and Enter/Space still open its path and line. Native copy and whitespace dismissal work.                                         |
| Tool rows                     | Rows reserve action space only when real actions exist. Recorded paths wrap at directory boundaries and retain literal copy. Expanded groups remain visible and clickable as the Dock narrows; the WebKit grid sizing repair preserves reversible transitions.                                                                                                 |
| Context Dock                  | File identity and return navigation precede a separate wrapping mode/action row. Full directories remain available beneath the filename. Opening from the tree focuses the return action; returning restores the tree cursor without stealing another region's focus. Panels grow through their complete content; fixed pane seams paint above child surfaces. |
| Subagent stop controls        | Shared filled stop marks and quiet circular controls replace the checkbox-like outline squares. Their placement is consistent across the narrative, Dock list and detail view; terminal entries reserve no dead cancel slot.                                                                                                                                   |
| Main and delegated messages   | Both use `MessageTurn` and `MessageBlock` for spacing, prose, bubbles, long-input folding, tools, code, tables, media and message actions. Child rows retain their own Run and referenced facts. Their overview keeps its scroll position; returning restores the originating task's focus.                                                                    |
| Motion and focus              | Shared moving item backplates remain interruptible and reversible and consume the host motion tokens. Reduced motion, scoped code Select All, native dismissal and focus return have established owners and regression coverage.                                                                                                                               |
| Authoring and acceptance      | One captured submission owns draft consumption and history. Rejected input stays editable; delayed acceptance preserves successor revisions, Session changes, attachment staging and other controls' focus. Goal and slash extensions share this settlement contract.                                                                                          |

File-reference gesture behavior lives in `/Users/tangerg/Desktop/flame/desktop/frontend/src/plugins/builtin/chat/file-references/public/FileRefLink.tsx`: pointer activation consults the browser's existing selection; keyboard activation remains available. It introduces no separate selection state. Read current callers and coverage before changing this boundary.

Activity disclosures now retain their quiet chevron before hover, making the
summary-to-evidence route discoverable. Their existing toggle, rotation and
reduced-motion owners remain in use.

The native scrolling background defect came from shared `Panel` roots shrinking
inside a column flex viewport: long children overflowed below the panel's canvas
and sticky-control boundary. `Panel` now opts out of that shrinking. The weak Dock
separation had a different cause: the inset seam painted beneath opaque child
reading surfaces. The conversation plane's fixed edge now paints the same
full-plane inset wash as the left sidebar edge, without a persistent
dark stroke or additional hover/drag darkening. Both edges share one wash rule;
the resize cursor and keyboard focus treatment remain available. Do not cover
either defect with per-view background patches or duplicate dividers.

The selected-subagent view no longer assembles a parallel message UI. Agent's
existing fact slicer projects each child message for the common renderer, so
running siblings do not suppress a finished child's actions. The same streaming
preference applies in both narratives. Copy and feedback retain the selected
message identity; regeneration appears only for a main-conversation message that
the existing replay operation can locate. Main and child scroll following remain
independent, while sharing the reading-focus guard and latest-content control.
Jumping to the bottom transfers focus to that reader before hiding the control.
The running-status pill keeps one line and grows with text size instead of
wrapping outside a fixed-height background under narrow-window pressure.

Tool groups and narrative waves now use the same `ToolActivitySummary` in the
shared message renderer. Their compact secondary line retains ongoing calls,
settled problems and refusals when folded beside the later answer. A nonzero
command exit and an error on the same call count once; provisional exit values
on running calls remain in the running
count. Refusals remain distinct from execution failures. Individual failure
marks use the Runtime status independently of optional explanatory text. The
existing automatic expansion and user-pinning rules remain in force.

The portable tool-result renderer now keeps one text control for output expansion
and collapse. Explicit expansion enters its bounded reader after positioning,
while collapse keeps the original control focused; a later user focus choice
still takes precedence. Long recorded results show their current row count in the
shared caption. Continuing a page reveals its first new row and focuses the
existing named result viewport, including the final page where the continuation
button disappears. Main and delegated messages consume this same renderer.
Sticky activity summaries now publish their measured height to the reading body,
using the established disclosure-clearance property. Keyboard targets reserve
that space when revealed, so output controls remain below a wrapped summary at
large text sizes. The shared activity component owns measurement and retires its
observer and scoped value with the component.

File references, tool material actions and the header worktree summary now
capture their source as an explicit inspection intent in Workspace navigation.
The Dock receives focus without stealing a later user choice, and its shared
return action or unhandled Escape restores the originating view and anchor.
Returning from a file opened inside a child narrative selects that same
subagent pane and retains its reader state. The source anchor can be revealed
after a width change. Background file updates keep the established outside-focus
guard; file-to-tree navigation remains independent. The inspection record is
ephemeral, retired on Session changes or navigation-port reinstallation and excluded from storage.
An unchanged child reader no longer replays its initial Back-button focus when
React Activity resumes it; native acceptance exposed this ownership conflict.

Task links in the main and child narratives now enter that same inspection owner.
The currently selected task uses the existing selected surface, a persistent open
mark and `aria-current`; its name remains owned by the enclosing delegation group
rather than repeated on every sibling. A task opened from another task retains
that source reader through React Activity. Only readers on the active inspection
path remain mounted, and leaving the path releases them. Each explicit step keeps
its source selection and preceding inspection. Returning restores the source task
and control; returning to a file also restores the file path and line captured
before another file replaced it. A new source in the conversation starts a new
path. The path stays ephemeral and retires with its Session.

The Dock's existing Back action and unhandled Escape return one inspection step.
Returning to the conversation focuses its original task summary while leaving the
inspected material available for comparison. The task header's separate **All
subagents** command opens the retained index and ends the nested task trail; it
uses a list mark rather than implying another Back action. Overview-to-task entry
keeps its existing focus and scroll behavior. The selected-reader focus effect
recognizes an inspection return and leaves focus settlement to the shared owner;
otherwise it could override the restored source with the index command.

Diff now distinguishes the general workspace entry from a file-specific entry.
The former opens a searchable changed-file overview with text-change totals; the
latter opens its requested file. Rows navigate to one selected reader rather than
expanding every file into the report. Comparison scope precedes local filtering;
file identity, layout and previous/next controls remain available while reading.
Complete directories and rename provenance stay outside the sticky identity zone.
Binary, metadata-only, partial and absent-file states keep their canonical meaning.

Local Diff history records whether the reader came from its list or directly from
a narrative source. Escape from the list-owned reader returns to the retained
query, scroll and current row; Escape from a direct file entry remains with the
host's source-return owner. The independent Files command always opens the list.
A popup dismisses before either path unwinds. Refresh retains a still-present file;
an absent target returns to the list with a notice. Failed scope reads retain the
accepted file and identify the actual comparison beside the requested scope.
`DiffFileList`, `DiffFileHeader` and `DiffFilePath` share file presentation in `ui`;
portable state and Runtime reads remain with their existing owners.

Usage applies the same distinction between query scope and presentation: the
report period and refresh precede source search and grouping. A pending or failed
period change visibly names the accepted period in the persistent control zone.
Filtering still changes only displayed buckets, never the report totals.

Skills now separates three catalog tasks. Available rows use the shared
`RecordButton` to enter a named document directly; personal library rows show their
complete lifecycle information and Archive/Restore without a redundant disclosure;
proposal rows expand their actual evidence and retain the existing captured
confirmation. Escape cancels that local confirmation before page navigation and
returns focus to its action. Runtime command and uncertain-acknowledgement owners
remain unchanged.

Document navigation keeps the Available catalog, query and canonical refreshes
alive beneath the reader. Its own cancellable read and persistent name/compact
scope replace the catalog controls. Literal instructions use `DocumentBody` and
`RecordBody`; source path and revision are secondary disclosure. Back/Escape
restores the catalog position and source row, falling back to search if a canonical
read removed it. Background completion does not take a later focus choice. First
failures claim no retained data; same-document refresh failures identify the
accepted document. Document navigation does not recreate the Runtime subscription.
Diff file rows now consume the same `RecordButton` presentation without changing
their selection, comparison or return ownership.

The cross-page composition rule is now recorded in `DESIGN.md`: establish location
and data scope, show a useful summary, then expose the current object's content and
actions. Source review across the existing pages gives this task map:

| Page family                      | Task model to preserve                                                                                                                                                                                                            |
| -------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Conversation and Tool evidence   | Read the answer, scan outcomes, expand supporting evidence, inspect material and return to its source.                                                                                                                            |
| File, Diff and subagent readers  | Locate an object, read it persistently, navigate related objects and unwind the actual entry path. A directory command is separate from returning to a narrative source.                                                          |
| Settings and resource management | Identify the selected scope/object before editing; keep current state and the primary command visible, with bounded forms and explicit cancellation. Memory, Skills and Schedules retain their existing command and scope owners. |
| Usage reports                    | Select the data period before local grouping/filtering; identify retained results by their accepted period.                                                                                                                       |
| Trajectory and diagnostics       | Scan a linear record summary and expand supporting evidence/provenance without turning every record into a new navigation destination.                                                                                            |

These are task-specific compositions of shared controls, not a requirement to turn
all pages into the same card, accordion or modal. The source review does not claim
that every unchanged page has received fresh native acceptance.

The empty Dock now has a clear title, supporting context and a fixed search field
above its scrolling groups. The directory and header picker use one `CatalogList`
for filtering, icons, open marks and keyboard selection, including searching by a
localized group name. Missing contributed icons keep a stable fallback column.
The directory is retained within its Session so inspection and return preserve
the query, results position and originating search control. Picker selections
retire their popup before entering the selected material; canceling restores the
trigger. The selection marker remains through Base UI's final focus settlement
so the trigger cannot steal focus back from the material. Full-view-to-Dock entry
uses the same inspection owner and returns to its named main reader.

The shared document focus rule now suppresses the browser's native outline on
`data-chrome-focus` controls. Their existing field edge or row state owns focus;
ordinary keyboard targets retain the global semantic ring. Native WebKit exposed
a second blue outline inside the catalog search field during keyboard navigation;
the final native review confirmed a single field edge and the ordinary semantic
ring on the Dock return button.

`SearchField` owns literal query defaults and local Escape clearing. Nonempty
editable queries clear through the existing `onClear` callback and retain input
focus, including when Escape starts on the clear action. Empty queries leave
enclosing navigation available. Native-only fields preserve the browser's default
clear rather than synthesizing input events. Explicitly handled keys and blocked
mutation states retain their previous owners. Actual portable filters and shortcut
search now provide the explicit clear callback and name; Settings no longer
duplicates this keyboard rule.

`ui/src/keyboard.ts` owns the shared `isComposing`/legacy keyCode 229 predicate.
Dock return, Diff/Skills local navigation and the former frontend helper's consumers
use it; the retired `src/lib/ime.ts` is removed. The stable code reader uses the
same predicate without changing its scoped literal Select All behavior. Recorded
output Find remains a bounded task: its owner captures Escape to close back to the
output, while Enter/Shift+Enter cycle matches outside composition.

Segmented tracks use the existing compact navigation shape step. Their items and
moving backplate share the track radius minus its actual padding; there is no
track border to subtract. This leaves room for the radius preference to change
short Tab items visibly. The visual-style owner and its first-paint stylesheet
mirror publish the same mapping. Preset values, the shared shape ladder, the
curve feature query and explicitly round controls retain their previous owners.

Schedules keeps its catalog/query/page mounted beneath a bounded Add/Edit task
and a separate named deletion alert. Shared `ModalDialog` owns the header,
scrolling body, persistent footer, focus trap and nested popup priorities.
`ui/src/modal-dialog.tsx` now establishes a modal stacking context so shared
menus/selects remain above its panel. Its native `finalFocus` and `dismissible`
contracts avoid competing focus cleanup; composition Escape includes legacy
keyCode 229 protection.

`plugins/schedules/src/component.tsx` owns the captured return target and keeps
command progress, failure and original-key reconciliation in the persistent
action zone. Cancellation returns to the actual available record/Add/Templates
entry. An absent or read-disabled record falls back to retained search; accepted
edits/deletion also return there during the canonical refresh. Creation and
cancellation return to Add/Templates even while that read is pending. A later
refresh does not steal focus. Accepted revisions, model identity, Runtime
commands, uncertain settlement/retry and read cancellation retain their previous
owners. This choice does not turn other management forms into modals.

Memory authoring remains one local non-modal task over its retained catalog.
`plugins/memory/src/component.tsx` shows the captured scope and stable record ID
in edit/deletion tasks. Each original command captures its action, target and
editor/record origin. Draft progress, refusal and original-key reconciliation
stay beside form actions; independent record feedback stays with Saved memory
without erasing a draft's refusal. Confirmed form results move to that catalog
context, including when no page could be accepted. New forms clear prior form
feedback and changing scope retires the previous scope's notices.

Only the original promise settles the task. Its subsequent canonical read has a
separate lifetime/loading state; it cannot revoke a known result or prolong the
command guard. No optimistic memory record or new receipt owner is introduced.
Local Escape cancels the current task after literal search and nested choices
have handled their key; composition leaves it intact and an unresolved command
consumes it without discarding the task. `RecordForm` still owns focus retirement.
The Memory resolver rejects a disconnected or read-disabled opener and uses
retained search. A later read never reclaims a subsequent focus choice.

## Verification checkpoint

For Memory authoring and command feedback:

- `npx vitest run visual/memoryDrafts.test.tsx` passed all 22 tests. The baseline's existing 10 tests passed before implementation; new task/feedback/focus cases exposed the old behavior. The CLI supplied the initial regression draft but did not complete implementation; its incorrect named-section/selection assumptions were corrected against the real shared components. Coverage retains original mutation identity, same-key reconciliation and independent approval/unpin, and adds local/search-first/nested Escape, IME and legacy 229, pending/unknown blocking, named feedback ownership, read-disabled return targets, scope retirement, results without an accepted catalog and later-focus preservation after a delayed read. No valid assertion was weakened.
- Portable `npm run build` and `npm run check` passed, including all ten exact ESM resources. The final frontend `VITEST_MAX_WORKERS=2 npm run check` passed 378 files and 2784 tests, followed by all unused-code, boundary, style/token, locale, bootstrap, resource, bundle and production-module gates. Non-failing loopback `ECONNREFUSED` diagnostics were emitted during the full run; no product cause or repair is claimed. An introduced test-selector type error was corrected before this final run.
- The supported production Wails arm64 build passed; `vtool` confirmed macOS 13.0. Shared UI, Go, Runtime, CLI, SDK and dependencies have no source changes in this batch. Their module suites were not rerun for this presentation-only change; separate Runtime commits are not requalified here. The Memory ESM is rebuilt, but an existing Runtime installation still requires the normal explicit release selection to consume a new package; no live installation was changed.
- Actual Wails WKWebView first reproduced missing local cancellation and an edit refusal surviving Cancel into a fresh Add task. Final Chinese/light/14 px/Flame Orange and English/dark/18 px/custom-blue scenes checked record identity, literal query/return, keyboard reopen, search-first Escape, failed edit then fresh Add, independent record failure with a retained editable draft, nested menu priority, named deletion/cancellation and source focus. The native system window menu resized the app to approximately 1120 px; keyboard resizing reached a 320 px Dock. Wrapped IDs, mixed CJK/English, long workspace text, scrolling fields/feedback/actions and the unchanged main conversation stayed usable. The full-pane Memory placement also retained its query and record focus on local cancellation, and explicit Close returned to the conversation.
- **Failed:** after one native View → Zoom In in the narrow/dark/18 px scene, the whole workspace enlarged beyond the window and cropped the right edge, including unrelated chrome. Actual Size restored the layout. No zoom percentage or 200% pass is claimed. Establish the native/viewport/layout owner in the next batch before changing any leaf component.
- Native command refusals used the fixture's explicit unsupported-write error through the production component and public client in a real Wails host. Successful acknowledgements, unknown receipts, delayed/failed reads and IME variants are deterministic component/public-client regressions, not native failure injection or OS input-method coverage. VoiceOver and rendered pixel contrast measurements were not run. No browser/visual suite or performance benchmark was run.
- The owned native app and preview have stopped. The disposable bundle, binaries, fixture output, prompts/logs/data and bundle-qualified WebKit/cache directories are cleaned before delivery. No own preferences, HTTP storage or saved application state were present. Existing prerequisite outputs, user data, read-only references and unrelated processes are retained.

Memory review scope is the add/edit/delete task and its relationship to independent
catalog commands, using React, shared Base UI controls, StyleX and existing semantic
tokens. Repository, module, design and desktop-polish instructions were read.
Other management pages and live Runtime writes are outside this fresh review.

| Domain        | Evidence inspected                                                                                                 | Result                                                |
| ------------- | ------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------- |
| Accessibility | Panel local keyboard owner, shared SearchField/menus/RecordForm, 22 regressions and native focus paths             | Changed task paths verified; VoiceOver not verified   |
| Layout        | Shared Panel/form/record owners, native Dock at 320 px, actual narrow window and full-pane placement               | Actual Size verified; workspace Zoom In fails         |
| Writing       | Captured command labels, record/scope identity, refusal/recovery and new-task feedback retirement                  | Changed task copy verified against its actual action  |
| Typography    | Shared RecordMeta/RecordBody/Metadata, wrapped IDs and mixed-language text at 14/18 px                             | Actual Size verified; no font system changed          |
| Colors        | Existing text/surface/focus/negative roles, passing palette regressions and light/dark/custom-accent native scenes | Role use retained; rendered pixel ratios not measured |
| UI polish     | Existing surfaces, modal-free authoring, normal-motion controls, nested menus and independent scrolling            | Changed flow verified at Actual Size                  |

| Severity | Domain        | Location                               | Before                                                                                     | After                                                                          | Why                                                                                      |
| -------- | ------------- | -------------------------------------- | ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------- |
| MEDIUM   | Writing       | `plugins/memory/src/component.tsx:263` | One unlabeled message served independent record and draft commands and survived a new form | Captured action/target/origin feedback stays in its corresponding task context | A result must identify the operation it settles                                          |
| MEDIUM   | Accessibility | `plugins/memory/src/component.tsx:339` | Return attempted a read-disabled opener; editor Escape had no local owner                  | Available-opener/search resolver and composition-safe local cancellation       | Keyboard work must preserve a useful return point and the current task boundary          |
| HIGH     | Layout        | Native View → Zoom In, whole workspace | Enlarged content is clipped at the window's right edge                                     | Pending owner diagnosis in the next batch                                      | Critical controls must remain reachable under scaling; no Memory-only patch is justified |

Review verdict: **Block for workspace zoom**. The changed Memory task paths pass at
Actual Size; this is not approval of the unverified VoiceOver/pixel-contrast work
or of native zoom. Continue with the recorded cross-page failure.

For bounded Schedules editing and deletion:

- The final frontend `VITEST_MAX_WORKERS=2 npm run check` passed 378 files and 2772 tests, plus type, lint, formatting, unused-code, dependency/boundary, token/style, locale, bootstrap, resource, bundle and production-module gates. Non-failing loopback `ECONNREFUSED` diagnostics appeared again; their cause is unclassified and no product repair is claimed. An initial new test incorrectly did arithmetic on wire `JSONNumber`; its fixture now supplies the next canonical revision directly, without weakening revision assertions.
- Shared UI `npm run check` passed all 52 tests and its independently packed UI/icons consumer, including modal dismissal/final-focus and both IME variants. Portable build/check passed all ten exact resources. The 19 focused Schedules presentation/public-client regressions passed, including the two baseline failures, named local cancellation, nested Select priority, pending dismissal, rejected draft retention, acknowledged create/edit/delete, original-key reconciliation, removed/read-disabled record fallback and available Add return during a pending catalog read. No valid assertion was weakened.
- The supported production Wails macOS arm64 build passed; `vtool` confirmed deployment floor 13.0. No production Go, Runtime, CLI, SDK or dependency source changed; earlier checks for those unchanged sources remain applicable.
- The baseline Wails app placed Cancel/Save below the initial viewport and left both inline editing and deletion open on Escape. An intermediate native candidate exposed an expanded but obscured model menu; the shared modal portal now owns the correct local stacking context. A further native failure check exposed command feedback above the scrolled viewport; the existing persistent action zone now keeps the outcome and its next action with submission.
- Actual Wails WKWebView acceptance checked Chinese/light/14 px/Flame Orange and English/dark/18 px/custom blue, normal 1440 px and approximately 1120 px windows, ordinary motion and a pointer-resized 320 px Dock. The dialog keeps its task title and Cancel/Save visible while fields scroll. Mixed CJK/English title/instructions, the complete long workspace-path value, grouped timing/model controls and reachable keyboard focus were exercised.
- Native entry/return covered Add, existing-record Edit and a copied template. Cancel, close and local Escape restored the actual entry; the retained `nightly` query survived editing and deletion cancellation. A model popup was visible and keyboard-selectable; its first Escape returned to Model and the next cancelled the task. Deletion initially focused Cancel and returned to the originating record.
- Native writes used the existing isolated fixture's explicit unsupported-write failure, not a live backend command. That failure retained editable values, stayed visible in the fixed action zone after bottom-of-form submission and allowed cancellation back to the record. Starting a new task cleared the previous task's feedback. Acknowledgement success/refusal/uncertainty, removed-opener and IME variants are public-client/component regressions, not claimed native failure injection or OS input-method coverage. Schedules full-pane placement was not separately exercised.
- No browser/visual suite, pixel comparison or performance benchmark was run. Existing browser assertions were migrated to the intentional dialog/alert-dialog and keyboard-reachability contracts, but are not reported as passing. Production Go, Runtime, CLI, SDK and dependency sources are unchanged.
- The native app exited and its preview stopped. Its disposable data, app bundle, production/fixture binaries, overlays, logs, fixture output and owned WebKit/cache directories were removed. No owned preferences, saved application state or HTTP storage remained. Reference caches and unrelated resources were retained.

For the reported Tab-item corner overlap:

- Existing preference/document and stylesheet-mirror checks passed all 26 tests. The final frontend `VITEST_MAX_WORKERS=2 npm run check` passed 377 files and 2760 tests, plus type, lint, formatting, unused-code, boundary, style/token, locale, bootstrap, resource, bundle and production-module gates. All ten portable resources still reproduce exactly. No new test was added to restate CSS mappings; the actual geometry defect was reproduced and checked natively.
- The UI README formatting check passed. Shared UI component and portable sources are unchanged, so the preceding 50-test UI/packed-consumer and portable module results remain applicable rather than being reported as new runs. The supported production Wails arm64 build passed; `vtool` confirmed macOS 13.0. Go, Runtime, CLI, SDK and dependency sources are unchanged.
- Old-build Wails acceptance compared the same selected Density item under Default and Soft: its capsule stayed the same while adjacent input and card shapes changed. In the final English/light/14 px/Flame Orange scene, that item has a compact Default corner, a visibly softer Soft corner and a tighter Sharp corner. Keyboard switching retains the selected option and focus.
- Chinese/dark/18 px/custom-blue acceptance covered normal 1440 px and approximately 1120 px windows, with the fixture's ordinary motion enabled. The track and moving item settle with matching corners and Default/Soft remain distinct. A separate production file-reader scene entered README from the file list, retained readable prose/code/table composition, pointer-resized the Dock to 320 px and switched Preview/Source with Left/Right while keeping focus on the current tab.
- Acceptance used canonical isolated fixtures in actual Wails WKWebView, not a live user Runtime. No browser/visual suite, pixel-comparison test or performance benchmark was run. The owned application exited; its preview, disposable data, bundle, binaries, logs, fixture output and WebKit caches were removed. User references and unrelated resources remain intact.

For literal search and local keyboard tasks:

- Shared UI `npm run check` passed all 50 tests and the independently packed UI/icons consumer. Portable build/check passed all ten exact ESM resources. The 44 focused Dock, captured-result, Diff and Skills regressions passed. The two original failing Dock cases now pass; the existing assertions for disabled/read-only/pending mutation, callback-ref cleanup, literal selection and source returns remain intact.
- The final frontend `VITEST_MAX_WORKERS=2 npm run check` passed 377 test files and 2760 tests, followed by unused-code, boundary, style/token, locale, bootstrap, resource, bundle and production-module gates. The test run emitted non-failing loopback `ECONNREFUSED` diagnostics; no product cause or repair is claimed for that output. The supported production Wails arm64 build passed and `vtool` confirmed macOS 13.0. Go, Runtime, CLI, SDK and dependency sources are unchanged.
- In the old Wails build, entering `session` in an inspection's changed-file search and pressing Escape returned focus to the conversation's change-count entry, retained the query and changed it to `Session` through native correction. The final Chinese/light/14 px/Flame Orange build instead clears the query, keeps the input and return action, and returns to the actual source only on the next empty Escape. Blur preserves literal spelling. Tab reaches the named clear button; Escape there clears once and restores input focus.
- English/dark/18 px/custom blue acceptance included a normal 1440 px window, an approximately 1120 px window and a pointer-resized 320 px Dock. Complete long-path query values, clear controls, count summaries and wrapping file identities stayed available. Pointer clearing allows continued typing; entering one matching file and local Escape restores the retained query and current row. Settings search also cleared locally and kept focus without leaving Settings.
- Actual Wails output Find was checked against the existing 50,000-line captured-output scene. Enter advanced a match and Shift+Enter returned; Escape from either a nonempty query or its focused clear button closed Find and restored the named output reader. Composition variants are covered by unit regressions; no OS input method was changed and no native IME coverage is claimed. The canonical in-memory fixture and production components ran in a real Wails host, without a live user Runtime. No browser/visual suite or performance benchmark was run.
- The native app exited. Its owned preview, disposable data, bundle, binaries, logs, fixture output and WebKit caches were removed. Reference caches and unrelated resources remain intact.

For Skills catalog, document and proposal tasks:

- The final frontend `VITEST_MAX_WORKERS=2 npm run check` passed: 377 test files and 2754 tests, plus type, lint, formatting, unused-code, boundary, token/style, locale, bootstrap, resource, bundle and production-module checks. The 33 focused Skills/Diff/Usage/transport checks passed. New Skills regressions cover direct row entry, retained query/scroll/source focus, first-read and retained-read failures, cancelled/stale reads, row-removal fallback, local proposal cancellation and the original uncertain command identity/retry.
- Shared UI `npm run check` passed 38 tests and its independently packed UI/icons consumer. Portable build/check passed all ten exact resources; Skills ESM was rebuilt. `RecordButton` consolidates the actual Diff/Skills navigation-row presentation; the host and Runtime retain their previous navigation and business authority. A type error identified a now-impossible Lifecycle fact in the proposal disclosure after library rows became static; that obsolete fact was removed before the final gates.
- The supported production Wails macOS arm64 build passed; `vtool` confirmed deployment floor 13.0. No production Go, Runtime, CLI, SDK or dependency source changed, so their earlier module results remain applicable.
- Actual Wails WKWebView inspection first reproduced query loss, search focus on document entry, catalog filtering hiding instructions, lost return focus and crowded narrow-Dock reading. The final native scenes checked Chinese/light/14 px/Flame Orange and English/dark/18 px/custom blue, at normal and approximately 1120 px widths with a pointer-resized 320 px Dock. Named headings, compact scope, complete source/revision, mixed CJK/English literal instructions, long scrolling and persistent Back remained readable. Explicit Back and local Escape restored the retained query and source row; keyboard Return reopened that document. Main conversation scrolling stayed independent.
- Native personal-library review confirmed direct lifecycle/Archive/Restore presentation without redundant disclosure. In the full-pane Skills view, local Escape cancelled proposal confirmation and restored Approve focus. A canonical isolated acceptance used the captured workspace/scope/revision, re-read the remaining proposal and returned focus to search when its row disappeared. Closing the full-pane view returned to the conversation. The shared Diff directory was visually rechecked; its existing selection and return regressions passed.
- Native acceptance used production components through the canonical in-memory fixture in the real Wails host. The fixture deliberately installs a no-op root workspace Escape shortcut; root event propagation is covered by the component regression, while native outer return used the explicit Close action. First document failure and uncertain acknowledgements were exercised in unit regressions rather than claimed as native failure injection. No visual/browser suite or performance benchmark was run.
- The native app exited successfully. The owned preview, disposable data, bundle, binaries, logs, fixture output and WebKit caches were removed. User reference caches and unrelated resources were retained.

For scope-first Diff review and Usage reports:

- The final frontend `VITEST_MAX_WORKERS=2 npm run check` passed: 376 test files and 2745 tests, followed by type, lint, format, unused-code, boundary, token/style, locale, bootstrap, resource, bundle and production-module checks. The new presentation regressions exercise list/detail navigation, direct-source return, query/scroll/focus retention, scope changes, stale/cancelled reads, partial/binary/metadata-only files and truthful retained-report labels. Existing assertions were migrated only for the intentional task-flow changes.
- Shared UI `npm run check` passed all 38 tests and its independently packed UI/icons consumer. Portable-plugin build/check passed all ten exact resource checks; Diff and Usage ESM resources were rebuilt. The final supported production Wails macOS arm64 build passed, with `vtool` confirming deployment floor 13.0. Go, Runtime and CLI sources are unchanged; their prior module results below remain applicable.
- Actual Wails WKWebView inspection reproduced the former all-expanded Diff, then checked the new file overview, one-file reader, previous/next navigation, scope switching, direct Tool entry and the separate directory command. Chinese/light/14 px/Flame Orange and English/dark/18 px/custom accent scenes were checked, including approximately 1120 px windows and a pointer-resized 320 px Dock. Complete paths, Chinese rename provenance, binary content and metadata-only changes remain explicit. Native review caught an inapplicable layout selector on files with no code rows; the bounded Claude Code CLI fix was reviewed, covered by regressions and rechecked in the final native fixture.
- Native keyboard review confirmed that a layout popup consumes Escape first, list-owned detail returns to its retained row, and directly entered detail returns to its actual narrative source. Heading focus follows explicit file navigation. Usage range controls precede local source filtering and grouping; filtering leaves totals intact, and native period selection returns focus to its trigger. The sidebar-matching seam, Composer, code/table, selection and theme owners remain in use.
- Acceptance used the isolated canonical in-memory Runtime fixture through production components in a real Wails host, not a live Runtime installation. No visual/browser suite or performance benchmark was run. Existing browser/performance probes were updated for the new navigation contract but are not reported as passing. The cross-page task map records source-based review and future scope, not fresh native acceptance of every page.
- The native app exited successfully. The owned preview, disposable data, bundle, binaries, logs, fixture output and WebKit caches were removed. User reference caches and unrelated resources were retained.

Earlier integrated verification remains in Git history. The relevant retained evidence is:

- Actual Wails scenes checked shared main/delegated narrative rendering, task/file/nested-source returns, directory/picker/full-view focus and scroll continuity, tool-outcome summaries, bounded code/tables/media, literal selection and the sidebar-matching seam. Normal and approximately 1120 px windows, a pointer-resized 320 px Dock, both themes, mixed CJK/English and 14/18 px text were exercised on the changed surfaces. This does not certify every unchanged page.
- Captured Tool-evidence review exercised ordinary keyboard scrolling, wrapping, Select All/Copy and a 50,000-line output. Shared reader and search regressions cover composition variants; no native IME-method change or fresh native IME coverage is claimed.
- The preceding unchanged Desktop Go sources passed `go test ./...` and `go vet ./...`. The unchanged CLI passed `go test ./...`, `go vet ./...` and `go build ./...` with the workspace enabled. Its initial acceptance-recovery timeout passed in isolation and in the subsequent full run without weakening assertions. Runtime/CLI sources have not changed in these UI batches.
- Earlier browser evidence includes the combined baseline's 20 passed Chromium/WebKit scenarios. A later exploratory run had 5 passed and 7 failed probes with invalid selectors/navigation assumptions; its added probes were removed. Those results are historical and do not qualify subsequent batches. The latest user direction is actual Wails acceptance without visual/browser suites or performance benchmarks.
- Each completed native batch exited its owned app and removed its preview, disposable data, bundle, logs, binaries, fixture output and owned WebKit caches. Do not assume those resources still exist. User reference caches and unrelated resources were retained.

These checks cover selected contracts and native scenarios. Consult the current checkpoint above for fresh results; unrun suites are not passing evidence.

## Future review candidates

The next concrete task is native workspace zoom. A single View → Zoom In in
Wails cropped unrelated chrome and Memory at approximately 1120 px; Actual Size
restored it. Reproduce against production and locate the magnification/viewport
or shell owner before choosing a fix. This is observed evidence, not a request to
change Memory's already verified authoring model.

1. **Extend the task comparison.** Reading-to-file-and-back, overview-to-subagent-and-back, direct narrative task links and nested task/file returns now have shared presentation and focus ownership. Catalogue/picker selection and full-view-to-Dock transitions enter the same inspection flow. Continue with richer captured evidence and the remaining detailed material states. Preserve the distinction between explicit inspection and background updates. Do not restart the implemented typography, file header, shared message renderer, nested return flow or sidebar-matching divider.
2. **Continue the reading composition around activity.** The shared prose step, paragraph/heading rhythm, disclosure affordance and folded tool-outcome summaries are implemented. Review how recorded evidence and media join an answer, especially where a summary leads into a dense detail view. Preserve the existing bounded table, code and selection owners.
3. **Refine the remaining Dock forms.** File identity/actions and delegated message reuse, overview continuity and return focus are established. Continue with richer tool details and cross-view navigation: decide which details belong inline or in a persistent pane, and reserve popovers/dialogs for genuinely bounded tasks. Keep the user-requested sidebar-matching divider unchanged, including hover and drag.
4. **Extend native constraint coverage as content changes.** Normal/narrow native windows, 320 px Dock, default/large text, long directories and both themes were reviewed for file/Diff and selected-subagent paths. Apply comparable checks to the next changed surfaces rather than rerunning broad visual suites or claiming every page is covered.
5. **Continue across other pages.** Memory authoring, command feedback and local cancellation are completed at Actual Size, with verification above. Its draft deliberately permits independent approval/unpin work in the catalog; preserve that model rather than applying Schedules' modal choice to every form. Other settings, installed views and dialogs still need task-specific inspection of location, scope, current-object actions and return paths. Retain the approved brand and Composer contracts.

These are review candidates, not defects or required rewrites. The latest objective resumes work and supersedes the earlier Schedules pause. Separate reproduced defects from design judgments and unverified hypotheses. Do not invent a defect to justify another small change, or substitute isolated icon, color and radius adjustments for a better page and task flow.

## Working and maintenance practice

- Read `/Users/tangerg/Desktop/flame/AGENTS.md`, `/Users/tangerg/Desktop/flame/PROJECT_RULES.md`, `/Users/tangerg/Desktop/flame/DEVELOPMENT.md`, the target module README and applicable nested instructions. This progress file does not replace them.
- The latest goal asks the primary agent to own the overall task model and prefer Claude Code CLI for bounded implementation details. This supersedes the earlier no-Claude preference. The primary agent still owns reference research, implementation review, real Wails acceptance, verification, commits, pushes and cleanup; CLI output is evidence to inspect, not approval to skip those responsibilities.
- Apply relevant better-series skills and inspect the references in depth. Preserve StyleX, Base UI, semantic tokens and one fact/one owner. Runtime owns durable product semantics; layout work does not authorize rebuilding those state machines in the client.
- Batch meaningful changes, then review their combined visual and interaction result. Prioritize development and actual native computer use. The user explicitly asked not to run visual/browser suites and to avoid meaningless tests; retain necessary regression coverage and module checks, and reuse valid results when source and dependencies have not changed.
- UI builds clear shared output, and frontend `precheck` rebuilds it. The supported Wails task can also reinstall frontend dependencies. Finish these operations before module scripts and native acceptance; overlapping them produces misleading missing-resource or dependency errors.
- Portable renderer changes include rebuilding and checking their published ESM resources. A source or resource rebuild does not update an existing Runtime installation; verify the actual selected resource when that installation is in scope.
- When the user requests the frontend, they mean the desktop app. Use actual Wails acceptance for desktop behavior. Visual review uses isolated fixtures; an explicitly requested live model check uses the user's existing configuration without printing or copying credentials.
- If the Mac is locked, advance independent source, reference and build work, and finish native acceptance after unlock. A native build does not prove native interaction.
- Commit explicit owned paths, push verified milestones to remote `main`, preserve unrelated working files during integration and clean owned resources. Update this file with the new product state and verification scope rather than starting another round log.
- Keep user-facing replies in Chinese and repository content in English. Retain the approved visual direction and task model across pages. The latest objective resumes continuous batches until the user explicitly stops. Finish verification, commit to `main`, push and clean each batch before advancing.
