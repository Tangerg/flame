import { describe, expect, it } from "vitest";
import { IconMap, IconByName, icons } from "./iconMap";

describe("Flame icon gallery", () => {
  it("renders every catalogued glyph through its named component", () => {
    const missing = icons.filter((entry) => typeof IconMap[entry.component] !== "function");
    expect(missing).toEqual([]);
    expect(new Set(icons.map((entry) => entry.name)).size).toBe(icons.length);
  });

  it("looks up each searchable name without inventing entries", () => {
    expect(Object.keys(IconByName)).toHaveLength(icons.length);
    for (const entry of icons) expect(IconByName[entry.name]).toBe(entry);
    expect(IconByName["not-an-icon"]).toBeUndefined();
  });
});
