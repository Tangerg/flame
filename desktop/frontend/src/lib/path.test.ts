import { describe, expect, it } from "vitest";
import { basename, splitFilePath } from "./path";

// A cwd is a directory path and arrives with its separator. The two helpers have to
// name it the same thing: one feeds a chip, the other the two-line row beside it.
// The cut used to be measured on the stripped path and applied to the original, so
// the row read "project/" while the chip read "project".
describe("a directory path carrying its trailing separator", () => {
  it.each([
    ["a/b/", "a", "b"],
    ["a/b//", "a", "b"],
    ["/Users/me/project/", "/Users/me", "project"],
    ["a/", "", "a"],
  ])("splits %o into %o + %o", (path, directory, name) => {
    expect(splitFilePath(path)).toEqual({ directory, name });
    expect(basename(path)).toBe(name);
  });

  it("agrees with basename on a root and on an empty path", () => {
    for (const path of ["/", ""]) {
      expect(splitFilePath(path).name).toBe(basename(path));
    }
  });

  it("still splits an ordinary file path", () => {
    expect(splitFilePath("src/lib/path.ts")).toEqual({ directory: "src/lib", name: "path.ts" });
    expect(splitFilePath("path.ts")).toEqual({ directory: "", name: "path.ts" });
  });
});
