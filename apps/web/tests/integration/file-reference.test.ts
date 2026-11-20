import { expect, it } from "vitest";
import { parseFileRefs } from "@/features/agent/public/fileRefs";

it("resolves the complete Unicode path in prose", () => {
  expect(parseFileRefs("Review runtime/internal/会话/store.go:8 before opening Diff.")).toEqual([
    "Review ",
    { path: "runtime/internal/会话/store.go", line: 8, column: 0 },
    " before opening Diff.",
  ]);
});

it("preserves decomposed spelling, position and surrounding punctuation", () => {
  const path = "docs/re\u0301sume\u0301.md";
  expect(parseFileRefs(`See (${path}:14:2).`)).toEqual([
    "See (",
    { path, line: 14, column: 2 },
    ").",
  ]);
});

it("keeps Unicode mentions and network paths as prose", () => {
  const text = "Keep @docs/旧名字.md:12 and https://例子.test/docs/旧名字.md:12 as text.";
  expect(parseFileRefs(text)).toEqual([text]);
});
