# Desktop UI progress and handoff

Updated: 2026-10-19. This continuation started from clean local and remote `main` at `213c44dc8`, including the approved brand, Composer and CLI changes. Git history identifies the subsequent reading and Context Dock batch described below.

This user-requested file is the maintained progress and handoff record for the desktop UI work. Update its sections in place as the product changes. Git history retains the chronology; keep progress organized by product area rather than numbered rounds or additional progress files.

## Current position

The shared presentation foundation and many reading and navigation defects have been repaired. The overall product has not yet reached the user's desired quality. Continue improving **visual refinement, typography and layout, and interaction design together**, with the center conversation and right Context Dock as the first priority.

The UI Goal has resumed in this chat. This continuation has improved the shared reading hierarchy and file-inspection path, repaired the native Dock background and boundary reports, unified main/delegated message rendering, and made tool outcomes visible before opening their details. The broader visual goal remains open. Historical paused or blocked statuses in earlier chats do not describe the current task.

The original checkout is `/Users/tangerg/Desktop/flame`, on branch `main`. All former local and remote branch tips were checked for ancestry before consolidation; their commits are retained in `main`. Only `main` remains locally and on `origin`. Read current Git state before starting; this document is a checkpoint, not an authority that can overwrite newer work.

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

For color-system work, retain the user's [Radix Colors](https://www.radix-ui.com/colors) and [HeroUI colors](https://heroui.com/en/docs/react/getting-started/colors) references. The default brand color is **Flame Orange `#FB6E30`**; users can choose their own interface accent through the existing theme owner. Approved brand artwork retains its original color. Avoid recreating the palette, appearance store or brand identity.

The older review at `/Users/tangerg/Desktop/flame/desktop/docs/Flame-Desktop-UI-Review-ZCode-2026-09-23.md` contains historical observations and reference pointers. Revalidate its findings against current code and behavior before using any item as a new task.

## Implemented foundation to retain

| Product area                  | Current behavior and ownership                                                                                                                                                                                                                                                                                         |
| ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Shared presentation           | Panels, persistent controls, resource groups, disclosures, forms, modal geometry, metric summaries and semantic badges share presentation owners. The host owns appearance values; `/Users/tangerg/Desktop/flame/ui` owns reusable presentation rules. Pages compose those rules.                                      |
| Color and brand               | The default accent is Flame Orange. Content headings, references, list markers, quote rules and inline code use semantic accent roles while body text keeps readable neutral ink. Theme customization and the approved brand assets already exist.                                                                     |
| Prose and layout              | Prose takes one reading step above interface text: 16 px with the default 14 px interface, while code keeps its separate preference. Paragraphs use one gap; headings have unitless leading and a visible following gap, including deeper levels. The `65ch` cap and wider code/table/media measures remain intentional. |
| Markdown tables               | Long tables stay within a bounded reading viewport. Compact table actions follow the table edge and wrap when necessary. Expanded previews keep controls and column headings reachable. Do not reintroduce page overflow or remove access to long data.                                                                |
| Code and recorded output      | One stable bounded code-reading region retains literal selection, scroll and focus as syntax highlighting arrives. Shared captions, wrapping and copy behavior are established. Loading messages no longer expose raw Markdown source as a temporary presentation.                                                     |
| Selection and file references | Ordinary action-button drags cannot select the surrounding application. Reading text and inline file references remain selectable. Dragging a file reference preserves selection without navigating the Dock; ordinary clicks and Enter/Space still open its path and line. Native copy and whitespace dismissal work. |
| Tool rows                     | Rows reserve action space only when real actions exist. Recorded paths wrap at directory boundaries and retain literal copy. Expanded groups remain visible and clickable as the Dock narrows; the WebKit grid sizing repair preserves reversible transitions.                                                         |
| Context Dock                  | File identity and return navigation precede a separate wrapping mode/action row. Full directories remain available beneath the filename. Opening from the tree focuses the return action; returning restores the tree cursor without stealing another region's focus. Panels grow through their complete content; fixed pane seams paint above child surfaces. |
| Subagent stop controls        | Shared filled stop marks and quiet circular controls replace the checkbox-like outline squares. Their placement is consistent across the narrative, Dock list and detail view; terminal entries reserve no dead cancel slot.                                                                                           |
| Main and delegated messages   | Both use `MessageTurn` and `MessageBlock` for spacing, prose, bubbles, long-input folding, tools, code, tables, media and message actions. Child rows retain their own Run and referenced facts. Their overview keeps its scroll position; returning restores the originating task's focus. |
| Motion and focus              | Shared moving item backplates remain interruptible and reversible and consume the host motion tokens. Reduced motion, scoped code Select All, native dismissal and focus return have established owners and regression coverage.                                                                                       |
| Authoring and acceptance      | One captured submission owns draft consumption and history. Rejected input stays editable; delayed acceptance preserves successor revisions, Session changes, attachment staging and other controls' focus. Goal and slash extensions share this settlement contract.                                                  |

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

