import { describe, expect, it } from "vitest";
import { forEachSeed } from "@/test/arbitrary";
import { segmentWords } from "./i18n/segmentWords";
import { basename, splitFilePath } from "./path";
import { fmtTokens } from "./format";

const LONE_SURROGATE = /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/;

describe("word segmentation, which every streamed token goes through", () => {
  it("rebuilds the text it was given", () => {
    forEachSeed(600, (a) => {
      const text = a.text();
      expect(segmentWords(text).join("")).toBe(text);
    });
  });

  it("never emits half a character", () => {
    forEachSeed(600, (a) => {
      const text = a.text();
      if (LONE_SURROGATE.test(text)) return;
      for (const word of segmentWords(text)) expect(word).not.toMatch(LONE_SURROGATE);
    });
  });
});

describe("path presentation, over the shapes a diff can carry", () => {
  it("keeps the basename inside the path", () => {
    forEachSeed(400, (a) => {
      const path = a.path();
      const trimmed = path.replace(/\/+$/, "") || path;
      expect(trimmed.endsWith(basename(path))).toBe(true);
    });
  });

  it("splits into a directory and a name that reassemble", () => {
    const broken: string[] = [];
    forEachSeed(400, (a) => {
      const path = a.path();
      const { directory, name } = splitFilePath(path);
      if (name !== basename(path)) broken.push(`name ${path}`);
      if (directory !== "" && `${directory}/${name}` !== path.replace(/\/+$/, "")) {
        broken.push(`join ${path}`);
      }
    });
    expect(broken).toEqual([]);
  });
});

describe("token formatting, over the numbers a runtime can report", () => {
  it("answers a finite string for any finite count", () => {
    forEachSeed(400, (a) => {
      const counts = [0, 1, 999, 1000, 999_999, 1_000_000, Number.MAX_SAFE_INTEGER, a.int(1e9)];
      for (const count of counts) {
        const out = fmtTokens(count);
        expect(typeof out).toBe("string");
        expect(out).not.toContain("NaN");
        expect(out).not.toContain("Infinity");
      }
    });
  });
});
