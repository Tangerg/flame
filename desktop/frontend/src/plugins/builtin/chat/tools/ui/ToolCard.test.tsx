import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { ToolCard } from "./ToolCard";

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
  // Codex keeps the invocation itself in the work narrative: read, write, running, failed
  // and refused calls all use the same transparent row. The material result (terminal output
  // or diff) earns a surface only after the row is opened, and the tone stays neutral —
  // colouring the identity glyph turns lifecycle back into a status card.
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
});
