# Desktop UI progress and handoff

Updated: 2026-10-20. Pasted-text review continues from the native hidden-zoom qualification at `55db20f41` on `main`, with the ordinary editing/return paths now natively qualified. The approved brand, Composer, CLI and earlier reading/Context Dock work remain integrated; Git history retains their chronology.

This user-requested file is the maintained progress and handoff record for the desktop UI work. Update its sections in place as the product changes. Git history retains the chronology; keep progress organized by product area rather than numbered rounds or additional progress files.

## Current position

The latest objective explicitly resumes continuous desktop UI work. Complete each evidence-backed batch with native acceptance, the relevant gates, an explicit-path commit and push to `main`, cleanup and an update to this file before selecting the next batch. Native page zoom and the single Dock width projection remain integrated. Open workspace views now retain one instance through selection, collapse and full-pane presentation, and distinct main sources stay mounted on the existing inspection path. Installed result searches, Memory queries/drafts and original command handles survive those changes. Hidden readers detach measurement rather than ending business contexts; source focus and true-close cancellation remain separate contracts.

Actual Wails qualified the retained-view paths in English/dark/18 px/custom blue and Chinese/light/14 px/Flame Orange, including a system-resized narrow window, 320 px Dock, 200% main-view fallback, an original unknown-result check and main-only source → file → source. The subsequent native focus investigation qualified the existing keyboard resize path and isolated clicked-label reverse navigation to a WebKit behavior also reproduced by a bare HTML button. No host preference or component workaround was added. The precise scenarios and limits are below.

Recorded Tool readers now share the literal code reader's local Select All boundary. Whole-reader copy uses the current presented rows or preview excerpt; expanded command output supplies the complete ANSI-stripped captured source, including virtualized lines and trailing newlines. Stable content children keep mutable row trees from shortening the browser's range. Partial selection, editable fields, explicit complete-result copy and Runtime ownership remain separate. Hidden-zoom reading drift reproduces in a plain native HTML scroller without virtualization and remains a platform limitation, qualified below.

Pasted-text review exposed a separate native defect: Put back in message discarded the current edit, restored the original paste and left focus on the document. The repair sends the edited text through one ComposerDraft transition and chooses a close destination from the owner's actual result. Full frontend gates, the supported Wails build and ordinary native editing/return paths pass. The separate 50,002-line preview still loses readable accessibility state, while a plain native textarea handles the exact same content; isolate that difference next without assuming a layout, binding or platform cause.

Task-oriented layout and interaction apply across the conversation, Context Dock, settings and installed views. Preserve the approved brand, shared reading hierarchy, Composer, code/table/selection fixes, sidebar-matching Dock seam and custom-accent theme owner. The earlier `cap`-based inline-icon sizing tip is a suggestion for future typography work, not a global icon redesign.

Recent integrated milestones are `c59e7f470` (scope-first Diff and Usage), `863e4b62f` (Skills catalog/document/return), `565c3eb08` (literal search and local keyboard ownership) and `01af05b5f` (the reported Tab-item Default/Soft overlap). Their behavior and evidence are recorded by product area below. Continue from those owners; do not rebuild them from an older chat.

Schedules now uses a bounded Add/Edit dialog and a separate deletion decision over the retained catalog. The existing command, captured revision, provider/model catalogs and original-key reconciliation remain authoritative. Focus retirement accounts for catalog reads temporarily disabling record actions, while Add/Templates remain available. Actual Wails acceptance qualified both the shared modal stacking repair and persistent command feedback after scrolling the fields. Detailed coverage and its limits are below.

The checkout is `/Users/tangerg/Desktop/flame`, on `main`. At the earlier branch-consolidation checkpoint, former local and remote tips were checked for ancestry and retained in `main`. Additional worktrees and their processes are outside this UI batch. Read current Git state before resuming; this document cannot overwrite newer work. Historical paused or blocked states in other chats do not describe the current checkout.

## Integrated brand and authoring work

The previously protected **76 modified, deleted or untracked files** have now been committed at the user's request and verified together in an isolated checkout. The original working tree is clean at this checkpoint. These features are implemented; do not recreate them from the older handoff inventory.

The user has approved the brand direction. Its production assets, settings pane, translations and source artwork are committed:

- `/Users/tangerg/Desktop/flame/brand` contains the primary mark, dimensional companion, plush character and usage guidance.
- `/Users/tangerg/Desktop/flame/design/branding` contains concept and refinement assets.
- `/Users/tangerg/Desktop/flame/desktop/frontend/src/lib/brand.ts` and `/Users/tangerg/Desktop/flame/desktop/frontend/src/lib/brand.stylex.ts` connect the assets and brand color.
- `/Users/tangerg/Desktop/flame/desktop/frontend/src/plugins/builtin/settings/brand` and `/Users/tangerg/Desktop/flame/desktop/frontend/visual/brandSettings.browser.spec.ts` belong to the brand work.

Composer submission captures the exact immutable draft and Session at the authoring owner. Slash handlers explicitly accept input or admit a send; returning from a handler alone does not clear the draft. Accepted input records history in its captured Session, consumes only the submitted revision, preserves later text and attachment edits, and avoids stealing another control's focus. Replacing Runtime storage retires old callbacks. Goal reports acceptance through this same owner rather than comparing text or clearing independently.

