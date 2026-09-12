import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { ToolCard } from "@/plugins/builtin/chat/tools/public/rendering";
import { taskPreview } from "./taskPreview";

const tool: ToolCall = {
  id: "delegate_1",
  runId: "run_1",
  name: "delegate_task",
  fn: "Audit the cancellation path",
  args: "{}",
  status: "ok",
};

beforeEach(async () => {
  await loadPluginsForTest(taskPreview);
});

afterEach(async () => {
  await resetKernelForTest();
});

describe("delegated reply preview", () => {
  it("renders the full reply as prose, including material after the ninth line", () => {
    const paragraphs = Array.from({ length: 20 }, (_, index) => `Finding ${index + 1}.`);
    render(
      <ToolCard
        expanded
        onToggleExpand={() => {}}
        tool={{ ...tool, result: ["## Audit reply", ...paragraphs].join("\n\n") }}
      />,
    );

    expect(screen.getByRole("heading", { name: "Audit reply" })).toBeTruthy();
    expect(screen.getByText("Finding 20.")).toBeTruthy();
    expect(screen.getByRole("region", { name: tool.fn }).tabIndex).toBe(0);
    expect(screen.queryByRole("button", { name: "View full reply" })).toBeNull();
  });

  it("keeps waiting and failed delegation visible", () => {
    const { rerender } = render(
      <ToolCard expanded onToggleExpand={() => {}} tool={{ ...tool, status: "running" }} />,
    );
    expect(screen.getByText("Sub-agent working…")).toBeTruthy();

    rerender(
      <ToolCard
        expanded
        onToggleExpand={() => {}}
        tool={{ ...tool, status: "err", result: "error: delegated worker canceled" }}
      />,
    );
    expect(screen.getByText("error: delegated worker canceled")).toBeTruthy();
  });
});
