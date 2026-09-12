import { describe, expect, it } from "vitest";
import { parseFileRefs } from "./fileRefs";

describe("parseFileRefs", () => {
  it("extracts a path:line reference with surrounding text", () => {
    expect(parseFileRefs("see src/foo.go:42 now")).toEqual([
      "see ",
      { path: "src/foo.go", line: 42, column: 0 },
      " now",
    ]);
  });

  it("matches a bare basename with a known extension", () => {
    expect(parseFileRefs("Composer.tsx")).toEqual([{ path: "Composer.tsx", line: 0, column: 0 }]);
  });

  it("matches a slashed path without an extension", () => {
    expect(parseFileRefs("cmd/flame/main")).toEqual([
      { path: "cmd/flame/main", line: 0, column: 0 },
    ]);
  });

  it("ignores prose abbreviations and versions", () => {
    expect(parseFileRefs("e.g. version 1.2.3 here")).toEqual(["e.g. version 1.2.3 here"]);
  });

  const paths = (text: string) =>
    parseFileRefs(text)
      .filter((segment) => typeof segment !== "string")
      .map((segment) => segment.path);

  it("does not offer to open a URL a tool printed", () => {
    const printed = [
      "Cloning into 'repo' from https://github.com/acme/repo.git",
      "npm notice See https://npmjs.com/package/left-pad for details",
      "curl -sSL https://example.test/install.sh | sh",
      "listening on http://127.0.0.1:5173/",
    ];
    expect(printed.filter((line) => paths(line).length > 0)).toEqual([]);
  });

  it("does not offer to open arithmetic or a date", () => {
    const prose = ["rate limit 30/60 requests", "on 2024/01/15 the build broke", "ratio was 3/4"];
    expect(prose.filter((line) => paths(line).length > 0)).toEqual([]);
  });

  it("still finds every path a tool actually wrote", () => {
    expect(paths("Edited src/app/main.ts:12")).toEqual(["src/app/main.ts"]);
    expect(paths("moved a/b.ts -> c/d.ts")).toEqual(["a/b.ts", "c/d.ts"]);
    expect(paths("check .github/workflows/ci.yml")).toEqual([".github/workflows/ci.yml"]);
    expect(paths("PATH=/usr/local/bin:/usr/bin")).toEqual(["/usr/local/bin", "/usr/bin"]);
    expect(paths("user@host:/var/log/app.log")).toEqual(["/var/log/app.log"]);
    expect(paths("see README.md and package.json")).toEqual(["README.md", "package.json"]);
  });

  it("ignores an email address", () => {
    expect(parseFileRefs("mail a@b.com please")).toEqual(["mail a@b.com please"]);
  });

  it("navigates by the line while keeping the column it was shown", () => {
    expect(parseFileRefs("a/b.py:10:5")).toEqual([{ path: "a/b.py", line: 10, column: 5 }]);
  });

  it("returns plain text unchanged when there's no reference", () => {
    expect(parseFileRefs("just words")).toEqual(["just words"]);
  });
});

describe("a reference carrying a column", () => {
  it("keeps the column it was given", () => {
    expect(parseFileRefs("see src/main.ts:12:3 for it")).toEqual([
      "see ",
      { path: "src/main.ts", line: 12, column: 3 },
      " for it",
    ]);
  });

  it("reports no column when the tool gave none", () => {
    expect(parseFileRefs("src/main.ts:12")).toEqual([{ path: "src/main.ts", line: 12, column: 0 }]);
  });
});
