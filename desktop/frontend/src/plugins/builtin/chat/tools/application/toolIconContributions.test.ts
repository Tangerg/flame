import { describe, expect, it } from "vitest";
import { ICON_NAMES, type IconName } from "@/ui/icons";
import { TOOL_ICON_BY_NAME, toolVerbId } from "@/lib/toolFamilies";
import { en } from "@/lib/i18n/locales/en";
import { defaultToolIconContributions, defaultToolIconFor } from "./toolIconContributions";

const entries = (items: { key: string; icon: string }[]) =>
  Object.fromEntries(items.map((item) => [item.key, item.icon]));

describe("tool icon contributions", () => {
  it("gives every built-in tool a glyph of its own", () => {
    const byGlyph = new Map<string, string[]>();
    for (const [tool, glyph] of Object.entries(TOOL_ICON_BY_NAME)) {
      byGlyph.set(glyph, [...(byGlyph.get(glyph) ?? []), tool]);
    }
    const shared = [...byGlyph].filter(([, tools]) => tools.length > 1);

    expect(shared).toEqual([]);
    expect(byGlyph.size).toBe(Object.keys(TOOL_ICON_BY_NAME).length);
    expect(Object.keys(TOOL_ICON_BY_NAME).length).toBeGreaterThan(20);
    expect(TOOL_ICON_BY_NAME).not.toHaveProperty("edit");
    expect(TOOL_ICON_BY_NAME).not.toHaveProperty("write");
  });

  it("gives every built-in tool a verb in both tenses", () => {
    const ids = Object.keys(TOOL_ICON_BY_NAME).map((name) => toolVerbId(name));
    expect(ids.filter((id) => id === undefined)).toEqual([]);

    const missing = ids.flatMap((id) =>
      (["doing", "done"] as const)
        .map((tense) => `tool.${tense}.${id}`)
        .filter((key) => !(key in en)),
    );
    expect(missing).toEqual([]);
  });

  it("has no verb for a name the Runtime does not publish", () => {
    expect(toolVerbId("edit")).toBeUndefined();
    expect(toolVerbId("write")).toBeUndefined();
    expect(toolVerbId("apply_patch")).toBe("applyPatch");
  });

  it("names glyphs the icon vocabulary actually has", () => {
    const unknown = Object.values(TOOL_ICON_BY_NAME).filter(
      (glyph) => !ICON_NAMES.has(glyph as IconName),
    );
    expect(unknown).toEqual([]);
  });

  it("turns the default icon table into registry contributions", () => {
    expect(entries(defaultToolIconContributions())).toEqual(TOOL_ICON_BY_NAME);
  });

  it("falls back to the generic tool glyph for a name it does not know", () => {
    expect(defaultToolIconFor("lsp")).toBe("code");
    expect(defaultToolIconFor("acme_do_thing")).toBe("tool");
  });
});
