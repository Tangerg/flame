import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { type ToolResultRendererSpec } from "@/features/chat/public/tools/types";
import { definePlugin } from "@/extensions/definePlugin";
import { TOOL_RESULT_RENDERER } from "@/features/chat/public/tools/types";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { selectToolResultRenderer } from "@/features/chat/tools/application/toolResultRenderer";
import { ToolPreview } from "@/features/chat/tools/ui/ToolPreview";
const tool: ToolCall = {
  id: "t1",
  runId: "r1",
  name: "grep",
  fn: "grep",
  args: "{}",
  status: "ok",
  result: JSON.stringify({ hits: [{ path: "src/a.ts", lineNumber: 12, snippet: "needle here" }] }),
};

function renderer(
  id: string,
  type: "toolName" | "resultShape",
  name: string,
): ToolResultRendererSpec {
  return {
    id,
    title: id,
    selector: { type, name },
    component: ({ fallback }) => (
      <div>
        <p>{id} rendered</p>
        {fallback}
      </div>
    ),
  };
}

afterEach(resetKernelForTest);

describe("optional result rendering", () => {
  it("keeps recorded results readable without a package", () => {
    render(<ToolPreview tool={tool} />);
    expect(screen.getByText(/needle here/)).toBeTruthy();
  });
  it("selects a unique named contribution before a shape contribution regardless of order", () => {
    const named = renderer("named", "toolName", "grep");
    const shaped = renderer("shaped", "resultShape", "search");
    expect(selectToolResultRenderer([shaped, named], tool)).toBe(named);
    expect(selectToolResultRenderer([named, shaped], tool)).toBe(named);
    expect(selectToolResultRenderer([shaped], tool)).toBe(shaped);
  });
  it("uses generic content for competing names or competing shapes", () => {
    const shaped = renderer("shape", "resultShape", "search");
    expect(
      selectToolResultRenderer(
        [renderer("a", "toolName", "grep"), renderer("b", "toolName", "grep"), shaped],
        tool,
      ),
    ).toBeUndefined();
    expect(
      selectToolResultRenderer([shaped, renderer("b", "resultShape", "search")], tool),
    ).toBeUndefined();
  });
  it("never opens a package for provisional output", () => {
    expect(
      selectToolResultRenderer([renderer("a", "toolName", "grep")], { ...tool, status: "running" }),
    ).toBeUndefined();
    expect(
      selectToolResultRenderer([renderer("a", "toolName", "grep")], { ...tool, result: undefined }),
    ).toBeUndefined();
  });
  it("withdraws enhanced presentation while retaining the recorded result", async () => {
    const spec = renderer("enhanced", "toolName", "grep");
    await loadPluginsForTest(
      definePlugin({
        name: "test.result-package",
        setup(ctx) {
          ctx.contribute(TOOL_RESULT_RENDERER, spec, { key: spec.id });
        },
      }),
    );
    render(<ToolPreview tool={tool} />);
    expect(screen.getByText("enhanced rendered")).toBeTruthy();
    expect(screen.getByText(/needle here/)).toBeTruthy();
    await resetKernelForTest();
    expect(screen.queryByText("enhanced rendered")).toBeNull();
    expect(screen.getByText(/needle here/)).toBeTruthy();
  });

  it.each(["enhanced", "fallback", "failed"] as const)(
    "keeps complete arguments after %s result presentation",
    async (mode) => {
      const call = {
        ...tool,
        args: JSON.stringify({
          path: "src/a.ts",
          cursor: { offset: 134, snapshot: "a".repeat(64) },
        }),
      };
      const spec: ToolResultRendererSpec = {
        id: "test-result",
        title: "Test result",
        selector: { type: "toolName", name: "grep" },
        component: ({ fallback }) => {
          if (mode === "failed") throw new Error("Result presentation failed");
          return mode === "fallback" ? (
            fallback
          ) : (
            <div role="region" aria-label="Enhanced result">
              {call.result}
            </div>
          );
        },
      };
      const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
      try {
        await loadPluginsForTest(
          definePlugin({
            name: "test.arguments-result",
            setup(ctx) {
              ctx.contribute(TOOL_RESULT_RENDERER, spec, { key: spec.id });
            },
          }),
        );
        render(<ToolPreview tool={call} />);
        const result = screen.getByRole("region", {
          name: mode === "enhanced" ? "Enhanced result" : "Result",
        });
        expect(result.textContent).toContain("needle here");
        const argumentsButton = screen.getByRole("button", { name: "Arguments" });
        expect(argumentsButton.getAttribute("aria-expanded")).toBe("false");
        fireEvent.click(argumentsButton);
        const argumentsRegion = await screen.findByRole("region", { name: "Arguments" });
        expect(argumentsRegion.textContent).toBe(JSON.stringify(JSON.parse(call.args), null, 2));
        expect(
          result.compareDocumentPosition(argumentsRegion) & Node.DOCUMENT_POSITION_FOLLOWING,
        ).toBeTruthy();
        await resetKernelForTest();
        expect(screen.getByRole("region", { name: "Result" }).textContent).toContain("needle here");
        expect(screen.getByRole("region", { name: "Arguments" })).toBe(argumentsRegion);
        expect(screen.getAllByRole("button", { name: "Arguments" })).toHaveLength(1);
      } finally {
        consoleError.mockRestore();
      }
    },
  );
});
