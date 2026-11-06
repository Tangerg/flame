# Component lab

The lab is a development entry in the existing visual fixture application. It
renders production components, with two searchable catalogs: shared foundations
and product components. Product scenarios reuse the existing Agent, Workspace,
Settings and Sidebar fixtures. The production router does not import this entry.

## Run

Complete the repository's normal `make setup` prerequisites first. From the
repository root:

```sh
npm run lab --workspace @flame/web
```

Vite binds to `127.0.0.1:4174` and opens `/visual/lab/`. `browser:dev` uses the
same server and port; run one of these commands at a time.

Direct entry points:

- `/visual/lab/?panel=foundation`
- `/visual/lab/?panel=business`
- `/visual/lab/?story=text-field&variant=styles&mode=inspect`
- `/visual/lab/?story=checkbox&variant=editable&mode=inspect&compare=on`
- `/visual/lab/?panel=business&story=business-agent-scenarios&variant=waiting&mode=inspect`

The catalog owns the valid story and variant IDs. An invalid deep link displays
its error instead of silently opening a different example. Copy current link
captures the selected story, variant, viewport, theme, language, motion and view
mode. Selection is URL state; the lab persists each panel's camera. The shared
appearance provider retains its existing storage behavior.

Shared UI and icons are consumed through their normal compiled public packages.
Vite updates application source and lab examples. After editing a shared package,
rebuild its normal output and reload the preview:

```sh
npm run build --workspace @flame/icons
npm run build --workspace @flame/ui
```

The icons build is necessary only when its source changed or its output is
missing. The lab does not alias private component source around package exports.

To build a static preview alongside the existing visual fixture entry:

```sh
npm run lab:build --workspace @flame/web
```

This writes `out/web/visual`. It does not replace the production Web bundle.

## Use

Gallery pages mount at most six interactive previews. Search matches component
names, groups, scene IDs, descriptions and source paths. Selecting a component
opens its focused preview. The right panel selects a named comparison or recorded
scenario and links to the actual component source.

Focused previews support component, phone, tablet and desktop widths. Theme
comparison mounts two independent documents with the same scenario. Changing
the scenario, theme, language or motion reloads that document; changing its
viewport preserves the current input and component instance. Reload example is
an explicit, separate action.

The camera supports background drag, background touch pan/pinch and
Ctrl/Command-wheel zoom around the pointer. Focus the background to use arrow
keys, `+`, `-` and `Home`. Fit and 100% centering remain reachable outside the
transformed content. Camera operations never reset component drafts. Each panel
stores one bounded, versioned camera value; malformed or unavailable storage
does not prevent previewing.

The observation panel captures real DOM activations, explicit sample callbacks,
rendering errors and unhandled failures. It retains only the latest bounded
in-memory log. Text drafts report their length; credential-shaped fields are
redacted. Callback enums, booleans and numbers remain inspectable. Clearing the
log does not change the component. The log is an interaction aid, not a Runtime
trace, profiler or command receipt store.

## Ownership and isolation

`foundationCatalog.ts` and `businessCatalog.ts` are the composition inputs to one
validated `StoryCatalog`. Navigation, search, counts, variant selectors, preview
URLs and source links are projections of those definitions. The catalog copies
and freezes its descriptions at construction. Production components do not
import the lab.

Each component example owns only its local input, selection or disclosure state.
Its variant vocabulary is checked against the production public API. Examples
compare one dimension at a time instead of generating every props combination.
Examples do not pretend that a recorded failure has recovered after a Retry click.

`environment.tsx` composes Flame's actual appearance plugins, locale packages,
Theme, FontScale, Tooltip, Callout and code highlighting providers. It does not
define another palette or translated production dictionary. Both the original
visual fixtures and the lab consume the shared stylesheet and provider contracts.

`fixture.ts` observes and imports the existing visual entry. It never rebuilds a
Session, Run, Goal or installed-component state machine. Fixture variants derive
from the existing `VISUAL_*` arrays, and their existing in-memory transport rejects
unmodelled Runtime operations. Clipboard, download and other browser interactions
retain their existing explicit behavior.

An iframe provides a separate document, module graph, portal root and focus
lifetime. It is not a security sandbox. The lab runs trusted repository examples;
it is not an editor for arbitrary HTML or JavaScript. Native component portals
stay in their preview document, so transforming the entire document keeps their
coordinates aligned without patching component DOM or browser APIs. Open preview
offers an ordinary untransformed document when native-size inspection is useful.

The observation bridge only sends bounded messages to its same-origin parent.
The parent binds each message to the exact active iframe, origin and instance ID
before accepting it. There is no inbound command channel. Listener, pointer,
animation-frame, observer and preview lifetimes retire with their owner.

The lab has no virtual animation clock. Normal/reduced motion uses the production
motion preference and the platform's reduced-motion setting. Recorded business
fixtures retain their existing clock contract.

## Extend

Add a typed `ComponentStory` to the relevant catalog and a colocated example
component. A definition supplies its identity, localized description, group,
source path, natural viewport, nonempty variants and lazy loader. The loader
returns a component accepting `LabExampleProps`; use its `observe` callback for
meaningful public interactions. Free-text inputs must report length rather than
duplicating the draft in the inspector.

Keep samples cohesive by the component family they exercise. Reuse production
components directly. Do not add a wrapper, production prop or compatibility
branch merely to make a sample possible. Register complete business scenarios
with `FixtureStory` targets from the existing fixture vocabulary.

After changing a story, verify its real interactions, both themes, a narrow
viewport and any native popover/modal focus path. Public components may offer
more combinations than the named examples; these examples are purposeful
comparisons, not a claim of exhaustive combinatorial coverage.

## Verify

```sh
npm run typecheck --workspace @flame/web
npm run test --workspace @flame/web -- visual/lab
npm run browser:test --workspace @flame/web -- visual/lab/lab.browser.spec.ts
npm run lab:build --workspace @flame/web
npm run check --workspace @flame/web
```

Install the browser matching the repository's Playwright version before running
browser tests. Browser tests use the existing fixture server, real components
and its console/error guard. The normal Web check does not replace native Wails
verification on macOS.

## Design references

- [Cards](https://cards.uidesigncourse.com/) and its
  [source](https://github.com/oliur/cards): a navigable component gallery and
  consistent design-system presentation.
- [Component Canvas](https://www.ui-skills.com/skills/ibelick/component-canvas):
  real component behavior, single-dimension comparisons, bounded camera input
  and preserving native preview interactions.
- [Stageshow](https://github.com/benjitaylor/skills/tree/main/stageshow): focused
  scene inspection, document isolation and explicit reload controls. Its faux
  product UI and virtual-clock implementation are not used here.

The implementation is authored for Flame using its existing dependencies. No
third-party component collection, reference-project assets or skill runtime is
vendored.
