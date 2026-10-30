# Desktop UI Polish

This document captures two recurring quality bars that are easy to lose when a
React WebView app evolves quickly:

- surface depth should come from a deliberate shadow model, not gray borders or
  one large blurry shadow;
- Flame should feel like a desktop app that happens to use web technology, not a
  website embedded in a window.

Background references (rationale, not Flame's current boundary specification):

- [高级感 UI 的边缘为什么看起来舒服：用 box-shadow hairline 解决卡片边缘发灰的问题](https://www.uweb.net.cn/zhishiku/wangzhanqianduankaifa/36454.html)
- [A Technical Deep Dive Into the New Raycast](https://www.raycast.com/blog/a-technical-deep-dive-into-the-new-raycast)

## 1. Shadow Model

The source article's useful idea is not "add stronger shadows". It is a layered
model:

1. **Edge**: a 1px optical boundary.
2. **Contact shadow**: a tiny near shadow that says the surface touches the
   background.
3. **Ambient shadow**: a wide, low-alpha falloff that gives elevation without a
   muddy border.

A shadow ring stacked on a border reads as a double edge. Flame's current
[Boundary roles](DESIGN.md#boundary-roles) specifies how each surface applies
this principle and identifies the owner of its geometry. The source article's
1px example is background rationale, not a requirement to outline every surface.

Inspect the computed shared token on the affected surface. A feature-local shadow
value bypasses the visual style; a palette-specific darkness belongs to the
theme's `--shadow-cast`.

## 2. Border Discipline

An edge should explain structure, focus or state. Review the surface against
[Boundary roles](DESIGN.md#boundary-roles), distinguishing an outer flank from an
internal seam or a control. Do not infer one treatment for all three from a shared
word such as "boundary".

Compare the expanded and collapsed sidebar, an open Dock, scrolling content,
hover, dragging and keyboard resize. Check the computed shared rule as well as
the image so a duplicate edge cannot hide behind a similar fill.

Avoid:

- a border AND a shadow edge ring on the same surface (double edge);
- gray borders that are only compensating for weak elevation;
- more than one line at a boundary — two 1px semi-transparent lines sharing a pixel
  double their alpha and read as a bright dot;
- decorative outlines that do not communicate structure, focus, or state.

## 3. Native WebView Feel

The Raycast article's key distinction is architectural and behavioral: a desktop
app may use web technology for UI, but it should not inherit website defaults.

Rules for Flame:

- Do not use `cursor: pointer` as a default marker for controls. Desktop apps do
  not need the browser hand cursor everywhere.
- Do not add hover highlights just because an element is interactive. Hover is
  reserved for dense lists, sidebar rows, icon buttons, and controls where it
  improves scanability.
- Glass blur is material, not decoration. It belongs on exactly two things:
  floating panels (menus, popovers, tooltips) and the composer with its top tray —
  the two places something is genuinely moving underneath, where a hint of what is
  covered is what makes them read as above it. Everywhere else, including modal
  dialogs, tokenized surfaces and shadows. Each of the three is a token
  (`--floating-backdrop`, `--composer-backdrop`, `--composer-tray-backdrop`); a raw
  `backdrop-blur-*` is how a fourth appears.
- Prefer native-feeling immediacy: no flicker, no layout jumps, no delayed
  reveal that makes a persistent surface feel like a web page entering.
- Popovers, command palette, tooltips, and composer overlays should read as
  application chrome: compact, anchored, keyboard-safe, and using the same
  elevation tokens.

## 4. Practical Checklist

Before merging UI polish work, inspect these points:

- Does the surface follow its [boundary role](DESIGN.md#boundary-roles), without
  an additional feature-local edge?
- Is the shadow token semantic enough for reuse, or is it a one-off arbitrary
  class?
- Does the component still work in light and dark themes?
- Does the UI rely on blur, opacity, or low contrast to feel "premium"?
- Are primary labels clear enough on a neutral sidebar or panel surface?
- Are hover states restrained and purposeful?
- Are interactive controls keyboard/focus safe through Base UI or existing common
  primitives?
- Did the change preserve the plugin system boundary instead of reaching around
  registry/slots?

## 5. Anti-Patterns

These are regressions:

- adding a new hardcoded shadow value inside a feature component;
- using border and shadow together without a clear reason for each layer;
- overriding the shared boundary role with a feature-local line or shadow;
- inverting a surface with `bg-fg` to make it stand out: it flips with the scheme,
  so the thing you emphasised becomes the brightest object on a dark palette and
  the only inverted one in the app (the approval card's command block did exactly
  this);
- making the light sidebar nearly white against a white canvas;
- adding `backdrop-blur` to make an ordinary panel feel expensive;
- increasing rounded corners to create perceived softness;
- adding hover backgrounds to every button, text link, and list item;
- styling a WebView like a marketing website instead of app chrome.
