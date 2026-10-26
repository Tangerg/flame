import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { ToolInspector } from "@/features/chat/tools/ui/ToolInspector";
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

const clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, "clipboard");

const writeText = vi.fn().mockResolvedValue(undefined);

beforeEach(() => {
  writeText.mockClear();
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
});

afterEach(() => {
  if (clipboardDescriptor) {
    Object.defineProperty(navigator, "clipboard", clipboardDescriptor);
  } else {
    Reflect.deleteProperty(navigator, "clipboard");
  }
});

describe("ToolInspector", () => {
  it("leads with the result and keeps complete formatted arguments in secondary details", async () => {
    const call = tool({ result: '{"reviewed":true,"files":["state.go","commit.go"]}' });
    const { container } = testClient.render(<ToolInspector tool={call} />);
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
      const { unmount } = testClient.render(<ToolInspector tool={call} />);
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
    const { container } = testClient.render(<ToolInspector tool={call} />);
    const result = screen.getByRole("region", { name: "Result" });
    expect(result.textContent).toBe(call.result);
    fireEvent.click(screen.getByRole("button", { name: "Arguments" }));
    await waitFor(() =>
      expect(screen.getByRole("region", { name: "Arguments" }).textContent).toBe(call.args),
    );
    expect(container.querySelector("script, component, a")).toBeNull();
  });

  it("presents and copies the exact JSON tokens the tool reported", async () => {
    const call = tool({
      args: String.raw`{"id":9007199254740993,"label":"caf\u00e9 \"{x}\"","id":18446744073709551615}`,
      result: '{"amount":1e400,"offset":-0,"ratio":1.0,"amount":0.10000000000000000555}',
    });
    const presentedResult =
      '{\n  "amount": 1e400,\n  "offset": -0,\n  "ratio": 1.0,\n  "amount": 0.10000000000000000555\n}';
    const presentedArgs = String.raw`{
  "id": 9007199254740993,
  "label": "caf\u00e9 \"{x}\"",
  "id": 18446744073709551615
}`;
    testClient.render(<ToolInspector tool={call} />);

    expect(screen.getByRole("region", { name: "Result" }).textContent).toBe(presentedResult);
    fireEvent.click(screen.getByRole("button", { name: "Arguments" }));
    const args = await screen.findByRole("region", { name: "Arguments" });
    expect(args.textContent).toBe(presentedArgs);

    fireEvent.click(screen.getByRole("button", { name: "Copy code" }));
    await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1));
    expect(writeText).toHaveBeenCalledWith(presentedResult);
  });
});
