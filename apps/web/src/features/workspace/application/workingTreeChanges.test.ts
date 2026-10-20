import { describe, expect, it } from "vitest";
import { summarizeWorkingTreeChanges } from "@/features/workspace/application/workingTreeChanges";
describe("working tree change summary", () => {
  it("keeps an absent or empty read without a summary", () => {
    expect(summarizeWorkingTreeChanges(undefined)).toBeNull();
    expect(summarizeWorkingTreeChanges([])).toBeNull();
  });

  it("totals complete recorded line counts including known zeroes", () => {
    expect(
      summarizeWorkingTreeChanges([
        { path: "added.ts", change: "add", added: 7, removed: 0 },
        { path: "modified.ts", change: "mod", added: 2, removed: 3 },
      ]),
    ).toEqual({ files: 2, added: 9, removed: 3 });
  });

  it("does not present a partial sum as a complete recorded total", () => {
    expect(
      summarizeWorkingTreeChanges([
        { path: "known.ts", change: "mod", added: 7, removed: 2 },
        { path: "partial.ts", change: "mod", removed: 3 },
      ]),
    ).toEqual({ files: 2, added: undefined, removed: 5 });
    expect(
      summarizeWorkingTreeChanges([
        { path: "partial.ts", change: "mod", added: 0 },
        { path: "known.ts", change: "mod", added: 2, removed: 3 },
      ]),
    ).toEqual({ files: 2, added: 2, removed: undefined });
    expect(
      summarizeWorkingTreeChanges([{ path: "image.png", change: "add", binary: true }]),
    ).toEqual({ files: 1, added: undefined, removed: undefined });
  });
});
