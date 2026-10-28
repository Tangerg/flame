import { beforeEach, describe, expect, it, onTestFinished } from "vitest";
import { act, fireEvent, screen } from "@testing-library/react";
import { useLayoutEffect, useRef } from "react";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { definePlugin } from "@/extensions/definePlugin";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { ContentVisibility } from "@/ui/index";
import { type ToolCall } from "@/features/agent/public/sessionView";
import {
  TOOL_RESULT_RENDERER,
  type ToolResultRendererSpec,
} from "@/features/chat/public/tools/types";
import { ToolPreview } from "./ToolPreview";

let client: TestClient;
beforeEach(() => {
  const owned = createTestClient();
  client = owned;
  onTestFinished(() => owned.dispose());
});

const tool: ToolCall = {
  id: "captured-read",
  runId: "run-1",
  name: "read",
  fn: "src/session.ts",
  args: '{"path":"src/session.ts"}',
  status: "ok",
  result: "captured source\nsecond line",
};

function EnhancedResult() {
  return (
    <div role="region" aria-label="Enhanced result">
      {tool.result}
      <button type="button">Find in result</button>
    </div>
  );
}

function FocusedResult() {
  const control = useRef<HTMLButtonElement>(null);
  useLayoutEffect(() => control.current?.focus(), []);
  return (
    <button type="button" ref={control}>
      Result action
    </button>
  );
}

async function installResult(component: ToolResultRendererSpec["component"] = EnhancedResult) {
  const spec: ToolResultRendererSpec = {
    id: "captured-result",
    title: "Captured result",
    selector: { type: "toolName", name: "read" },
    component,
  };
  await loadPluginsForTest(
    definePlugin({
      name: "test.result-focus",
      setup(ctx) {
        ctx.contribute(TOOL_RESULT_RENDERER, spec);
      },
    }),
  );
}

function focus(control: HTMLElement) {
  act(() => control.focus());
  expect(document.activeElement).toBe(control);
}

describe("Tool result presentation focus", () => {
  it.each(["running", "err"] as const)("does not name an absent %s result", (status) => {
    client.render(<ToolPreview tool={{ ...tool, status, result: undefined }} />);
    expect(screen.queryByRole("group", { name: "Result" })).toBeNull();
    expect(screen.getByRole("region", { name: "Arguments" })).toBeTruthy();
  });

  it.each(["reader", "copy", "wrap"])(
    "keeps the same result context when completion removes its focused %s",
    async (origin) => {
      await installResult();
      const { rerender } = client.render(<ToolPreview tool={{ ...tool, status: "running" }} />);
      const host = screen.getByRole("group", { name: "Result" });
      const source =
        origin === "reader"
          ? screen.getByRole("region", { name: "Result" })
          : screen.getByRole("button", {
              name: origin === "copy" ? "Copy code" : "Enable word wrap",
            });
      focus(source);

      rerender(<ToolPreview tool={tool} />);

      expect(screen.getByRole("region", { name: "Enhanced result" }).textContent).toContain(
        tool.result,
      );
      expect(source.isConnected).toBe(false);
      expect(screen.getByRole("group", { name: "Result" })).toBe(host);
      expect(document.activeElement).toBe(host);
      expect(host.tabIndex).toBe(-1);
    },
  );

  it("retains local reading when a contribution arrives or is withdrawn", async () => {
    client.render(<ToolPreview tool={tool} />);
    const host = screen.getByRole("group", { name: "Result" });
    focus(screen.getByRole("region", { name: "Result" }));
    await installResult();
    expect(document.activeElement).toBe(host);

    focus(screen.getByRole("button", { name: "Find in result" }));
    await resetKernelForTest();

    expect(screen.getByRole("region", { name: "Result" }).textContent).toContain(tool.result);
    expect(document.activeElement).toBe(host);
  });

  it("keeps the existing reader and position during provisional output updates", () => {
    const { rerender } = client.render(<ToolPreview tool={{ ...tool, status: "running" }} />);
    const reader = screen.getByRole("region", { name: "Result" });
    focus(reader);
    reader.scrollTop = 45;

    rerender(<ToolPreview tool={{ ...tool, status: "running", result: `${tool.result}\nmore` }} />);

    expect(screen.getByRole("region", { name: "Result" })).toBe(reader);
    expect(reader.textContent).toContain("more");
    expect(reader.scrollTop).toBe(45);
    expect(document.activeElement).toBe(reader);
  });

  it("preserves the separately mounted Arguments reader through completion", async () => {
    await installResult();
    const { rerender } = client.render(<ToolPreview tool={{ ...tool, status: "running" }} />);
    fireEvent.click(screen.getByRole("button", { name: "Arguments" }));
    const args = await screen.findByRole("region", { name: "Arguments" });
    focus(args);

    rerender(<ToolPreview tool={tool} />);

    expect(screen.getByRole("region", { name: "Arguments" })).toBe(args);
    expect(document.activeElement).toBe(args);
  });

  it("does not reclaim a later outside editing target", async () => {
    await installResult();
    const render = (call: ToolCall) => (
      <>
        <input aria-label="Draft" defaultValue="Keep this draft" />
        <ToolPreview tool={call} />
      </>
    );
    const { rerender } = client.render(render({ ...tool, status: "running" }));
    focus(screen.getByRole("region", { name: "Result" }));
    const draft = screen.getByRole("textbox", { name: "Draft" });
    focus(draft);

    rerender(render(tool));

    expect(screen.getByRole("textbox", { name: "Draft" })).toBe(draft);
    expect(document.activeElement).toBe(draft);
    expect((draft as HTMLInputElement).value).toBe("Keep this draft");
  });

  it("leaves a replacement Tool's arrival to its owner", async () => {
    await installResult();
    const { rerender } = client.render(<ToolPreview tool={{ ...tool, status: "running" }} />);
    focus(screen.getByRole("region", { name: "Result" }));

    rerender(<ToolPreview tool={{ ...tool, id: "another-read" }} />);

    expect(document.activeElement).not.toBe(screen.getByRole("group", { name: "Result" }));
  });

  it.each(["visibility", "hidden", "inert"] as const)(
    "does not focus a retiring result through %s or queue a return on reveal",
    async (mode) => {
      await installResult();
      const render = (call: ToolCall, hidden: boolean) => (
        <ContentVisibility visible={mode !== "visibility" || !hidden}>
          <div hidden={mode === "hidden" && hidden} inert={mode === "inert" && hidden}>
            <ToolPreview tool={call} />
          </div>
        </ContentVisibility>
      );
      const { rerender } = client.render(render({ ...tool, status: "running" }, false));
      const host = screen.getByRole("group", { name: "Result" });
      focus(screen.getByRole("region", { name: "Result" }));

      rerender(render(tool, true));
      expect(document.activeElement).not.toBe(host);
      rerender(render(tool, false));
      expect(document.activeElement).not.toBe(host);
    },
  );

  it("respects focus claimed by the newly presented result", async () => {
    await installResult(FocusedResult);
    const { rerender } = client.render(<ToolPreview tool={{ ...tool, status: "running" }} />);
    focus(screen.getByRole("region", { name: "Result" }));

    rerender(<ToolPreview tool={tool} />);

    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Result action" }));
  });
});
