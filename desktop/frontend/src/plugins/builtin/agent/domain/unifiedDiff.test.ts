import { describe, expect, it } from "vitest";
import { parseUnifiedDiff } from "./unifiedDiff";

describe("parseUnifiedDiff", () => {
  it("counts a modification without mistaking the headers for hunk lines", () => {
    expect(
      parseUnifiedDiff(
        [
          "diff --git a/src/a.ts b/src/a.ts",
          "--- a/src/a.ts",
          "+++ b/src/a.ts",
          "@@ -1,3 +1,4 @@",
          " keep",
          "-gone",
          "+added",
          "+also added",
        ].join("\n"),
      ),
    ).toEqual([{ path: "src/a.ts", status: "modified", added: 2, removed: 1 }]);
  });

  it("reads a creation and a deletion off /dev/null", () => {
    expect(
      parseUnifiedDiff(
        [
          "diff --git a/new.ts b/new.ts",
          "new file mode 100644",
          "--- /dev/null",
          "+++ b/new.ts",
          "@@ -0,0 +1 @@",
          "+hello",
          "diff --git a/old.ts b/old.ts",
          "deleted file mode 100644",
          "--- a/old.ts",
          "+++ /dev/null",
          "@@ -1 +0,0 @@",
          "-bye",
        ].join("\n"),
      ),
    ).toEqual([
      { path: "new.ts", status: "added", added: 1, removed: 0 },
      { path: "old.ts", status: "deleted", added: 0, removed: 1 },
    ]);
  });

  it("carries where a rename came from", () => {
    expect(
      parseUnifiedDiff(
        ["diff --git a/old.ts b/new.ts", "rename from old.ts", "rename to new.ts"].join("\n"),
      ),
    ).toEqual([{ path: "new.ts", from: "old.ts", status: "moved", added: 0, removed: 0 }]);
  });

  it("splits a plain unified diff that never says `diff --git`", () => {
    expect(
      parseUnifiedDiff(
        [
          "--- a/one.ts",
          "+++ b/one.ts",
          "@@ -1 +1 @@",
          "-a",
          "+b",
          "--- a/two.ts",
          "+++ b/two.ts",
          "@@ -1 +1,2 @@",
          " keep",
          "+c",
        ].join("\n"),
      ),
    ).toEqual([
      { path: "one.ts", status: "modified", added: 1, removed: 1 },
      { path: "two.ts", status: "modified", added: 1, removed: 0 },
    ]);
  });

  it("answers nothing for text that is not a patch", () => {
    expect(parseUnifiedDiff("")).toEqual([]);
    expect(parseUnifiedDiff("I will now edit the file.")).toEqual([]);
  });
});
