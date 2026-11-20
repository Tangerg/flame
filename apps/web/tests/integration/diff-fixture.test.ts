import { expect, it } from "vitest";
import { ACTIVE_DIFF_FILE, evidenceDiff } from "../../lab/scenarios/diffSnapshot";

const emptyFile = "empty.txt";
const emptyFileHeader = 'diff --git "a/empty.txt" "b/empty.txt"';

it("includes only tracked evidence in the branch workspace comparison", () => {
  const { manifest, patch } = evidenceDiff(undefined, "base").visual;
  expect(manifest.map((file) => file.path)).toEqual([
    ACTIVE_DIFF_FILE,
    "docs/新名字.md",
    "assets/image.bin",
  ]);
  expect(patch).not.toContain(emptyFileHeader);
});

it("returns no branch changes for the untracked empty file", () => {
  const { manifest, patch } = evidenceDiff(emptyFile, "base").visual;
  expect(manifest).toEqual([]);
  expect(patch).toBe("");
});

it.each([undefined, emptyFile])(
  "keeps the untracked empty file in default Worktree scope %s",
  (path) => {
    const { manifest, patch } = evidenceDiff(path).visual;
    expect(manifest).toContainEqual({ path: emptyFile, status: "untracked", added: 0, removed: 0 });
    expect(patch).toContain(emptyFileHeader);
  },
);