The pasted-text repair keeps those submission and storage contracts. The
required `restorePaste(id, text)` intent accepts the current reviewed text in one
draft transition, removes only that paste and preserves the typed prefix and
other attachments. Empty reviewed text removes the paste without resurrecting
its old body; a missing ID remains a no-op. `editPaste` reports whether the owner
kept the exact paste, so the view does not repeat the domain's blank-text rule.
Cancel/Escape and ordinary Save return to the surviving chip; Put back and a Save
that removes it return to the existing Composer textarea. One local close intent
captures the focus origin and target before a retiring action. The resolver
suppresses a return on unrelated later focus, retirement without a close intent
or a disconnected target. `TextEditorDialog` only forwards the shared modal's
existing `finalFocus` option. The native scenarios and qualification boundaries
are recorded below.

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

Recorded-evidence review traced ZCode's actual `ToolCallBody.tsx` and
`renderers/ExecuteOutput.tsx` for a bounded content viewport separate from its
summary and actions, and ChatGPT's `artifact-preview-header-56e5190c3624.js` and
`artifact-originating-turn-a94e56394ddb.js` for object/source identity around
material. Flame's existing `CodeBody` supplies the relevant local keyboard
contract. The [Selection API](https://www.w3.org/TR/selection-api/) describes the
single browser-owned range and its live DOM boundaries; the
[Clipboard Event API](https://www.w3.org/TR/clipboard-apis/#clipboard-event-api)
supplies complete presented text during a trusted copy event. The repair uses
those native owners rather than storing another selection fact. Static reference
artwork supplies no keyboard evidence, and no new visual skin was introduced.

Pasted-text review traced ZCode's `v4/composer/useComposerAttachments.ts` for
large-paste attachment ownership and `ChatMediaAttachmentPreviewDialog.tsx` for
a bounded named reading task. ChatGPT's actual `app-primary-44ec287874b7.js`
pasted-attachment component supplies identity, removal and an explicit Show in
text field action. The similarly named clipboard-paste modules contain an icon,
not paste behavior. Those sources support task identity and a return to
authoring; Flame's editable review and accepted-draft rules remain its own.
The existing Base UI 1.9.0 implementation confirms that a function `finalFocus`
is explicit and bypasses its default outside-focus guard, which the local close
intent must therefore preserve. No second component system or visual skin was
introduced.

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

`ui/src/reader-selection.ts` owns the shared local Select All guard and
whole-reader copy event. CodeBody retains its existing literal-region behavior.
Captured result readers supply one presented-row slice to rendering and copy;
line labels, line numbers and continuation controls are presentation, not copied
evidence. Preview copy includes only the presented excerpt, preserving real blank
lines instead of rendering padding. Expanded output copies the complete original
ANSI-stripped source, including offscreen lines and trailing newlines. Partial,
outside, collapsed, already-handled and editable-descendant copies stay native.
The browser range is the only selection owner. Mutable result and virtual-output
rows sit beneath one stable child so reconciliation cannot shorten its boundary
offsets; the existing viewport, focus, scroll and measurement owners stay in place.

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

## Retained workspace and result views

The native Memory query reset had two host causes. Activity hide disconnected
`RenderingComponent`'s external-store subscription, ending the captured context;
composition Effect cleanup also retired resolved descendants. Main and Dock
placements separately rendered the same view. Preserving only a search value
would have left drafts and original mutation retry closures attached to retired
contexts.

`ChatPanel.tsx` now renders one keyed body per exact Session/view ID. Its existing
Dock tabs, active main view and distinct main sources on the inspection chain
supply retention; there is no second state cache. A same-view return shortcut
cannot keep a closed moved tab alive. `PanelHost` changes only its context value
through the optional `hosted` prop, preserving full-pane and Dock density without
moving children. The stable Dock container is neutral and non-focusable in main
mode; the named body and persistent window header keep their roles, and main
content does not inherit Dock containment/clipping.

Conversation results and retained delegated readers use the same mounted-tree
policy. `src/ui/content-visibility.tsx` projects inherited presentation visibility
without state, cancellation or I/O. `MessageStream` preserves the public follow
library's two DOM wrappers and viewport class, attaching its measuring refs only
while visible. `ChatStream` suspends only its Composer geometry observer, and
`SubagentTranscript` combines ancestor and local visibility. This matters in real
WebKit: retained hidden readers otherwise measured zero and jumped to the bottom.
No scroll-position cache or replacement business lifetime was added.

Native page zoom while a retained reader is hidden has a separate limit. On
macOS 26.5, changing 150% to 200% while its ancestor is `hidden` changed a
777,329 CSS-pixel position to 582,997 on return. A plain HTML scroller reproduced
the exact change without React, virtualization, ResizeObserver or application
scroll handlers. Ordinary hide/show at unchanged zoom retained 777,329, and visible
zoom retained that same position. This is not evidence of another component
lifetime or virtual-window defect. The [WebKit scroll-area implementation](https://github.com/WebKit/WebKit/blob/main/Source/WebCore/rendering/RenderLayerScrollableArea.cpp)
saves and restores a layer position; [the DOM scroll getter](https://github.com/WebKit/WebKit/blob/main/Source/WebCore/dom/Element.cpp)
removes the current zoom. A saved layer offset restored under a different scale
is consistent with the observed 150/200 ratio. That is a source-based explanation,
not identification of the exact installed system WebKit revision. Keep this
limitation explicit rather than adding a competing application scroll cache or
keeping every hidden workspace laid out solely for this edge case.

`useInspectionFocus.ts` gives a removed layout trigger's focus to the presented
main body, visible Dock or conversation only when focus fell to the document
body. Existing explicit source returns and later outside focus keep priority.
Actual tab removal, main-only replacement, exact Session/target replacement and
resource/contribution retirement retain their cancellation behavior. Runtime,
receipts, bindings, SDK and portable component sources are unchanged.

Reference evidence: ZCode's read-only `AnimatedSidePanePanel.tsx` retains actual
tab content with `forceMount` plus inactive hiding and explicit visibility for
owned side-pane work. ChatGPT's actual `ReviewThreadSidePanelTab` separates its
review state from the portal target; it is not export-renderer evidence. The
[React Activity contract](https://react.dev/reference/react/Activity) documents
Effect cleanup on hide; [Radix Tabs](https://www.radix-ui.com/primitives/docs/components/tabs)
provides the semantic reference for persistent tab content. Flame retains its
Base UI/StyleX owners. The better-layout/accessibility review drove instance,
measurement, landmark and focus boundaries; existing typography, writing and
semantic color owners were checked in both native themes without a palette or
prose redesign.

Verification for this contract:

- Final frontend `VITEST_MAX_WORKERS=2 npm run check` passed **381 files / 2,806 tests**, all architecture/style/resource guards, all ten exact ESM resources, production bundling and shared-module linking. Shared UI `npm run check` passed **52 tests** and the independently packed consumer. The supported final Wails arm64 build passed; `file` and `vtool` confirmed arm64 and macOS 13.0. Standalone Go/SDK/portable module suites were not rerun because those sources are unchanged; the frontend gate includes its isolated Runtime lifecycle integration.
- Normal and StrictMode regressions retain the same query/draft nodes and captured contexts across collapse, tab switching, main promotion and return. A deferred original save settles once while hidden. Real installed conversation/delegated renderers retain state; actual close, Session replacement and hidden resource/lifetime retirement still end them. Measurement tests use public follow refs and a recording ResizeObserver, including hidden ancestors. Main-only source/file return and closing a moved main view are separate regressions.
- Original host/conversation/nested cases exposed aborted contexts; measuring regressions failed without the visibility gate. Native acceptance then exposed the reader jump and lost main-trigger focus. A final source-return regression exposed main-only source unmount; retaining every self-return would instead keep a closed tab alive, so only distinct inspection sources retain a main-only view. The initial gate correctly rejected cross-context tests inside private builtin folders; they now live in `visual/*InstalledContinuity.test.tsx`. Unsupported role-query options and initial fixture assumptions were corrected without weakening assertions, deadlines or checks. Recurring non-failing localhost `ECONNREFUSED` diagnostics remain unclassified.
- Actual Wails preserved Memory query `compaction`, mixed CJK/English long-path drafts and input identity through explicit collapse, tab switching, main promotion, Actual Size and return. A delayed refusal across main → Dock → hidden kept one command/key, returned named feedback and left the draft editable. In the light Chinese scene, an unknown original result survived hide and full-pane movement: **one command, two observations, the same key**, with its signal live until actual tab close. Cancel/Save remained blocked while unknown, and local Escape after settlement restored the record action with the query retained.
- The dark delegated reader kept its search input identity/value and exact **3,300 px** reading position through Files-tab round trips, collapse/show and nested-task Escape return. Before measurement gating the same native path jumped to the bottom. A main-only Subagents view absent from the Dock's tab list also retained its real installed result search through file inspection and returned focus to the original file reference. Moving it into the Dock reused the view; Delete then removed the instance. No native resource-generation/reload test is claimed.
- Native 200% zoom with a collapsed fixture sidebar produced a 720 × 450 CSS viewport and reachable main controls. Focus entered the named main region; Shift-Tab/Return reached Close and returned to the conversation with its unsent Composer text intact. System Move & Resize produced an approximately 1,122-point narrow window. Real pointer drags changed a 660 px Dock to 620 px and reached 320 px in the light scene, retaining the query/draft and seam. Editor Tab reached Cancel after scrolling, and Escape returned to Edit. Native keyboard resizing was not freshly qualified: the recorded tab-to-handle focus route needs investigation below.
- These scenes use the production components and shared React graph through actual Wails asset/IPC hosting, canonical in-memory reads and bounded synthetic command observations. They qualify UI continuity and the SDK's original-handle mapping, not a live Runtime installation, durable receipt journal or provider restart. No live package selection or Runtime write was performed. No browser visual suite or performance baseline ran. VoiceOver, real OS IME composition and rendered contrast measurements remain outside this qualification.
- Cleanup completed: all owned native/CLI/check/build processes exited. The app was unregistered; its bundle, fixture/data, overlays, production binary, logs, all four completed CLI reports and the acceptance root were removed. Newly created bundle-specific WebKit/cache paths were removed with preexistence/birth-time guards. No preview server was started. Existing shared outputs/compiler caches, read-only Ace references and unrelated worktrees/processes were retained. The primary agent owned research, boundaries, review and native acceptance; Claude Code CLI supplied the bounded implementation and regression work.

## Native workspace zoom and Dock geometry

macOS View commands retain the default menu roles, shortcuts, 5% steps and
Actual Size floor. `desktop/window_zoom_darwin.go` replaces only their callbacks;
`desktop/window_chrome_darwin.go` reads and changes `WKWebView.pageZoom` on the
AppKit main thread. WebKit owns the scale. The exact window's public content
view hierarchy supplies its WebView. Native button coordinates are divided by
that scale before crossing the existing CSS-pixel boundary, and the native
fullscreen style mask supplies measured zeroes. Other platforms keep their
Wails menu implementation. Dependencies and IPC methods are unchanged.

The pinned Wails macOS roles used viewport magnification. Production first
reproduced an outer scroll area and missing sidebar footer after a single Zoom
In. Switching to page zoom exposed a second problem: at 200%, a collapsed
320 px Dock had a -640 px margin, making the 720 px conversation 1040 px wide.
The Dock now collapses its flex basis to zero. While open, the duplicate CSS
width formula also drifted: at 120%, End painted approximately 518 px in a
925 px row and left 407 px for reading instead of the existing 440 px floor.

`ChatPanel.tsx` now projects the observed row through `shellGeometry` once,
using the saved ratio or automatic width. `AgentDockRow` renders that pixel
measure and `DockResizer` consumes the same committed width. Row observation
advances no saved preference. The retired `dockWidth.ts` CSS arithmetic and its
exclusive CSS/TypeScript comparison probes are removed; the independent name
legibility probe remains. The preferred-width constant is now private to its
actual algorithm owner.

Direct pointer and keyboard resizing suspend width transitions. Opening and
closing retain their visibility timing, surfaces and seam. The host structure
stays mounted; installed-view continuity is qualified in the section above.
`ResizeHandle`
retires a live preview when a new committed width arrives, preserving the new
projection after rollback; cancellation without that replacement retains the
captured value and priority. Its accessible value and Dock gesture origin use
the committed pixel width, avoiding a cached intermediate animation frame.

Reference evidence: ZCode's read-only preload zoom code uses native content zoom
and divides macOS control padding by the current factor. The local Apple SDK
and [Apple pageZoom documentation](https://developer.apple.com/documentation/webkit/wkwebview/pagezoom)
distinguish content scaling from viewport magnification; page zoom is available
below the project's macOS 13 floor. Public [Wails menu APIs](https://v3.wails.io/reference/menu/)
provide the callback boundary; the installed beta.29 source establishes the
actual behavior. The better-layout/accessibility review drove viewport reflow,
reachable controls and width semantics. Existing type, color and surface owners
remain the visual basis.

## Verification checkpoint

For pasted-text review and return to authoring:

- Actual baseline Wails used English/dark/18 px/custom blue with an isolated canonical Agent fixture. An 18-line CJK paste could be reviewed and cancelled back to its original chip. After replacing the editor with `Edited evidence · 修改后正文` and `Final line`, Put back instead appended the original 18 lines to `Typed prefix · 保留草稿`. Focus fell to the document and subsequent typing did nothing. This reproduced both the stale-text and lost-authoring-focus defects without a live Runtime write or Send action.
- The first regression baseline failed six cases and passed 34: edited/emptied put-back lost current text, and retiring attachments lost focus. The first repair then exposed three later-outside-focus failures across Cancel, Save and Put back. The revised composed tests cover those paths, surviving-chip returns, Session retirement, removed Composer targets and the synchronous owner-reported edit result. After primary review captured Save's exit before the owner change, the primary focused run passed 39 files and 305 tests. Existing accepted-submission, draft history/storage, Goal and plugin-editor checks remain included; assertions and deadlines were not weakened.
- The complete frontend `VITEST_MAX_WORKERS=2 npm run check` passed 381 files and 2,834 tests, including its isolated Runtime lifecycle integration, followed by every type, lint, format, unused-code, architecture, style/token, locale, bootstrap, exact-resource, bundle and production React/JSX linkage gate. Recurring loopback `ECONNREFUSED` diagnostics remained non-failing and unclassified. The subsequent supported `wails3 task build` passed; `file` and `vtool` confirmed arm64 and macOS 13.0, with the existing non-failing duplicate `-lobjc` warning. Final native fixture assets were rebuilt serially after that build. No shared UI package, portable component resource, Runtime, CLI, SDK, dependency, protocol or persisted-data format changed. Private in-process callback signatures were migrated together. Standalone module suites were not rerun for unchanged sources; the preceding shared-UI results remain historical. No browser suite or performance benchmark was started.
- Actual final Wails acceptance qualified English/dark/18 px/custom blue in a 1440-point window and Chinese/light/14 px/Flame Orange after native Move & Resize → Left produced a 1122 × 950 CSS viewport. The named editor, field focus and persistent footer remained readable. Edited CJK/English content returned exactly after the typed prefix, with the caret at the message end; post-close typing appended there. A separate large attachment remained intact. Saving blank text removed the last attachment and returned focus to the message. In the light scene, ordinary Save returned to the same chip, reopening showed the saved two-line text, and Cancel/Escape discarded later edits and restored that chip. Keyboard Tab/Return put back an empty review without resurrecting its old body, and Chinese typing continued at the prefix end.
- Return is qualified after the dialog closes. An automated key burst sent during the existing exit transition did not append; no in-transition keystroke guarantee is claimed. Later-outside-focus and Session/target retirement are composed regressions, not native failure injection. No live Runtime write, Send, resource replacement, OS IME, VoiceOver, rendered pixel-contrast or other-platform qualification occurred. These scenes use the production components and real Wails host over canonical isolated reads.
- The initial 50,002-line probe overlapped a tool-reported macOS lock. After the native window became available again, the ordinary paths above were qualified. A separate plain native textarea then held the exact **1,188,891-character / 50,002-line** generated text, with direct equality confirmed by read-only diagnostics. Its accessibility tree stayed available; Cmd-Down visibly reached line 49999 and the final blank lines, with the caret at 1,188,891. Escape cleared that disposable field. The final production editor accepted the exact same value but again yielded an empty accessibility tree, and the Escape probe did not restore readable accessibility state. Native Quit eventually exited. This narrows the next investigation; it does not establish a general WebKit textarea limit, a specific application bottleneck or an oversized-editor repair.
- Both bounded Claude Code CLI implementations completed with actual model `claude-opus-5-5`, no subagents and no permission denials. The primary owned reference research, observed failures, review, final gates and native acceptance. The earlier locked baseline instance was stopped through an audited owned-process SIGTERM; the final instance exited after native Quit. All owned native, CLI, test and build processes exited. The bundle was unregistered, and its acceptance root, fixture/plain control, overlays, production binary, data, diagnostics, CLI reports and logs were removed. Newly created bundle-specific WebKit/cache paths were removed against the pre-launch baseline and birth times; preferences, HTTP storage and saved state were absent. No preview server was started. Shared outputs/compiler caches, read-only reference assets and unrelated worktrees/processes were retained.

For the native hidden-zoom investigation:

- Actual Wails on macOS 26.5 used production components and the canonical read-only Tool-shell fixture in English/dark/18 px/custom blue, at a 1440-point window. Find located captured output line 31415. Visible native zoom from 100% to 150% retained the same output node and 777,329 CSS-pixel position. Entering the fixture's unavailable file main view hid that conversation; it is a valid hiding probe, not qualification of a registered Files workflow. After hidden zoom to 200%, Close and Actual Size returned that same node at 582,997, around line 23555. Its row height and complete content height remained unchanged.
- A separate plain HTML page in the same real Wails host used one fixed-height overflow container and a tall content child, with no React, virtual rows, ResizeObserver or application scroll handlers. A native button established the same reference position. The identical 150% → hide → 200% → show → Actual Size sequence reproduced exactly 777,329 → 582,997. A separate hide/show at unchanged 100% retained 777,329. Temporary read-only diagnostics observed DOM geometry; they did not restore or synthesize scroll state. No live Runtime write, recorded Tool execution or installed-package update occurred.
- This confirms a native hidden-zoom limitation, not passing reading continuity under that condition. Current upstream WebKit source supplies a consistent save/restore-and-zoom explanation; the exact system WebKit revision and an upstream repair were not qualified. The better-layout/accessibility review retains browser scroll ownership and the existing semantic visibility/focus boundaries. No product source, theme, dependency or generated resource changed, and no implementation was delegated because none was justified by this investigation.
- The isolated fixture production build and native overlay compilation passed; the duplicate `-lobjc` warning remained non-failing. Only this handoff changed, so its formatter and `git diff --check` are the fresh repository checks. The preceding full UI/resource gates and supported production build remain historical results, not new runs. No browser suite, performance benchmark, VoiceOver, OS IME, other-platform or new theme/window matrix ran. The first disposable launch had an incorrect asset root and unregistered scene parameter; those acceptance setup errors were corrected before qualification.
- Cleanup completed: every owned native/build process exited, the app was unregistered, and the fixture, plain page, overlay, binary, data, observations and logs were removed with their acceptance root. Newly created bundle-specific WebKit/cache directories were removed against the pre-launch baseline and birth times; preferences, HTTP storage and saved state were absent. No preview server was started. Existing shared outputs, compiler caches, read-only references and unrelated worktrees/processes were retained.

For recorded-evidence selection and copy:

- The final complete `VITEST_MAX_WORKERS=2 npm run check` passed 381 files and 2,820 tests, followed by all type, lint, format, unused-code, architecture, style/token, locale, bootstrap, resource, bundle and production React/JSX linkage gates. The preceding full run passed all frontend tests but timed out once in the unchanged Runtime Memory durable-cold-read case. That exact case passed alone in a fresh isolated Runtime in 10.92 seconds, then the complete gate passed with the original assertions and deadlines. The one timeout's cause remains unclassified; recurring loopback `ECONNREFUSED` diagnostics were also present on successful runs.
- New regressions exposed whole-workspace Select All, copied reader framing and live-range shrinkage during virtual-window/page reconciliation. Review also exposed editable-descendant copy overriding a retained reader range. Each was reproduced before its repair. The final focused recorded-result run passed 29 tests; the CLI's visual-unit run passed 13 files and 124 tests. Ordinary partial/cross-boundary/collapsed/handled copy, composition and modifiers, literal prose, ANSI, real blank lines, filtering, paging and exact complete-source bytes remain covered. Test-only ANSI-regex lint and incorrect overscan/trailing-newline expectations were corrected without relaxing the contracts.
- Shared UI `npm run check` passed 58 tests and the independently packed UI/icons consumer. Portable build/check passed all ten exact resources after the stable content boundaries; only the Tool-results ESM changed. No production Go, SDK, Runtime or CLI source changed, so their standalone module checks were not rerun. Existing browser row selectors were migrated for the intentional content boundary, but no browser suite or performance benchmark ran.
- The supported `wails3 task build` production build passed. `file` confirmed arm64 and `vtool` confirmed the macOS 13.0 deployment floor. The duplicate `-lobjc` linker warning remains non-failing. There is no dependency, persisted-data or protocol migration.
- Actual Wails acceptance used English/dark/18 px/custom blue in a 1440-point window and Chinese/light/14 px/Flame Orange after system Move & Resize produced a 1122 × 950 CSS viewport. A real pointer drag reduced the light Dock from 364 to 320 px. The final rapid expansion → Page Up → Select All → Copy sequence supplied the complete **1,188,891-character** ANSI-stripped source, including two trailing newlines, in both scenes. Only 29–32 output rows were mounted. The earlier structure had fallen back to partial native copy during that same rapid light sequence; the stable content child resolved it without selection state or retries.
- Native source-result copies contained the presented 40 rows (1,599 characters), 80 rows (3,199) or filtered record 42 (39), excluding the `Line 42` framing that the first candidate actually pasted. The preview's nine presented lines copied 192 characters with a real blank instead of padding spaces. Real mouse selection and system paste preserved exactly one 23-character output line. Full-output system paste produced a 50,002-line Composer text fragment. Find retained native input Select All/Copy, and Escape returned focus to Tool output; subsequent selection, virtual scrolling and complete copy remained correct. Native Tab to paging normally retired the old selection; the app does not recreate it.
- Acceptance used production components and the captured SDK context with deterministic, read-only providers in an actual native host. It did not execute recorded Tools, mutate a live Runtime, update an existing installation or qualify native resource replacement. Rebuilt sources/resources must still be delivered through the normal installation owner. VoiceOver, OS IME, other platform GUIs and rendered pixel contrast were not qualified. The exploratory hidden-zoom reading change and oversized-paste preview limit below are not passing continuity/preview claims. An unsupported `zh-CN` fixture query was corrected to the registered `zh` locale; it was an acceptance setup error, not a product defect.
- Cleanup completed: all owned native, CLI, test and build processes exited; the app was unregistered and removed. Its newly created bundle-specific WebKit/cache paths were removed against the pre-launch baseline, with no preferences, HTTP storage or saved state present. The acceptance root, fixture/data, overlays, production binary, logs, all three completed CLI reports and the CLI's guarded temporary baseline copies were removed. No preview server was started. Shared outputs/compiler caches, read-only reference assets and unrelated worktrees/processes were retained. Claude Code CLI (`claude-opus-5-5`) supplied the bounded details and regressions; the primary agent owned research, decisions, review, native qualification and final verification.

For native zoom and shared geometry:

- The final focused run passed 51 tests across four ChatPanel/ResizeHandle/geometry files. New controlled-width regressions failed against the old preview settlement (500 px was overwritten by 320 px), then passed with the fix. Observed row changes retain the mounted task and saved ratio; accessible values report the committed width while the surface transitions. Existing cancellation, priority restoration, keyboard and Session ownership assertions remain.
- Desktop `go test ./...` with the supported macOS 13 linker floor and `go vet ./...` passed. The supported Wails arm64 production build passed; the duplicate `-lobjc` linker warning remains non-failing. Windows compilation was checked by the bounded CLI, with no native Windows UI qualification. No dependency, persisted-data or protocol migration is required.
- The final `VITEST_MAX_WORKERS=2 npm run check` passed all 378 files / 2788 tests and every subsequent type/lint/format, unused-code, architecture, style/token, locale, bootstrap, exact-resource, bundle and production-module gate. Earlier gates caught the retired preferred-width export and a CSS-variable owner map still naming the deleted helper; both now point to their actual owners. One interim Runtime setup exceeded its unchanged 60-second compilation hook, leaving 49 cases unrun. After isolated Runtime/CLI/package precompilation, the focused 49 cases and the final full gate passed. No timeout or assertion was weakened. Non-failing loopback `ECONNREFUSED` diagnostics remain unclassified; no Runtime repair is claimed.
- Final actual Wails scenes used canonical in-memory reads and production components through the real native asset/IPC host, with temporary read-only viewport diagnostics. English/dark/18 px/custom blue and Chinese/light/14 px/Flame Orange covered menu and keyboard zoom, 120% width bounds, 200% reading and material fallback, settings scrolling, a nested language menu's Escape/focus return and explicit Close. At 200% in a 1440-point window, the document and conversation were 720 CSS px wide with no outer overflow; the collapsed Dock was zero width. At 120%, keyboard End and a real mouse drag produced a 485 px Dock beside a 440 px conversation in a 925 px row. Home and arrow adjustments applied immediately. A native mouse drag at Actual Size also changed 523 px to 603 px and retained a ratio.
- The system window menu produced approximately 1120-point geometry. One Zoom In kept the conversation, Memory controls, record actions and Composer reachable. Fullscreen reported zero native-button clearance; a settled exit restored the scaled values. Actual Size and the existing Zoom Out floor restored a 1.0 content scale. A retained query and mixed CJK/English long-path draft survived enlargement/Actual Size without a write; moving focus later to the sidebar was preserved. Local editor Escape returned to its record. Full-width Memory remained scrollable with keyboard cancellation and a reachable Close, but the separately observed query reset above is not a continuity pass.
- These are isolated UI and native-host checks. The initial production reproduction read an already-running local Runtime; no business mutation was submitted. Later acceptance used inert canonical providers. Live Runtime recovery, native mid-drag cancellation/resize injection, OS IME, VoiceOver, pixel contrast and other platform GUIs were not requalified. No browser/visual suite or performance benchmark ran. Drag retirement and cancellation order are covered by the component regressions.
- Cleanup completed: the native app exited, its bundle was unregistered and removed, and its newly created bundle-specific WebKit/cache directories were deleted. This batch's isolated root, fixture assets/data, production and prebuild binaries, overlays, logs and all three completed CLI reports were removed. The timed-out E2E root and builds had already exited and cleaned up. No preview server was started. Read-only Ace reference caches, existing shared build outputs/compiler caches and unrelated worktrees/processes were retained. Claude Code CLI supplied the bounded native and geometry implementation; the primary agent reviewed the diff, corrected the accessible-width projection, performed native acceptance and completed verification.

For the subsequent native keyboard investigation:

- Actual Wails on macOS 26.5 qualified the unchanged default host in English/light/14 px/Flame Orange. Tab from Approval mode reached the resize handle; Tab entered the selected Dock tab and Shift-Tab returned to the handle. Home committed 320 px, End committed 725 px, Right reduced width by 8 px and Shift-Right by 24 px. Focus remained on the named separator and the semantic focus outline was visible. This is focused keyboard qualification, not a fresh pass of every earlier theme/window scenario.
- Clicking the tab label, by accessibility action or real screenshot-coordinate mouse input, left repeated Shift-Tab on that tab. Read-only event diagnostics showed an unprevented Tab event and no focus transition. Forward Tab to Browse panels, followed by two Shift-Tabs, reached the handle. A disposable bare HTML page with three ordinary buttons and a nested text span reproduced the clicked-label behavior without React, Base UI or Flame's focus handlers; pure keyboard traversal remained reversible there as well.
- A controlled native A/B explicitly enabled Wails' existing `TabFocusesLinks` preference in an external Go overlay. It did not change the clicked-label result; the normal keyboard path also worked with the production default. The result matches the focus-navigation starting-point issue described in [WHATWG HTML #12514](https://github.com/whatwg/html/issues/12514) and its [proposed specification repair](https://github.com/whatwg/html/pull/12843). This is an evidence-backed platform limitation, not a resize-algorithm failure. Revisit it when the underlying WebKit behavior changes; do not add a competing application Tab order or change users' OS settings.
- Production source, dependencies and generated resources were unchanged. The isolated fixture build and native overlay compilation passed; the duplicate `-lobjc` warning remained non-failing. Only this documentation changed, so verification used its formatter and `git diff --check`; prior full gates remain historical evidence rather than newly run checks. No browser suite, performance benchmark, OS IME or VoiceOver check ran. No bounded implementation was delegated because the investigation justified no production change.
- Cleanup completed: all owned native processes exited, the bundle was unregistered, and the acceptance app, bare probe, fixture, overlay, binary, data and logs were removed. Newly created bundle-specific WebKit/cache paths were removed against the pre-launch baseline. No owned preferences, HTTP storage or saved state were present; no preview server was started. Shared outputs, compiler caches, read-only references and unrelated worktrees/processes were retained.

For Memory authoring and command feedback:

- `npx vitest run visual/memoryDrafts.test.tsx` passed all 22 tests. The baseline's existing 10 tests passed before implementation; new task/feedback/focus cases exposed the old behavior. The CLI supplied the initial regression draft but did not complete implementation; its incorrect named-section/selection assumptions were corrected against the real shared components. Coverage retains original mutation identity, same-key reconciliation and independent approval/unpin, and adds local/search-first/nested Escape, IME and legacy 229, pending/unknown blocking, named feedback ownership, read-disabled return targets, scope retirement, results without an accepted catalog and later-focus preservation after a delayed read. No valid assertion was weakened.
- Portable `npm run build` and `npm run check` passed, including all ten exact ESM resources. The final frontend `VITEST_MAX_WORKERS=2 npm run check` passed 378 files and 2784 tests, followed by all unused-code, boundary, style/token, locale, bootstrap, resource, bundle and production-module gates. Non-failing loopback `ECONNREFUSED` diagnostics were emitted during the full run; no product cause or repair is claimed. An introduced test-selector type error was corrected before this final run.
- The supported production Wails arm64 build passed; `vtool` confirmed macOS 13.0. Shared UI, Go, Runtime, CLI, SDK and dependencies have no source changes in this batch. Their module suites were not rerun for this presentation-only change; separate Runtime commits are not requalified here. The Memory ESM is rebuilt, but an existing Runtime installation still requires the normal explicit release selection to consume a new package; no live installation was changed.
- Actual Wails WKWebView first reproduced missing local cancellation and an edit refusal surviving Cancel into a fresh Add task. Final Chinese/light/14 px/Flame Orange and English/dark/18 px/custom-blue scenes checked record identity, literal query/return, keyboard reopen, search-first Escape, failed edit then fresh Add, independent record failure with a retained editable draft, nested menu priority, named deletion/cancellation and source focus. The native system window menu resized the app to approximately 1120 px; keyboard resizing reached a 320 px Dock. Wrapped IDs, mixed CJK/English, long workspace text, scrolling fields/feedback/actions and the unchanged main conversation stayed usable. The full-pane Memory placement also retained its query and record focus on local cancellation, and explicit Close returned to the conversation.
- The Memory batch first failed native View → Zoom In in its narrow/dark/18 px scene. That baseline led to the native workspace and shared Dock fixes above; the subsequent zoom qualification is recorded separately below.
- Native command refusals used the fixture's explicit unsupported-write error through the production component and public client in a real Wails host. Successful acknowledgements, unknown receipts, delayed/failed reads and IME variants are deterministic component/public-client regressions, not native failure injection or OS input-method coverage. VoiceOver and rendered pixel contrast measurements were not run. No browser/visual suite or performance benchmark was run.
- The owned native app and preview have stopped. The disposable bundle, binaries, fixture output, prompts/logs/data and bundle-qualified WebKit/cache directories are cleaned before delivery. No own preferences, HTTP storage or saved application state were present. Existing prerequisite outputs, user data, read-only references and unrelated processes are retained.

Memory review scope is the add/edit/delete task and its relationship to independent
catalog commands, using React, shared Base UI controls, StyleX and existing semantic
tokens. Repository, module, design and desktop-polish instructions were read.
Other management pages and live Runtime writes are outside this fresh review.

| Domain        | Evidence inspected                                                                                                 | Result                                                |
| ------------- | ------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------- |
| Accessibility | Panel local keyboard owner, shared SearchField/menus/RecordForm, 22 regressions and native focus paths             | Changed task paths verified; VoiceOver not verified   |
| Layout        | Shared Panel/form/record owners, native Dock at 320 px, actual narrow window and full-pane placement               | Actual Size and recorded native zoom scenes verified  |
| Writing       | Captured command labels, record/scope identity, refusal/recovery and new-task feedback retirement                  | Changed task copy verified against its actual action  |
| Typography    | Shared RecordMeta/RecordBody/Metadata, wrapped IDs and mixed-language text at 14/18 px                             | Actual Size verified; no font system changed          |
| Colors        | Existing text/surface/focus/negative roles, passing palette regressions and light/dark/custom-accent native scenes | Role use retained; rendered pixel ratios not measured |
| UI polish     | Existing surfaces, modal-free authoring, normal-motion controls, nested menus and independent scrolling            | Changed flow verified at Actual Size                  |

| Severity | Domain        | Location                               | Before                                                                                     | After                                                                          | Why                                                                                      |
| -------- | ------------- | -------------------------------------- | ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------- |
| MEDIUM   | Writing       | `plugins/memory/src/component.tsx:263` | One unlabeled message served independent record and draft commands and survived a new form | Captured action/target/origin feedback stays in its corresponding task context | A result must identify the operation it settles                                          |
| MEDIUM   | Accessibility | `plugins/memory/src/component.tsx:339` | Return attempted a read-disabled opener; editor Escape had no local owner                  | Available-opener/search resolver and composition-safe local cancellation       | Keyboard work must preserve a useful return point and the current task boundary          |
| HIGH     | Layout        | Native View → Zoom In, whole workspace | Enlarged content is clipped at the window's right edge                                     | Native page zoom, CSS-pixel chrome and one Dock width projection               | Critical controls must remain reachable under scaling; no Memory-only patch is justified |

Review verdict: **Pass for the changed zoom and geometry contracts in the recorded native scenes**. The original workspace zoom block is resolved. Subsequent installed-view continuity is qualified above; VoiceOver and rendered contrast measurements remain unqualified.

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

The native tab-to-handle investigation is complete. Pure keyboard traversal and
resizing are qualified above; clicked-label reverse traversal reproduces in bare
HTML and remains a platform limitation. Preserve the existing geometry and focus
owners. Continue with the remaining task-oriented evidence and reading paths,
selecting scope from current source and actual native observations.

The hidden-zoom investigation is complete at the scope above. Its exact position
change reproduces without application scrolling code; retain the documented
platform limitation and revisit it when WebKit behavior changes. The selection
repair does not fix or claim that condition. The ordinary pasted-text return
repair is now qualified above. Next isolate the oversized preview's difference
from the working plain textarea: controlled value/default value/text content,
autosize, modal rendering and native accessibility remain hypotheses, not
diagnoses. The exact same generated content worked in the plain native control;
the application preview still lost readable accessibility state. Do not infer
a performance bottleneck or start a broader Composer rewrite from that probe.

Memory query/draft continuity, original-command observation and main-only source
retention are now implemented and qualified above. Do not recreate them or make
Memory authoring modal. Preserve captured-target retirement and true-close
cancellation while extending other task paths.

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
