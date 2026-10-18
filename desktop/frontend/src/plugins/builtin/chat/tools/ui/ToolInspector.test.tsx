import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { ToolInspector } from "./ToolInspector";

function tool(values: Partial<ToolCall>): ToolCall {
  return {
    id: "inspection",
    runId: "run",
    name: "inspect_workspace",
    fn: "inspect_workspace",
    args: '{"workspace":{"root":"/workspace/Flame sources"},"checks":["ownership"]}',
    status: "ok",
    ...values,
  };
}

describe("ToolInspector", () => {
  it("leads with the result and keeps complete formatted arguments in secondary details", async () => {
    const call = tool({ result: '{"reviewed":true,"files":["state.go","commit.go"]}' });
    const { container } = render(<ToolInspector tool={call} />);
    const result = screen.getByRole("region", { name: "Result" });
    expect(result.textContent).toBe(JSON.stringify(JSON.parse(call.result!), null, 2));
    const details = screen.getByRole("button", { name: "Arguments" });
    expect(details.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("region", { name: "Arguments" })).toBeNull();

    fireEvent.click(details);
    const args = await screen.findByRole("region", { name: "Arguments" });
    expect(args.textContent).toBe(JSON.stringify(JSON.parse(call.args), null, 2));
    expect(result.compareDocumentPosition(args) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(container.querySelectorAll('button[aria-label="Copy code"]')).toHaveLength(1);
  });

  it("keeps arguments immediately readable when no result exists, with an empty-result notice only after success", () => {
    for (const status of ["running", "err", "ok"] as const) {
      const call = tool({ status });
      const { unmount } = render(<ToolInspector tool={call} />);
      expect(screen.getByRole("region", { name: "Arguments" }).textContent).toBe(
        JSON.stringify(JSON.parse(call.args), null, 2),
      );
      expect(screen.getByRole("button", { name: "Arguments" }).getAttribute("aria-expanded")).toBe(
        "true",
      );
      expect(screen.queryByText("(no result body — tool returned empty)") !== null).toBe(
        status === "ok",
      );
      unmount();
    }
  });

  it("preserves malformed source and markup as literal text in both sections", async () => {
    const call = tool({
      args: "{not json <component>}",
      result:
        "<script>globalThis.inspectorExecuted = true</script>\nhttps://example.invalid/source",
    });
    const { container } = render(<ToolInspector tool={call} />);
    const result = screen.getByRole("region", { name: "Result" });
    expect(result.textContent).toBe(call.result);
    fireEvent.click(screen.getByRole("button", { name: "Arguments" }));
    await waitFor(() =>
      expect(screen.getByRole("region", { name: "Arguments" }).textContent).toBe(call.args),
    );
    expect(container.querySelector("script, component, a")).toBeNull();
  });
});
