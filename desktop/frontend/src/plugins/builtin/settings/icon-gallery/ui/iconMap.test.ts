import { describe, expect, it } from "vitest";
import { IconMap, TocById, rawToc } from "./iconMap";

/**
 * The glob is the whole risk here.
 *
 * `import.meta.glob` that matches nothing yields `{}` — no error, no warning, no build
 * failure. This one pointed three directories above the `node_modules` that exists, so
 * `IconMap` was empty for the entire history of the repository and every one of the 321
 * cards in the gallery rendered its `?` fallback. The pane had a WCAG audit, which reads
 * names and cannot see a missing picture.
 *
 * A floor rather than an exact count: the catalogue grows with the package, and a test that
 * has to be edited whenever a vendor ships an icon is a test people learn to update without
 * reading. What cannot be allowed is the map going empty, or losing most of itself.
 */
describe("iconMap", () => {
  it("resolves the icon modules the glob is written to find", () => {
    expect(Object.keys(IconMap).length).toBeGreaterThan(100);
  });

  // The catalogue and the components are two lists that have to line up: an entry with no
  // component is the `?` card, which is what the broken glob produced 321 times.
  it("has a component for nearly every entry in the catalogue", () => {
    const missing = rawToc.filter((entry) => !IconMap[entry.id]).map((entry) => entry.id);
    expect(rawToc.length).toBeGreaterThan(100);
    expect(missing.length / rawToc.length).toBeLessThan(0.1);
  });

  it("indexes the catalogue by id", () => {
    expect(Object.keys(TocById).length).toBe(rawToc.length);
    expect(TocById[rawToc[0]!.id]).toBe(rawToc[0]);
  });
});
