import { expect, it } from "vitest";
import { catalog } from "./catalog";
import { labCases } from "./cases";
import { readSelection } from "./selection";

it("projects every registered variant into a unique independently openable case", () => {
  const selection = readSelection("theme=dark&locale=zh-CN&font-size=18", catalog);
  const cases = labCases(catalog, selection);
  expect(cases).toHaveLength(
    catalog.stories.reduce((sum, story) => sum + story.variants.length, 0),
  );
  expect(new Set(cases.map((entry) => entry.id)).size).toBe(cases.length);
  for (const entry of cases) {
    const story = catalog.story(entry.storyId);
    const url = new URL(entry.url, "https://flame.test");
    expect(entry.width).toBe(story.width);
    expect(entry.height).toBe(story.height);
    expect(url.searchParams.get("theme")).toBe("dark");
    expect(url.searchParams.get("font-size")).toBe("18");
    expect(url.searchParams.get("motion")).toBe("off");
    expect(url.searchParams.get("locale")).toBe(story.kind === "fixture" ? "zh" : "zh-CN");
  }
});

it("preserves component identities and recorded fixture targets in verification links", () => {
  const cases = labCases(catalog, readSelection("font-size=18", catalog));
  for (const entry of cases.filter((entry) => entry.kind === "component")) {
    const url = new URL(entry.url, "https://flame.test");
    expect(readSelection(url.search, catalog)).toMatchObject({
      storyId: entry.storyId,
      variantId: entry.variantId,
      fontSize: 18,
    });
  }
  for (const entry of cases.filter((entry) => entry.kind === "fixture")) {
    const story = catalog.story(entry.storyId);
    if (story.kind !== "fixture") throw new Error("Fixture case lost its source story");
    const url = new URL(entry.url, "https://flame.test");
    expect(url.pathname).toBe("/lab/fixture.html");
    expect(url.searchParams.get("state")).toBe(
      story.variants.find((variant) => variant.id === entry.variantId)!.target.state,
    );
  }
});

it("changes verification geometry without changing the scenario document", () => {
  const selection = readSelection("", catalog);
  const original = labCases(catalog, selection);
  const narrow = labCases(catalog, { ...selection, viewport: "phone" });
  expect(narrow.map((entry) => entry.url)).toEqual(original.map((entry) => entry.url));
  expect(narrow.every((entry) => entry.width === 390)).toBe(true);
});
