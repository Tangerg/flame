# Flame icons

`@flame/icons` owns Flame's original drawings, optical sizing, SVG exports, React components, and the review preview. It is a private module in this repository. Product concepts and string-to-icon mappings remain in the consuming application.

Read [DESIGN.md](DESIGN.md) before adding or changing a drawing. Its primary reference is [The making of Cursor's icons](https://www.minoradventures.co/blog/the-making-of-cursors-icons), by Marek Minor of Minor Adventures.

## React API

```tsx
import { Brain, BrainOff, BellOff } from "@flame/icons/react";

<Brain />;
<BrainOff size={14} className="muted" />;
<button aria-label="Mute notifications">
  <BellOff size={16} />
</button>;
<BellOff size={24} aria-label="Notifications muted" />;
```

`size` is the actual square viewport in CSS pixels: a finite positive number, defaulting to 16. Sizes below 22 use the 16px optical master; sizes from 22 use the 24px master. Both drawings remain vector shapes. Inspect 9–36px in the preview; 9–11px is an application constraint that needs special care, rather than a promise that every detailed icon stays equally legible.

Color inherits through `currentColor`. Native SVG props, `className`, `style`, events, `data-*`, ARIA, and a React 19 SVG ref work normally. Decorative icons are hidden from accessibility APIs by default. `aria-label` or `aria-labelledby` makes an icon an image unless explicitly overridden; action controls should carry their own accessible name.

Geometry, strokes, children, and width/height attributes belong to the drawing. Use `size` for dimensions. CSS should change color, opacity, or layout rather than stroke weight, fill, or viewport dimensions. Separate named components express semantic states; `disabled` belongs to the control. The library has no provider, theme registry, runtime name lookup, stroke options, or speculative filled style.

## Authoring and build

```sh
cd icons
npm ci
npm run check
npm run preview
```

The preview runs at <http://127.0.0.1:8770>. Set `PORT` to choose another port. Search by component name or concept, filter by category, and inspect each exact master, automatic sizing, light/dark surfaces, context examples, and downloadable SVGs.

`source/16` and `source/24` contain the authored path-only SVGs. Base drawings and the shared `slash`/`strike` marks are the geometry owners. Circle, square, file, folder, book, calendar, clipboard, message, bookmark, and shield variants share their corresponding contours through `base` in the catalog and author only their internal details. `catalog.mjs` owns public names, family relationships, categories, and discovery metadata. The builder validates the source and composes complete standalone glyphs, then emits React modules, declarations, SVGs, catalog, and preview into ignored output. Generated files are never hand-edited or committed.

The collection contains 601 glyphs across 17 categories, including every application glyph, the reference article’s standalone construction concepts, and A–Z / 0–9 glyphs. Its 34 base/off pairs and `Strikethrough` share one cancellation language. `slash` and `strike` are private authoring inputs. Metadata is available separately from `@flame/icons/catalog`; SVGs use `@flame/icons/svg/16/bell-off.svg` or the corresponding 24px path. Named React imports are tree-shaken; the checks verify that importing `BellOff` does not include unrelated geometry or catalog data.

Consumers use the repository's existing npm `file:` convention. Build this module before installing or building `desktop/frontend`:

```sh
cd icons
npm ci
npm run build
cd ../desktop/frontend
npm ci
npm run check
```

Extend the collection for a distinct, useful concept, draw both masters, update the catalog once, and run the package checks and visual review. Choose a semantic drawing at each application call site; a new export does not imply the application uses it. Changes to existing glyphs also require the consuming application's checks. This package does not publish, commit, or push as part of its build.
