import { describe, expect, it } from "vitest";
import { forEachSeed } from "@/test/arbitrary";
import { normalizeMathDelimiters } from "./preprocess";

// This rewrites agent prose before remark-math parses it, so its input is any text
// a model can emit. Text that already carries a lone surrogate keeps it; what must
// not happen is the rewrite CREATING one out of a well-formed input.
const LONE_SURROGATE = /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/;

describe("math normalization, over arbitrary agent prose", () => {
  it("never splits a character that arrived whole", () => {
    forEachSeed(600, (a) => {
      const text = a.text();
      if (LONE_SURROGATE.test(text)) return;
      expect(normalizeMathDelimiters(text)).not.toMatch(LONE_SURROGATE);
    });
  });

  it("leaves text carrying no math delimiter untouched", () => {
    forEachSeed(600, (a) => {
      const text = a.text();
      if (/[$\\]/.test(text)) return;
      expect(normalizeMathDelimiters(text)).toBe(text);
    });
  });
});
