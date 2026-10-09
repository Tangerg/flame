# Icon design standard

## Primary reference

**[The making of Cursor's icons](https://www.minoradventures.co/blog/the-making-of-cursors-icons)** — Marek Minor, Minor Adventures, July 30, 2026. This article is the primary methodological reference for Flame's icon system. Read its construction and optical-adjustment examples when reviewing a new drawing. The linked original is authoritative for the author's explanation and illustrations; this document records Flame's own implementation decisions.

The following is a concise paraphrase of the reference, not a reproduction of its text or artwork:

- Draw independent 16px and 24px optical masters, with 1.25px and 1.5px nominal strokes.
- Begin with horizontal, vertical, and 45-degree construction; round deliberately.
- Prefer continuous silhouettes, natural object proportions, and direct cancellation marks without simulated depth.
- Balance square, circular, horizontal, and vertical forms optically. Shared boxes do not imply equal visual mass.
- Correct congested junctions, local stroke density, dot sizes, and overlap spacing by eye.
- Keep recurring objects and modifiers consistent; review at actual sizes and in context.
- Maintain a reproducible pipeline and a discoverable vocabulary, rather than relying on the designer's memory.

## Flame decisions

The six approved samples establish the construction language: brain, pin, eye, bell, plug, and strikethrough. Flame owns their drawings. Cursor's exported assets, typeface, Unicode mapping, and icon-font delivery are not inputs to this package. Cursor's brand mark remains an external reference in the article, rather than an exported Flame icon. Study the method and author a distinct drawing; hand tracing and attribution do not establish permission to reproduce someone else's artwork.

Use 16px geometry below 22px and 24px geometry at or above 22px. This threshold follows the reference's optical-size approach and is verified against Flame's actual 9–36px range. Keep this choice internal: product callers specify the size they need, not the master to use. An exact-master selector belongs in the review tool.

Keep base contours complete under a diagonal cancellation stroke. The slash goes from top left to bottom right; strikethrough uses a horizontal stroke. These modifiers are composed during the build into complete glyphs. No background-colored eraser, mask, shadow, or runtime assembly is needed. Local thinning of inner strokes is authored in the source, not exposed as a consumer setting.

Keep round caps and joins, precise silhouettes, and restrained rounding. Mechanical bodies use straight edges and small corners; circular objects overshoot the square body dimensions. A document is tall, a folder wide, and a pencil diagonal. File folds and internal partitions are locally thinner than the contour. Background shapes in layered glyphs end before the foreground object, with explicit spacing in the path itself. Geometric intersections (`Atom`, `SquareCircle`) retain both contours because the overlap is their subject. Cancellation strokes remain the exception: they cross complete contours. Brain lobes, pupils, bell details, and the typographic S retain curves where they communicate the object. The 24px bell includes a crown detail that the smaller drawing omits. Preserve tall, wide, and diagonal proportions inside the common viewport; do not enlarge every object until it fills a square.

Author corner transitions in the centerline path; round caps and joins alone leave inner right angles and diagonal tips sharp. Use restrained arcs on larger bodies and short tangent curves at folded shoulders and pencil ends. At 16px, the file, folder, terminal, and compose bodies use 0.75px corner transitions; their 24px drawings use 1.125px, with 1.25px on the larger compose frame. Small internal symbols need less rounding so their counters stay open. Preserve straight runs, object proportions, and recognizable fold lines rather than rounding every point indiscriminately.

Use `currentColor` and outlined SVGs. React is the first supported adapter because Flame already uses it. Filled glyphs, other frameworks, arbitrary themes, optical overrides, and global configuration require demonstrated needs before becoming APIs. Application mappings decide which glyph represents each product concept.

## Optical construction

These are Flame's reference ink spans, including the nominal stroke. They balance basic shapes rather than forcing every object into a bounding box.

| Shape      | 16px master | 24px master |
| ---------- | ----------- | ----------- |
| Square     | 13 × 13     | 19 × 19     |
| Circle     | 14 × 14     | 21 × 21     |
| Horizontal | 14 × 11     | 21 × 17     |
| Vertical   | 11 × 14     | 17 × 21     |

`Circle` owns the recurring circular contour. Its related glyphs author only their internal details; the builder composes the shared contour into complete standalone SVGs and React drawings. A clock, globe, face, and circular status symbol must not acquire different rim dimensions independently. File, folder, calendar, clipboard, book, message, bookmark, square, and shield states likewise share their base contour. Each internal symbol is positioned for that object’s usable interior, rather than automatically centered in the viewport.

Build cloud and flame silhouettes from horizontal, vertical, and diagonal segments with deliberate corner transitions. Round an intentional construction rather than assembling inflated bubbles. Keep natural curves for objects where they communicate the concept. Star points use short rounded transitions, and the small star in `Sparkles` has a lighter stroke and a larger counter relative to its body.

Correct dense junctions locally. The globe's meridians end slightly inside its rim; the paper plane's crease ends inside the tip and central junction; the at sign's counter joins its stem without stacking a full circle underneath it. Braces and the asterisk have thinner central strokes. The transparent cube keeps its complete intersecting structure with lighter internal rays. These small authored adjustments must remain readable at actual size; do not turn every junction into a visible gap.

Round-capped short strokes form dots. A selected-state dot, a question mark's dot, and a face's eyes have different optical roles and deliberately different diameters. Avoid tiny outlined rings where a solid dot is intended. Horizontal and vertical ellipses share dot weight and spacing. At 16px, music details inside files, folders, and messages use solid noteheads with separated stems; the larger drawings have room for outlined counters.

Position internal symbols in the usable space of their container: above a book's binding and a bookmark's notch, below a calendar's header or a document's fold. The 16px calendar has a shallower header so its clock, date marks, and actions have room below it; center those details in the remaining body. Keep clock hands inside the rim, and begin a search handle at its lens boundary. Miniature symbols need their own counters and local stroke weights; shrinking the full-size object is insufficient. The 16px keyboard, calendar, and text states deliberately carry fewer details than their 24px drawings. A document or folder already supplies the image boundary, so its picture detail needs a mountain and sun rather than a second cramped frame. Structural joints such as a printer's paper feed share an edge; they do not require the separation used between independent layered objects.

## Applying the reference examples

The vocabulary includes the standalone object concepts from the closed-contour and natural-proportion figures, the optical-junction studies (`At`, `Globe`, `Flame`, `Send`, `GitMerge`, `Braces`), and the density studies (`Asterisk`, `CubeTransparent`, `FlaskDisabled`, `RadioTower`, `Sparkles`). The rebus and sharp-detail figures inform additional everyday symbols such as `Microphone`, `Database`, `Camera`, `Pointer`, and `FilePdf`. Inspect both Before and After states of the article's interactive studies; the thumbnails show the uncorrected versions. Cursor's logo illustrates the density principle in the original article only.

Draw each master deliberately. Preserve the concept and construction relationship while adjusting body dimensions, inner counters, curve tension, and detail density for its grid. Primitive circles and rectangles can share a mathematical construction; a complete glyph must not be a rescaled third-party path. Review the authored SVGs directly. Generated React and export files are projections of that geometry.

## Review a drawing

1. **Meaning:** the symbol remains recognizable alone and beside related states; an off glyph retains its base identity. Directional details follow the action: `Target` points into the bullseye to express reaching a goal.
2. **Construction:** the contour is intentional, related objects share geometry, and a cancellation mark follows the common angle and extent.
3. **Weight:** compare with adjacent glyphs and Flame's text, including dense intersections. Local thinning must improve the real-size result.
4. **Space:** internal counters and overlaps remain distinguishable. For layered glyphs, use the reference's three-unit overlap gap on the 16px grid as the starting criterion; do not confuse it with a minimum for every internal counter.
5. **Optical size:** inspect both authored masters, the 21/22px transition, every application's size token, and user font sizes 11–18px. Review small sizes on a physical display at normal zoom.
6. **Context:** inspect light/dark surfaces, inherited color, lists, toolbars, and muted states. An enlarged specimen alone cannot establish legibility.
7. **Delivery:** rebuild, check SVG/React parity, ref and accessibility behavior, and named-import tree shaking. Update the application's mapping and consumers for changed semantics.

Automated checks protect delivery contracts. They cannot decide whether a silhouette feels balanced or a symbol means the intended thing; those judgments remain part of visual review.
