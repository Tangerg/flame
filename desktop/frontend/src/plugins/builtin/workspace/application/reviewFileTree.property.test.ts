import { describe, expect, it } from "vitest";
import { Arbitrary, forEachSeed } from "@/test/arbitrary";
import { buildReviewFileTree, filterReviewFiles, type ReviewTreeNode } from "./reviewFileTree";
import type { WorkspaceFileDiff } from "./workspaceQueries";

// The navigator is built from the diff's own paths, and a diff can name anything the
// repository does: nested, duplicated, dotted, non-Latin, or a directory and a file
// sharing one name. Nothing here validates those first, so the tree build has to be
// total over them and its keys have to stay unique — a collision is a React
// duplicate-key loop, and a lost file is a file nobody can review.

function files(a: Arbitrary): WorkspaceFileDiff[] {
  return Array.from({ length: a.int(10) }, () => ({
    path: a.path(),
    status: a.pick(["added", "modified", "deleted", "renamed", "untracked"] as const),
    added: a.int(50),
    removed: a.int(50),
    binary: a.bool(0.1),
    rows: [],
  }));
}

function walk(nodes: readonly ReviewTreeNode[], visit: (node: ReviewTreeNode) => void): void {
  for (const node of nodes) {
    visit(node);
    if (node.kind === "directory") walk(node.children, visit);
  }
}

describe("the review navigator, over the paths a diff can carry", () => {
  it("keeps every distinct file that has a name", () => {
    forEachSeed(400, (a) => {
      const input = files(a);
      const expected = new Set(
        input
          .filter((file) => file.path.split("/").filter((segment) => segment.length > 0).length > 0)
          .map((file) => file.path),
      ).size;
      let seen = 0;
      walk(buildReviewFileTree(input), (node) => {
        if (node.kind === "file") seen += 1;
      });
      expect(seen).toBe(expected);
    });
  });

  it("gives every node in one parent a distinct key", () => {
    forEachSeed(400, (a) => {
      const check = (nodes: readonly ReviewTreeNode[]) => {
        const keys = nodes.map((node) => `${node.kind}:${node.path}`);
        expect(new Set(keys).size).toBe(keys.length);
        for (const node of nodes) if (node.kind === "directory") check(node.children);
      };
      check(buildReviewFileTree(files(a)));
    });
  });

  it("terminates and stays within the depth the paths describe", () => {
    forEachSeed(400, (a) => {
      const input = files(a);
      const deepest = Math.max(
        0,
        ...input.map((file) => file.path.split("/").filter((s) => s.length > 0).length),
      );
      let depth = 0;
      const measure = (nodes: readonly ReviewTreeNode[], level: number) => {
        depth = Math.max(depth, level);
        for (const node of nodes) {
          if (node.kind === "directory") measure(node.children, level + 1);
        }
      };
      measure(buildReviewFileTree(input), 0);
      expect(depth).toBeLessThanOrEqual(deepest);
    });
  });

  it("never lets a filter invent a file the diff did not carry", () => {
    forEachSeed(400, (a) => {
      const input = files(a);
      const kept = filterReviewFiles(input, a.text());
      expect(kept.length).toBeLessThanOrEqual(input.length);
      for (const file of kept) expect(input).toContain(file);
    });
  });

  it("keeps everything when the query is blank", () => {
    forEachSeed(200, (a) => {
      const input = files(a);
      expect(filterReviewFiles(input, "   ")).toEqual(input);
    });
  });
});