## Verification checkpoint

For shared tool-outcome summaries:

- The complete frontend `npm run check` passed: 373 test files and 2719 tests, including type, lint, formatting, unused-code, boundary, token, style, bootstrap, locale, resource, bundle-budget and production-module checks. Focused checks passed all 64 tests. Regression coverage exercises folded wave outcomes, running-to-failed summary updates without overriding a user's fold, single-count semantics and failure marks without error text.
- Shared UI `npm run check` passed 38 tests and its independently packed consumer; portable-plugin `npm run check` passed all ten resource checks. The eight locales have complete summary labels. No shared-package or portable-resource source changed.
- The supported Wails production macOS arm64 build passed with deployment floor 13.0. Actual Wails WKWebView computer use checked Chinese/light/default-size and 18 px scenes, English/dark/18 px scenes, normal and approximately 1120 px native windows, and a pointer-resized 320 px Dock.
- Native inspection confirmed that the folded wave retains two problems and one refusal, expanding reveals the corresponding nonzero command exit, failed patch and refused operation, and Space/Enter toggles retain summary focus. The status line remains available while scrolling detailed output. Successful waves keep their compact single-line summary beside prose and live tool records. The existing sidebar-matching Dock wash is unchanged.
- No visual/browser suite was run. The native app exited successfully; the preview, disposable data, bundle, build logs, fixture output and owned WebKit caches were removed. User reference caches and unrelated acceptance resources were retained. Go, Runtime and CLI sources remain unchanged, so their earlier module results below remain applicable.

For shared main/delegated narrative rendering:

- All 372 frontend test files and 2715 tests passed. The gate completed in stages: after removing an obsolete public export found by Knip, type, lint, formatting, unused-code, boundary, token, style, bootstrap, resource, bundle-budget and production-module checks passed. Focused final projection, subagent and main-stream checks passed all 23 tests. The final status-pill style passed formatting, lint and token/style/class/shorthand checks; it does not change message behavior.
- Regression coverage now exercises child fact isolation, shared long-input folding, overview scroll preservation and return focus, and the absence of a main-conversation replay command for a child answer. The existing menu fixture now supplies its main-conversation membership; assertions were not removed to hide a failure.
- Shared UI `npm run check` passed 38 tests and its packed consumer; portable-plugin `npm run check` passed all ten resource checks. No shared-package or portable-resource source changed.
- The supported Wails production macOS arm64 build passed with deployment floor 13.0. No Go, Runtime or CLI source changed; their earlier module results below remain the latest tests for those unchanged sources.
- Actual Wails WKWebView computer use checked Chinese/light/default-size and English/dark/18 px interface scenes, normal and approximately 1120 px native windows, and pointer-resized 320 px Dock. Shared child user folding, mixed-language prose, deeper headings, bounded tables, code, diagrams, images, copy menus and running tool disclosure were inspected. The final large-text status pill stayed on one line at the narrow native window.
- Native interaction confirmed that opening a task focuses Back, returning to a scrolled overview preserves its position and focuses the originating task, and both main and child latest-content actions return focus to their own reader. Main and child scrolling remained independent. The approved sidebar-matching Dock wash was retained without new seam or hover styling.
- No visual/browser suite was run. The native app exited successfully; the preview, disposable data, bundle, build logs, fixture output and owned WebKit caches were removed. User reference caches and unrelated acceptance resources were retained.

