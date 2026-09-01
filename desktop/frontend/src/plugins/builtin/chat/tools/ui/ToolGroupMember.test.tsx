import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { ToolGroupMember } from "./ToolGroupMember";

function member(tool: Partial<ToolCall>) {
  const call: ToolCall = {
    id: "t1",
    runId: "r1",
    name: "read",
    fn: "read",
    args: "",
    status: "ok",
    ...tool,
  };
  return render(<ToolGroupMember tool={call} expanded={false} onToggleExpand={() => {}} />);
}

describe("ToolGroupMember", () => {
  // The row used to print the target ALONE, so a column of them read as a list of paths and
  // the only thing saying what happened to each was a glyph.
  it("says the act and the thing acted on, not one or the other", () => {
    member({ name: "read", fn: "src/App.tsx", fnKind: "path" });

    expect(screen.getByText("Read")).toBeTruthy();
    expect(screen.getByTitle("src/App.tsx")).toBeTruthy();
  });

  // The tone the presentation assigns has to survive to the row. It did not: the renderer
  // handled a "success" nothing ever produced and swept "negative" into the muted default,
  // so a non-zero exit read as an ordinary measurement.
  it("shows a non-zero exit as a failure rather than as one more grey figure", () => {
    const { container } = member({ name: "shell", fn: "go test ./...", exitCode: 1 });

    const chip = container.querySelector(".text-negative");
    expect(chip?.textContent).toContain("1");
  });

  it("carries the line counts a running edit already knows", () => {
    member({
      name: "apply_patch",
      fn: "src/App.tsx",
      fnKind: "path",
      status: "running",
      added: 12,
      removed: 3,
    });

    expect(screen.getByText("+12")).toBeTruthy();
    expect(screen.getByText("−3")).toBeTruthy();
  });
});
