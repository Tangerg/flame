import { describe, expect, it } from "vitest";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { type ToolResultRendererSpec } from "@/features/chat/public/tools/types";
import { resolveRenderer } from "@/features/plugins/public/renderers";
import { toolRendererCandidates } from "@/features/chat/tools/application/toolResultRenderer";

const tool: ToolCall = {
  id: "t1",
  runId: "r1",
  name: "grep",
  resultShape: "search",
  fn: "grep",
  args: "{}",
  status: "ok",
  result: JSON.stringify({ hits: [{ path: "src/a.ts", lineNumber: 12, snippet: "needle here" }] }),
};

function renderer(
  id: string,
  selector: ToolResultRendererSpec["selector"],
): ToolResultRendererSpec {
  return {
    id,
    title: id,
    selector,
    component: () => null,
  };
}

describe("optional result rendering", () => {
  it("selects a unique named contribution before a shape contribution regardless of order", () => {
    const named = renderer("named", { type: "toolName", name: "grep" });
    const shaped = renderer("shaped", { type: "resultShape", shape: "search" });
    expect(selectToolResultRenderer([shaped, named], tool)).toBe(named);
    expect(selectToolResultRenderer([named, shaped], tool)).toBe(named);
    expect(selectToolResultRenderer([shaped], tool)).toBe(shaped);
  });

  it("uses generic content for competing names or competing shapes", () => {
    const shaped = renderer("shape", { type: "resultShape", shape: "search" });
    expect(
      selectToolResultRenderer(
        [
          renderer("a", { type: "toolName", name: "grep" }),
          renderer("b", { type: "toolName", name: "grep" }),
          shaped,
        ],
        tool,
      ),
    ).toBeUndefined();
    expect(
      selectToolResultRenderer(
        [shaped, renderer("b", { type: "resultShape", shape: "search" })],
        tool,
      ),
    ).toBeUndefined();
  });
  it("never opens a package for provisional output", () => {
    expect(
      selectToolResultRenderer([renderer("a", { type: "toolName", name: "grep" })], {
        ...tool,
        status: "running",
      }),
    ).toBeUndefined();
    expect(
      selectToolResultRenderer([renderer("a", { type: "toolName", name: "grep" })], {
        ...tool,
        result: undefined,
      }),
    ).toBeUndefined();
  });
});

function selectToolResultRenderer(
  renderers: readonly ToolResultRendererSpec[],
  target: Parameters<typeof toolRendererCandidates>[1],
) {
  return resolveRenderer(toolRendererCandidates(renderers, target), null);
}