For the preceding reading and file/Diff inspection work:

- The complete frontend `npm run check` passed: 372 files and 2711 tests, including the shared Panel sizing change. Final shell styling then passed type, lint, formatting, design-token, bootstrap, authored-class, dead-style, shorthand, CSS-variable, bundle-budget and production-module checks. The file-navigation regression asserts entry focus, return focus and preservation of focus in another region.
- Shared UI `npm run check` passed all 38 tests and the independently packed consumer. Portable views were rebuilt and `npm run check` passed all ten resource checks. Initially stale local icon output was rebuilt from unchanged source before the successful checks; no icon or portable-resource source changed.
- Desktop `go test ./...` and `go vet ./...` passed. The final supported Wails production macOS arm64 build passed with deployment floor 13.0. Runtime and CLI code did not change; their earlier baseline checks below remain the latest results for those modules.
- Actual Wails WKWebView acceptance used the isolated production-component fixture and a disposable data directory. Chinese/light/default-size and English/dark/18 px interface scenes were reviewed at the normal 1440 px width and an approximately 1120 px native window. Prose, mixed CJK/English, nested lists, deeper headings, tables and independent code sizing were inspected together.
- Native file inspection verified entry focus on Back, Tab into Preview, arrow-key Source selection, keyboard return to the selected tree item, complete long-directory display and successful path-copy feedback. Dock width was adjusted both with the keyboard to 320 px and with actual pointer dragging. macOS Option+Tab reaches the custom separator under the host's current tab-navigation policy; no system preference was changed.
- The reported Diff background cutoff was reproduced before the Panel fix and checked after it in both themes, including long scrolling and expanded comparison details. The final divider was dragged in the actual app: it uses the left sidebar's full-plane wash, with no separate line or hover/drag darkening. The left sidebar itself retains its existing appearance.
- Per the user's updated direction, no visual/browser suite qualifies this batch. An earlier exploratory browser run did not pass (5 passed, 7 failed); its newly added probes had invalid selectors/navigation assumptions and were removed. Existing prose-size and file-header expectations were aligned with the intentional changes; those suites were not rerun. Native computer use supplies the visual and interaction acceptance.
- The temporary native app exited successfully. Its preview service, disposable data, bundle, build logs, WebKit caches, fixture build and owned exploratory results were removed. User reference caches and unrelated pre-existing acceptance files were retained.

For the verified combined baseline:

- The complete frontend `npm run check` passed: 372 test files and 2710 tests, plus type, lint, formatting, boundaries, design-system, bundle and production-module checks. The WebKit clipboard-permission correction in the brand browser test also passed focused lint, formatting and type checks.
- CLI `go test ./...`, `go vet ./...` and `go build ./...` passed with the repository workspace enabled. The initial combined run timed out in the unchanged acceptance-recovery integration case; that case passed in isolation and the subsequent complete CLI gate passed without changing assertions or deadlines.
- Twenty Chromium/WebKit browser scenarios passed, covering light/dark brand assets at 1120px and 320px, settings search, HEX copying, PNG download, steer/stop admission, compact Goal mode, model/image capability handling and file-reference keyboard navigation.
- Earlier shared reading verification remains relevant to its unchanged owners: the UI package passed 38 tests, portable-plugin checks passed, and prior browser/native checks qualified path dragging, literal copying, whitespace selection dismissal, table bounds and reading geometry. The latest combined gate also includes the current source for these owners.
- The supported production macOS arm64 build passed with deployment floor 13.0. The pinned desktop host is Wails v3, not v2.
- Actual Wails WKWebView acceptance used an isolated in-memory visual fixture with Chinese UI. The three approved brand images and exact color values rendered, the copy action showed focused feedback, native paste and Cmd+A/BackSpace cleared the draft, and Enter on `/goal` cleared the invocation and entered compact mode while retaining Composer focus. No real Runtime configuration or credentials were read.
- The native app exited successfully. Owned preview services, browser sessions, native bundles, caches, temporary build outputs and managed review worktrees were cleaned. Do not assume an old preview or review app is still available.

