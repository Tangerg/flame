import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { ToolGroup } from "@/features/chat/tools/ui/ToolGroup";
describe("ToolGroup", () => {
  it("updates the folded summary as a call settles without overriding the user's fold", () => {
    const calls: ToolCall[] = [
      { id: "read", runId: "r1", name: "read", fn: "file.ts", args: "", status: "running" },
      { id: "search", runId: "r1", name: "grep", fn: "pattern", args: "", status: "ok" },
    ];
    const group = () => (
      <ToolGroup tools={calls} expandedIds={new Set()} onToggleExpand={vi.fn()} />
    );
    const { rerender } = render(group());
    const summary = screen.getByRole("button", { name: /1 running.*2 calls/ });
    expect(summary.getAttribute("aria-expanded")).toBe("true");

    fireEvent.click(summary);
    calls[0] = { ...calls[0]!, status: "err" };
    rerender(group());

    expect(summary.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getByRole("button", { name: /1 issue.*2 calls/ })).toBe(summary);
    expect(screen.queryByText("1 running")).toBeNull();
  });
});
