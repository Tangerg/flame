import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { ICON_NAMES, knownIconName } from "./icon";
import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";

describe("narrowing a contributed icon name", () => {
  it("keeps a name the set actually draws", () => {
    for (const name of ["tool", "alert", "chevron-down", "x"]) {
      expect(knownIconName(name)).toBe(name);
    }
  });

  it("refuses a name nothing draws, so the caller has to choose a fallback", () => {
    for (const value of [
      "no-such-icon",
      "",
      " ",
      "TOOL",
      "tool ",
      "constructor",
      "__proto__",
      "toString",
      null,
      undefined,
    ]) {
      expect(knownIconName(value)).toBeUndefined();
    }
  });

  it("answers nothing for a prototype member masquerading as a name", () => {
    expect(knownIconName("hasOwnProperty")).toBeUndefined();
    expect(ICON_NAMES.has("valueOf" as never)).toBe(false);
  });

  it("agrees with the set it narrows to, over every name in it", () => {
    expect(ICON_NAMES.size).toBeGreaterThan(50);
    for (const name of ICON_NAMES) expect(knownIconName(name)).toBe(name);
  });

  it("draws every glyph the built-in tool table names", () => {
    const missing = Object.entries(TOOL_ICON_BY_NAME)
      .filter(([, glyph]) => knownIconName(glyph) === undefined)
      .map(([tool, glyph]) => `${tool} -> ${glyph}`);
    expect(missing).toEqual([]);
    expect(Object.keys(TOOL_ICON_BY_NAME).length).toBeGreaterThan(20);
  });
});

describe("the glyph set", () => {
  const ALIAS = /export \{ default \} from '\.\/([a-z0-9-]+)\.mjs'/;

  it("names every icon by the name Lucide still owns", () => {
    const source = readFileSync(join(process.cwd(), "src/ui/icons/icon.tsx"), "utf8");
    const components = [...source.matchAll(/^ {2}([A-Z][A-Za-z0-9]*),$/gm)].map(
      (match) => match[1]!,
    );
    expect(components.length).toBeGreaterThan(50);

    const renamed = components.flatMap((component) => {
      const file = component.replace(/(?<!^)(?=[A-Z0-9])/g, "-").toLowerCase();
      const path = join(process.cwd(), `node_modules/lucide-react/dist/esm/icons/${file}.mjs`);
      const alias = ALIAS.exec(readFileSync(path, "utf8"));
      return alias ? [`${file} -> ${alias[1]}`] : [];
    });
    expect(renamed).toEqual([]);
  });
});