These checks cover the changed contracts and representative native scenarios. They do not establish that every page, dialog, layout or interaction meets the final design goal. No known product failure remains in the changed paths; unrun browser suites are not reported as passing.

## Next work: refine the product, layout and interactions

1. **Extend the task comparison.** Reading-to-file-and-back and overview-to-subagent-and-back now have shared presentation and explicit focus ownership. Native tool inspection still opens the current file in the Dock while leaving keyboard focus on its conversation opener. Review this explicit activation and its return route without breaking the file workspace's intentional outside-focus guard for background updates. Do not restart the implemented typography, file header, shared message renderer or sidebar-matching divider.
2. **Continue the reading composition around activity.** The shared prose step, paragraph/heading rhythm, disclosure affordance and folded tool-outcome summaries are implemented. Review how recorded evidence and media join an answer, especially where a summary leads into a dense detail view. Preserve the existing bounded table, code and selection owners.
3. **Refine the remaining Dock forms.** File identity/actions and delegated message reuse, overview continuity and return focus are established. Continue with richer tool details and cross-view navigation: decide which details belong inline or in a persistent pane, and reserve popovers/dialogs for genuinely bounded tasks. Keep the user-requested sidebar-matching divider unchanged, including hover and drag.
4. **Extend native constraint coverage as content changes.** Normal/narrow native windows, 320 px Dock, default/large text, long directories and both themes were reviewed for file/Diff and selected-subagent paths. Apply comparable checks to the next changed surfaces rather than rerunning broad visual suites or claiming every page is covered.
5. **Continue across other pages.** Carry the same layout and interaction grammar into settings, installed plugin pages, forms, resource lists and dialogs. Retain the approved brand and the implemented Composer contracts while checking how the surrounding surfaces fit them.

Build a coherent batch from observed problems. Separate reproduced defects from design judgments and unverified hypotheses. Do not invent a defect to justify another small change, or substitute isolated icon, color and radius adjustments for a better page and task flow.

## Working and maintenance practice

- Read `/Users/tangerg/Desktop/flame/AGENTS.md`, `/Users/tangerg/Desktop/flame/PROJECT_RULES.md`, `/Users/tangerg/Desktop/flame/DEVELOPMENT.md`, the target module README and applicable nested instructions. This progress file does not replace them.
- The primary agent should implement directly. The user retired Claude CLI assistance to preserve quota; do not carry forward the obsolete delegation instructions from older handoffs.
- Apply relevant better-series skills and inspect the references in depth. Preserve StyleX, Base UI, semantic tokens and one fact/one owner. Runtime owns durable product semantics; layout work does not authorize rebuilding those state machines in the client.
- Batch meaningful changes, then review their combined visual and interaction result. Prioritize development and actual native computer use. The user explicitly asked not to run visual/browser suites and to avoid meaningless tests; retain necessary regression coverage and module checks, and reuse valid results when source and dependencies have not changed.
- UI builds clear shared output, and frontend `precheck` rebuilds it. The supported Wails task can also reinstall frontend dependencies. Finish these operations before module scripts and native acceptance; overlapping them produces misleading missing-resource or dependency errors.
- Portable renderer changes include rebuilding and checking their published ESM resources. A source or resource rebuild does not update an existing Runtime installation; verify the actual selected resource when that installation is in scope.
- When the user requests the frontend, they mean the desktop app. Use actual Wails acceptance for desktop behavior. Visual review uses isolated fixtures; an explicitly requested live model check uses the user's existing configuration without printing or copying credentials.
- If the Mac is locked, advance independent source, reference and build work, and finish native acceptance after unlock. A native build does not prove native interaction.
- Commit explicit owned paths, push verified milestones to remote `main`, preserve unrelated working files during integration and clean owned resources. Update this file with the new product state and verification scope rather than starting another round log.
- Keep user-facing replies in Chinese and repository content in English. The overall UI goal remains open until the user accepts it or asks to stop.
