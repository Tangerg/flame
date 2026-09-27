import { describe, expect, it } from "vitest";
import { intraLineDiff } from "./intraLineDiff";

describe("intraLineDiff", () => {
  it("marks the differing middle, trimming common prefix + suffix", () => {
    expect(intraLineDiff("foo bar baz", "foo qux baz")).toEqual({ del: [4, 7], add: [4, 7] });
  });

  it("marks only the appended tail on a pure insertion", () => {
    expect(intraLineDiff("abc", "abcdef")).toEqual({ del: null, add: [3, 6] });
  });

  it("marks only the removed tail on a pure deletion", () => {
    expect(intraLineDiff("abcdef", "abc")).toEqual({ del: [3, 6], add: null });
  });

  it("returns null/null for identical lines", () => {
    expect(intraLineDiff("same", "same")).toEqual({ del: null, add: null });
  });

  it("returns null/null when the lines share no prefix or suffix", () => {
    expect(intraLineDiff("xxx", "yyy")).toEqual({ del: null, add: null });
  });

  it("handles a shared prefix only (no common suffix)", () => {
    expect(intraLineDiff("const a = 1;", "const a = 2;")).toEqual({ del: [10, 11], add: [10, 11] });
  });
});

describe("intraLineDiff on characters outside the BMP", () => {
  const lone = /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/;

  it.each([
    ["const a = '🙂';", "const a = '🙃';"],
    ["x = 😀😀", "x = 😀🎉"],
    ["🙂 tail", "🙃 tail"],
    ["lead 🙂", "lead 🙃"],
    ["a🙂b🙂c", "a🙂b🙃c"],
  ])("never marks half a character in %s", (a, b) => {
    const { del, add } = intraLineDiff(a, b);
    expect(a1(del, a)).not.toMatch(lone);
    expect(a1(add, b)).not.toMatch(lone);
  });

  it("still covers everything that changed", () => {
    const { del, add } = intraLineDiff("const a = '🙂';", "const a = '🙃';");
    expect(a1(del, "const a = '🙂';")).toContain("🙂");
    expect(a1(add, "const a = '🙃';")).toContain("🙃");
  });

  it("leaves an unchanged astral line alone", () => {
    expect(intraLineDiff("🙂🙃", "🙂🙃")).toEqual({ del: null, add: null });
  });
});

function a1(range: [number, number] | null, text: string): string {
  return range ? text.slice(range[0], range[1]) : "";
}
