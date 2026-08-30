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

  it("ignores an email address", () => {
    expect(parseFileRefs("mail a@b.com please")).toEqual(["mail a@b.com please"]);
  });

  // The viewer opens a file at a LINE, so the column is not part of navigation —
  // but it is part of what the tool wrote, and the link replaces that text.
  it("navigates by the line while keeping the column it was shown", () => {
    expect(parseFileRefs("a/b.py:10:5")).toEqual([{ path: "a/b.py", line: 10, column: 5 }]);
  });

  it("returns plain text unchanged when there's no reference", () => {
    expect(parseFileRefs("just words")).toEqual(["just words"]);
  });
});

// `tsc`, `grep` and `eslint` all emit `path:line:col`. The viewer navigates by line,
// but the reference still has to read as the tool wrote it — the column used to be
// consumed by the match and rendered by nothing, so it vanished from the output.
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
