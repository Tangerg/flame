# Desktop UI progress and handoff

Updated: 2026-10-19. Verified UI code baseline: `8d363c859542dd9a76d57d372543405fb67ad9e5`, pushed to remote `main` and integrated into the original checkout. Documentation commits may follow this baseline.

This user-requested file is the maintained progress and handoff record for the desktop UI work. Update its sections in place as the product changes. Git history retains the chronology; keep progress organized by product area rather than numbered rounds or additional progress files.

## Current position

The shared presentation foundation and many reading and navigation defects have been repaired. The overall product has not yet reached the user's desired quality. Continue improving **visual refinement, typography and layout, and interaction design together**, with the center conversation and right Context Dock as the first priority.

The previous chat's Goal was paused at the user's request to continue in a new chat. Its earlier Mac-lock blocker was resolved, and actual native acceptance of the latest UI code was completed. Historical blocked statuses in temporary logs do not describe unfinished native acceptance.

The original checkout is `/Users/tangerg/Desktop/flame`, on branch `codex/report-root-fixes` at this handoff. Read current Git state before starting; this document is a checkpoint, not an authority that can overwrite newer work.

## Preserve the user's unfinished working tree

At handoff, the original checkout had **76 modified, deleted or untracked files**, with an empty index. They were preserved across the latest integration, including file bytes, symlink targets and deletions. The latest UI checks qualified isolated committed code; the complete dirty working tree was not tested as one combined release.

The user has confirmed that the brand work is ready. Preserve that accepted direction even though its files remain uncommitted:

- `/Users/tangerg/Desktop/flame/brand` contains the primary mark, dimensional companion, plush character and usage guidance.
- `/Users/tangerg/Desktop/flame/design/branding` contains concept and refinement assets.
- `/Users/tangerg/Desktop/flame/desktop/frontend/src/lib/brand.ts` and `/Users/tangerg/Desktop/flame/desktop/frontend/src/lib/brand.stylex.ts` connect the assets and brand color.
- `/Users/tangerg/Desktop/flame/desktop/frontend/src/plugins/builtin/settings/brand` and `/Users/tangerg/Desktop/flame/desktop/frontend/visual/brandSettings.browser.spec.ts` belong to the brand work.

Independent input and execution-control work also remains under `/Users/tangerg/Desktop/flame/desktop/frontend/src/plugins/builtin/chat/composer` and `/Users/tangerg/Desktop/flame/desktop/frontend/src/plugins/builtin/chat/goal`, with related shell, SDK, locale, fixture and test changes. The deletion of `/Users/tangerg/Desktop/flame/desktop/frontend/src/plugins/builtin/chat/composer/application/focus.ts` is part of that work; do not restore it from the committed baseline.

Preserve the other existing CLI terminal, documentation, appearance, plugin-registration and bundle-check changes as well. Inspect the entire current status and relevant diffs before editing. Keep unrelated work outside UI commits, avoid bulk formatting or staging, and verify preservation immediately before and after integration.

An isolated worktree starts without these uncommitted files. A missing brand pane or asset there is not evidence that the approved brand work must be implemented again.

## Reference products and example images

Deeply study the two desktop references before choosing the next implementation. Read their actual layout, component and interaction code, and inspect visual evidence. Naming a reference or borrowing a few colors is insufficient.

| Reference | Where to look | What to compare |
| --- | --- | --- |
| ZCode | `/Users/tangerg/Desktop/ZCode/packages/ui/src/app-shell`, `/Users/tangerg/Desktop/ZCode/packages/ui/src/ToolCallBlocks`, `/Users/tangerg/Desktop/ZCode/packages/ui/src/v4`, and `/Users/tangerg/Desktop/ZCode/packages/ui/src/settings` | Workspace proportions, reading rhythm, side-pane behavior, resizing, tool summary/detail transitions, contextual actions and settings composition. |
| ChatGPT desktop reference | `/Users/tangerg/Desktop/study/chatgpt/deob/webview` and `/Users/tangerg/Desktop/study/chatgpt/extracted` | Conversation composition, typography, spacing, progressive disclosure, contextual surfaces, focus and keyboard behavior. This is an extracted reference; establish what each source or asset represents. |
| HeroUI | `/Users/tangerg/Desktop/heroui` | Shared control surfaces, optical alignment, state treatment and semantic color roles, adapted to desktop density. |

