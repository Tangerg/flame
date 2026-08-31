import { describe, expect, it } from "vitest";
import { ICON_NAMES, knownIconName } from "./icon";
import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";

// Icon names arrive as plain strings: plugin contributions, workspace view and settings pane
// specs, and MCP server data the Runtime forwards. Seven call sites used to cast one straight
// to `IconName`, which type-checks and then draws NOTHING for a name we do not have — no
// error, no fallback, a gap where a glyph belongs.

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

  // `constructor` and `toString` are the ones a plain object lookup would have answered with a
  // function. The set is a Set, so it never does — this pins that it stays one.
  it("answers nothing for a prototype member masquerading as a name", () => {
    expect(knownIconName("hasOwnProperty")).toBeUndefined();
    expect(ICON_NAMES.has("valueOf" as never)).toBe(false);
  });

  it("agrees with the set it narrows to, over every name in it", () => {
    expect(ICON_NAMES.size).toBeGreaterThan(50);
    for (const name of ICON_NAMES) expect(knownIconName(name)).toBe(name);
  });

  // The built-in tool table feeds both the registry contributions and the no-plugin fallback,
  // so a glyph renamed out of the icon set would silently blank every card for that tool.
  it("draws every glyph the built-in tool table names", () => {
    const missing = Object.entries(TOOL_ICON_BY_NAME)
      .filter(([, glyph]) => knownIconName(glyph) === undefined)
      .map(([tool, glyph]) => `${tool} -> ${glyph}`);
    expect(missing).toEqual([]);
    expect(Object.keys(TOOL_ICON_BY_NAME).length).toBeGreaterThan(20);
  });
});
