import { describe, expect, it } from "vitest";
import { ICON_NAMES, type IconName } from "@/ui/icons";
import { TOOL_ICON_BY_NAME, toolVerbId } from "@/lib/toolFamilies";
import { en } from "@/lib/i18n/locales/en";
import { defaultToolIconContributions, defaultToolIconFor } from "./toolIconContributions";

const entries = (items: { key: string; icon: string }[]) =>
  Object.fromEntries(items.map((item) => [item.key, item.icon]));

describe("tool icon contributions", () => {
  // The glyph is the only part of a folded row a reader takes in without reading
  // it, so a shared one spends that on nothing: `list` used to stand for reading
  // shell output, three plan-mode calls and a deferred result, and `search` for
  // grep, two recall families and the tool catalog. Assert the WHOLE table is
  // injective rather than spot-checking pairs — the pairs were what let the reuse
  // build up unnoticed.
  it("gives every built-in tool a glyph of its own", () => {
    const byGlyph = new Map<string, string[]>();
    for (const [tool, glyph] of Object.entries(TOOL_ICON_BY_NAME)) {
      byGlyph.set(glyph, [...(byGlyph.get(glyph) ?? []), tool]);
    }
    const shared = [...byGlyph].filter(([, tools]) => tools.length > 1);

    expect(shared).toEqual([]);
    expect(byGlyph.size).toBe(Object.keys(TOOL_ICON_BY_NAME).length);
    expect(Object.keys(TOOL_ICON_BY_NAME)).toHaveLength(30);
    expect(TOOL_ICON_BY_NAME).not.toHaveProperty("edit");
    expect(TOOL_ICON_BY_NAME).not.toHaveProperty("write");
  });

  // The verb and the glyph are two halves of one row, and they were held by two lists.
  // The second had drifted to `edit` and `write` — the exact pair the assertion above
  // guards this one against — so a transcript could never have rendered either, while
  // eight catalogs carried both tenses of both. `check-locales` cannot see this: it
  // credits every `tool.*` key to the template these are built from, and its own
  // completeness rules then carry `en` to the other seven.
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

  // A glyph the vocabulary does not have renders as nothing at all, and the table
  // is a plain Record of strings — so this is the only thing standing between a
  // typo here and an invisible icon in the transcript.
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
