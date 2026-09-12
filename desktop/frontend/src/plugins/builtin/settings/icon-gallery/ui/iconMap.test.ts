import { describe, expect, it } from "vitest";
import { IconMap, TocById, rawToc } from "./iconMap";

describe("iconMap", () => {
  it("resolves the icon modules the glob is written to find", () => {
    expect(Object.keys(IconMap).length).toBeGreaterThan(100);
  });

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
