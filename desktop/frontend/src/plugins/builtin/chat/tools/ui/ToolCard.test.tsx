import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { ToolCard } from "./ToolCard";

const opened = vi.hoisted(() => vi.fn());

vi.mock("@/plugins/sdk", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/plugins/sdk")>();
  return {
    ...actual,
    useExtensionPoint: (point: { id: string }) =>
      point.id === "flame.tool.viewOpener"
        ? [{ id: "test-opener", predicate: () => true, open: opened }]
        : [],
  };
});

function card(tool: Partial<ToolCall>) {
  const call: ToolCall = {
    id: "t1",
    runId: "r1",
    name: "read",
    fn: "read",
    args: "",
    status: "ok",
    ...tool,
  };
  return render(<ToolCard tool={call} expanded={false} onToggleExpand={() => {}} />);
}

describe("ToolCard", () => {
  it("keeps every invocation on the narrative line, whatever it is doing", () => {
    const cases: Array<Partial<ToolCall>> = [
      { name: "read", safetyClass: "safe", status: "ok" },
      { name: "shell", safetyClass: "exec", status: "running" },
      { name: "apply_patch", safetyClass: "write", status: "ok" },
      { name: "apply_patch", safetyClass: "write", status: "err", error: "denied" },
      { name: "apply_patch", safetyClass: "write", status: "denied" },
    ];

    const rows = cases.map((entry) => {
      const { container, unmount } = card(entry);
      const row = container.querySelector("[data-shell]");
      const attributes = {
        shell: row?.getAttribute("data-shell"),
        tone: row?.getAttribute("data-tone"),
      };
      unmount();
      return attributes;
    });

    expect(rows).toEqual(cases.map(() => ({ shell: "line", tone: "neutral" })));
  });

  it("opens the workspace view only when its own action is pressed", () => {
    card({ name: "apply_patch", safetyClass: "write", status: "ok" });

    expect(opened).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Open in the context dock" }));
    expect(opened).toHaveBeenCalledOnce();
  });

  it("offers no disclosure for a call that never ran", () => {
    const { container } = card({ name: "apply_patch", safetyClass: "write", status: "denied" });

    expect(container.querySelector("[aria-expanded]")).toBeNull();
    expect(container.querySelector('[data-slot="agent-activity-chevron"]')).toBeNull();
    expect(container.querySelector('[role="region"]')).toBeNull();
    expect(screen.queryByText("No changes to show")).toBeNull();
  });
});
