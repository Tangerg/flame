import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

const opened = vi.hoisted(() => vi.fn());
vi.mock("@/plugins/builtin/workspace/public/deeplinks", () => ({
  openFileInWorkingTreeDiff: opened,
}));
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { ApplyPatchPreview } from "./patch";

function patchTool(result: string | undefined, status: ToolCall["status"] = "ok"): ToolCall {
  return {
    id: "patch_1",
    runId: "run_1",
    name: "apply_patch",
    fn: "apply_patch",
    args: "",
    result,
    status,
  };
}

describe("ApplyPatchPreview", () => {
  it("renders only the exact call receipt as quiet file-change rows", () => {
    const { container } = render(
      <ApplyPatchPreview
        tool={patchTool(
          '{"changes":[{"path":"src/new.ts","status":"added"},{"path":"src/current.ts","status":"moved","from":"src/old.ts"}]}',
        )}
      />,
    );

    expect(screen.getByText("Created")).toBeTruthy();
    expect(screen.getByText("Moved")).toBeTruthy();
    expect(screen.getByTitle("src/new.ts")).toBeTruthy();
    expect(screen.getByTitle("src/old.ts")).toBeTruthy();
    expect(screen.getByTitle("src/current.ts")).toBeTruthy();
    expect(container.querySelectorAll("[data-patch-change]")).toHaveLength(2);
    expect(container.querySelector("[class*='diff-added']")).toBeNull();
  });

  it("shows what a running call is changing, with the counts the receipt never carries", () => {
    render(
      <ApplyPatchPreview
        tool={{
          ...patchTool(undefined, "running"),
          changes: [
            { path: "src/a.ts", status: "modified", added: 12, removed: 3 },
            { path: "src/b.ts", status: "added", added: 40, removed: 0 },
          ],
        }}
      />,
    );

    expect(screen.getByTitle("src/a.ts")).toBeTruthy();
    expect(screen.getByTitle("src/b.ts")).toBeTruthy();
    expect(screen.getByText("+12")).toBeTruthy();
    expect(screen.queryByText("Running…")).toBeNull();
  });

  it("hands the row back to the receipt the moment the call settles", () => {
    render(
      <ApplyPatchPreview
        tool={{
          ...patchTool('{"changes":[{"path":"src/a.ts","status":"modified"}]}', "ok"),
          changes: [{ path: "src/never-applied.ts", status: "added", added: 9, removed: 0 }],
        }}
      />,
    );

    expect(screen.getByText("Edited")).toBeTruthy();
    expect(screen.queryByTitle("src/never-applied.ts")).toBeNull();
  });

  it("keeps running and completed empty receipts distinct", () => {
    const { rerender } = render(<ApplyPatchPreview tool={patchTool(undefined, "running")} />);
    expect(screen.getByText("Running…")).toBeTruthy();

    rerender(<ApplyPatchPreview tool={patchTool('{"changes":[]}', "ok")} />);
    expect(screen.getByText("No changes to show")).toBeTruthy();
  });

  it("reaches the fifteenth file of a large patch and opens the one it names", () => {
    const changes = Array.from({ length: 15 }, (_, index) => ({
      path: `src/file-${index + 1}.ts`,
      status: "modified",
    }));
    const { container } = render(
      <ApplyPatchPreview tool={patchTool(JSON.stringify({ changes }))} />,
    );
    expect(container.querySelectorAll("[data-patch-change]")).toHaveLength(9);

    fireEvent.click(screen.getByRole("button", { name: /6 more/ }));
    expect(container.querySelectorAll("[data-patch-change]")).toHaveLength(15);

    fireEvent.click(screen.getByTitle("src/file-15.ts"));
    expect(opened).toHaveBeenCalledWith("src/file-15.ts");
  });
});