The user also explicitly requested the Ace Design examples:

- [Alpaca](https://acedesign.io/work/alpaca). Cached overview: `/tmp/flame-alpaca-reference-8f0lz7l0/contact-sheet.jpg`.
- [Marketer](https://acedesign.io/work/marketer). Cached overview: `/tmp/flame-marketer-reference-3epzp5i1/contact-sheet.jpg`.

Both cache directories existed at handoff and contain the full PNGs, a manifest and the source page. Open the overview and relevant full-size images. Compare proportions, grouping, whitespace and information hierarchy; if a temporary cache disappears, revisit the source page. Reference repositories and artwork remain read-only material.

For color-system work, retain the user's [Radix Colors](https://www.radix-ui.com/colors) and [HeroUI colors](https://heroui.com/en/docs/react/getting-started/colors) references. The default brand color is **Flame Orange `#FB6E30`**; users can choose their own interface accent through the existing theme owner. Approved brand artwork retains its original color. Avoid recreating the palette, appearance store or brand identity.

The older review at `/Users/tangerg/Desktop/flame/desktop/docs/Flame-Desktop-UI-Review-ZCode-2026-09-23.md` contains historical observations and reference pointers. Revalidate its findings against current code and behavior before using any item as a new task.

## Implemented foundation to retain

| Product area | Current behavior and ownership |
| --- | --- |
| Shared presentation | Panels, persistent controls, resource groups, disclosures, forms, modal geometry, metric summaries and semantic badges share presentation owners. The host owns appearance values; `/Users/tangerg/Desktop/flame/ui` owns reusable presentation rules. Pages compose those rules. |
| Color and brand | The default accent is Flame Orange. Content headings, references, list markers, quote rules and inline code use semantic accent roles while body text keeps readable neutral ink. Theme customization and the approved brand assets already exist. |
| Prose and layout | Paragraphs, lists and quotations use a `65ch` reading cap with direction-aware alignment. Code, tables and media retain the host's available width. English, CJK and RTL cases have been checked. Keep these different content measures intentional. |
| Markdown tables | Long tables stay within a bounded reading viewport. Compact table actions follow the table edge and wrap when necessary. Expanded previews keep controls and column headings reachable. Do not reintroduce page overflow or remove access to long data. |
| Code and recorded output | One stable bounded code-reading region retains literal selection, scroll and focus as syntax highlighting arrives. Shared captions, wrapping and copy behavior are established. Loading messages no longer expose raw Markdown source as a temporary presentation. |
| Selection and file references | Ordinary action-button drags cannot select the surrounding application. Reading text and inline file references remain selectable. Dragging a file reference preserves selection without navigating the Dock; ordinary clicks and Enter/Space still open its path and line. Native copy and whitespace dismissal work. |
| Tool rows | Rows reserve action space only when real actions exist. Recorded paths wrap at directory boundaries and retain literal copy. Expanded groups remain visible and clickable as the Dock narrows; the WebKit grid sizing repair preserves reversible transitions. |
| Context Dock | Shared panels adapt to Dock presentation. File headers, path wrapping, copy feedback, recorded change counts, selected subagent context and navigation focus have been refined. Preserve the distinction between recorded evidence and current workspace files. |
| Subagent stop controls | Shared filled stop marks and quiet circular controls replace the checkbox-like outline squares. Their placement is consistent across the narrative, Dock list and detail view; terminal entries reserve no dead cancel slot. |
| Motion and focus | Shared moving item backplates remain interruptible and reversible and consume the host motion tokens. Reduced motion, scoped code Select All, native dismissal and focus return have established owners and regression coverage. |

The most recent behavior lives in `/Users/tangerg/Desktop/flame/desktop/frontend/src/plugins/builtin/chat/file-references/public/FileRefLink.tsx`: pointer activation consults the browser's existing selection; keyboard activation remains available. It introduces no separate selection state. Read current callers and coverage before changing this boundary.

## Verification checkpoint

For the verified UI baseline:

- The complete frontend `npm run check` passed: 372 test files and 2693 tests, plus type, lint, formatting, boundaries, design-system, bundle and production-module checks.
- Browser coverage included 36 related Chromium/WebKit scenarios across light and dark themes. Thirty-five passed in the initial run. One run was interrupted by rebuilding shared UI output while Vite was reading it; after restarting the preview, all six focused gesture and keyboard checks, including the affected case, passed.
- The preceding shared reading change passed the UI package check with 38 tests, the portable-plugin checks and 48 existing Chromium/WebKit checks. Those results qualify that committed change, not subsequent user WIP.
- The supported production macOS arm64 build passed with deployment floor 13.0. The pinned desktop host is Wails v3, not v2.
- Actual Wails WKWebView acceptance used an isolated in-memory visual fixture with Chinese UI. Short and wrapped path selections retained the file list, native copy/paste preserved the literal, whitespace cleared selection, a click opened README line 15, and Tab/Space opened the next reference.
- The native app exited successfully. Owned preview services, browser sessions, native bundles, caches, temporary build outputs and managed review worktrees were cleaned. Do not assume an old preview or review app is still available.

These checks cover the changed contracts and representative native scenarios. They do not establish that every page, dialog, layout or interaction meets the final design goal. There is no outstanding known failure introduced by the latest committed UI fix.

## Next work: refine the product, layout and interactions

1. **Compare complete tasks with the references.** Trace a conversation from reading a response through inspecting a tool/file/subagent in the Dock and returning to work. Compare shell proportions, reading order, density and action placement using actual implementations and the supplied images.
2. **Improve the center reading area as a composition.** Review line lengths, type scale, paragraph and heading rhythm, nested content, tables, code, tool groups and media together. Give content room to breathe while keeping a clear hierarchy. Resolve structural spacing and alignment at the responsible shared owner.
3. **Refine the Dock's product form.** Decide how summaries lead to details, when a task uses inline expansion, a panel, a popover or a dialog, and how users navigate back. Review tabs, panel headers, filters, action visibility, resizing, scrolling and focus as one interaction flow. More cards or decorative controls do not by themselves improve this flow.
4. **Check layout under real constraints.** Review different window and Dock widths, body/code font sizes, long paths, translated labels and light/dark themes. Distinguish intentional code/table scrolling from unintended page overflow. Examine optical alignment and whitespace in the actual desktop host.
5. **Continue across other pages.** Carry the same layout and interaction grammar into settings, installed plugin pages, forms, resource lists and dialogs. Preserve the user's brand and Composer work while checking how the surrounding surfaces fit it.

Build a coherent batch from observed problems. Separate reproduced defects from design judgments and unverified hypotheses. Do not invent a defect to justify another small change, or substitute isolated icon, color and radius adjustments for a better page and task flow.

## Working and maintenance practice

- Read `/Users/tangerg/Desktop/flame/AGENTS.md`, `/Users/tangerg/Desktop/flame/PROJECT_RULES.md`, `/Users/tangerg/Desktop/flame/DEVELOPMENT.md`, the target module README and applicable nested instructions. This progress file does not replace them.
- The primary agent should implement directly. The user retired Claude CLI assistance to preserve quota; do not carry forward the obsolete delegation instructions from older handoffs.
- Apply relevant better-series skills and inspect the references in depth. Preserve StyleX, Base UI, semantic tokens and one fact/one owner. Runtime owns durable product semantics; layout work does not authorize rebuilding those state machines in the client.
- Batch meaningful changes, then review their combined visual and interaction result. Use proportionate regression coverage and the required module gates. Reuse valid results when source and dependencies have not changed; tests alone are not visual acceptance.
- UI builds clear shared output, and frontend `precheck` rebuilds it. Finish those builds before browser/native acceptance. Running Vite or portable checks against a concurrently cleared output directory produces misleading resource failures.
- Portable renderer changes include rebuilding and checking their published ESM resources. A source or resource rebuild does not update an existing Runtime installation; verify the actual selected resource when that installation is in scope.
- When the user requests the frontend, they mean the desktop app. Use actual Wails acceptance for desktop behavior. Visual review uses isolated fixtures; an explicitly requested live model check uses the user's existing configuration without printing or copying credentials.
- If the Mac is locked, advance independent source, reference, headless browser and build work, and finish native acceptance after unlock. A native build or headless browser run does not prove native interaction.
- Commit explicit owned paths, push verified milestones to remote `main`, preserve unrelated working files during integration and clean owned resources. Update this file with the new product state and verification scope rather than starting another round log.
- Keep user-facing replies in Chinese and repository content in English. The overall UI goal remains open until the user accepts it or asks to stop.
