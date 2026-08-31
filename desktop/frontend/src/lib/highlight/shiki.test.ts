import { describe, expect, it } from "vitest";
import type { Highlighter } from "shiki";
import { langFromPath, resolveLang } from "./shiki";

// The names JavaScript hands out from `Object.prototype` to anyone who indexes an
// object with a string they did not choose.
const INHERITED_KEYS = ["constructor", "toString", "valueOf", "hasOwnProperty", "__proto__"];

describe("langFromPath", () => {
  it("maps common extensions to their Shiki language", () => {
    expect(langFromPath("a/b/main.go")).toBe("go");
    expect(langFromPath("script.py")).toBe("python");
    expect(langFromPath("src/App.tsx")).toBe("tsx");
    expect(langFromPath("lib/util.ts")).toBe("typescript");
    expect(langFromPath("main.rs")).toBe("rust");
    expect(langFromPath("style.scss")).toBe("scss");
  });

  it("recognizes well-known bare filenames", () => {
    expect(langFromPath("deploy/Dockerfile")).toBe("dockerfile");
    expect(langFromPath("Makefile")).toBe("bash");
  });

  it("falls back to text for unknown / extensionless paths", () => {
    expect(langFromPath("notes")).toBe("text");
    expect(langFromPath("data.kdl")).toBe("text");
  });

  it("is case-insensitive on the extension", () => {
    expect(langFromPath("README.MD")).toBe("markdown");
  });

  // A file named `constructor` used to return the Object constructor itself, and
  // `resolveLang` then threw on `lang.toLowerCase()` — taking the diff view down
  // with it. The guard is the TYPE, because a lookup table that answers with an
  // inherited member is wrong however the caller happens to survive it.
  it("answers with a language tag for filenames that name an inherited member", () => {
    for (const key of INHERITED_KEYS) {
      expect(typeof langFromPath(key)).toBe("string");
      expect(typeof langFromPath(`src/${key}`)).toBe("string");
      expect(typeof langFromPath(`file.${key}`)).toBe("string");
    }
  });
});

describe("resolveLang", () => {
  const highlighter = {
    getLoadedLanguages: () => ["typescript", "bash", "cpp"],
  } as unknown as Highlighter;

  it("keeps a loaded tag and aliases a known short one", () => {
    expect(resolveLang(highlighter, "typescript")).toBe("typescript");
    expect(resolveLang(highlighter, "sh")).toBe("bash");
    expect(resolveLang(highlighter, "c++")).toBe("cpp");
  });

  it("degrades an un-bundled tag to text", () => {
    expect(resolveLang(highlighter, "kdl")).toBe("text");
  });

  it("degrades a tag that names an inherited member rather than throwing", () => {
    for (const key of INHERITED_KEYS) {
      expect(resolveLang(highlighter, key)).toBe("text");
    }
  });
});
