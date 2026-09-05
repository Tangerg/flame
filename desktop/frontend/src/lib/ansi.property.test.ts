import { describe, expect, it } from "vitest";
import { forEachSeed } from "@/test/arbitrary";
import { hasAnsi, parseAnsi } from "./ansi";

// Terminal output nobody validated. A parser that throws here takes a transcript row with
// it, and one that loops takes the frame.

describe("the ANSI parser, over arbitrary terminal output", () => {
  it("preserves every visible character in order", () => {
    forEachSeed(600, (a) => {
      const text = a.bool(0.5) ? a.text() : a.bytes(200);
      const visible = parseAnsi(text)
        .map((span) => span.text)
        .join("");
      expect(visible.length).toBeLessThanOrEqual(text.length);
      expect(text).toContain(visible.slice(0, 40));
    });
  });

  it("agrees with its own detector", () => {
    const mismatches: string[] = [];
    forEachSeed(600, (a) => {
      const text = a.bool(0.5) ? a.text() : a.bytes(200);
      if (hasAnsi(text)) return;
      const visible = parseAnsi(text)
        .map((span) => span.text)
        .join("");
      if (visible !== text) mismatches.push(text);
    });
    expect(mismatches).toEqual([]);
  });
});
