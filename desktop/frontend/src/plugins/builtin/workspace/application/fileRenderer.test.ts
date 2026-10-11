import { describe, expect, it } from "vitest";
import type { WorkspaceFileRendererSpec } from "@/plugins/sdk/types/workspace";
import { selectFileRenderer } from "./fileRenderer";

const markdown: WorkspaceFileRendererSpec = {
  id: "markdown",
  title: "Markdown",
  extension: "md",
  component: () => null,
};

describe("file renderer selection", () => {
  it("selects only a unique match for the file extension", () => {
    expect(selectFileRenderer([markdown], "md")).toBe(markdown);
    expect(selectFileRenderer([markdown], "txt")).toBeUndefined();
    expect(selectFileRenderer([markdown, { ...markdown, id: "other" }], "md")).toBeUndefined();
  });
});
