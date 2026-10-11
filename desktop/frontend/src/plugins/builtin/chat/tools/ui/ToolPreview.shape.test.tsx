import { afterEach, describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import type { ToolResultRendererSpec } from "@/plugins/sdk/types/tool";
import { definePlugin, TOOL_RESULT_RENDERER } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { selectToolResultRenderer } from "../application/toolResultRenderer";
import { ToolPreview } from "./ToolPreview";

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
});
