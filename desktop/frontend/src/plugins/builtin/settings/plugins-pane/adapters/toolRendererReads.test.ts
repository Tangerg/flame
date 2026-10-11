import { describe, expect, it, vi } from "vitest";
import { createToolRendererReads } from "./toolRendererReads";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";

const binding = { installationId: "installation", digest: "digest", rendererId: "renderer" };
const tool: ToolCall = {
  id: "call",
  runId: "run",
  name: "read",
  fn: "a.go",
  args: "{}",
  status: "ok",
  result: "recorded bytes",
  range: { start: 41, end: 41 },
};

describe("the captured recorded result", () => {
  it("never follows replacement records or mutates the host projection", async () => {
    const captured = structuredClone(tool);
    const request = { ...binding };
    const readRenderer = vi.fn(async (_request: typeof binding, _signal?: AbortSignal) => ({
      html: "<html></html>",
    }));
    const reads = createToolRendererReads({ readRenderer }, request, captured);
    captured.result = "later bytes";
    captured.range!.start = 1;
    request.digest = "replacement";
    const loaded = await reads.load(new AbortController().signal);
    expect(loaded.initial.tool).toEqual(tool);
    loaded.initial.tool.result = "guest edit";
    const refreshed = await reads.read(undefined, new AbortController().signal);
    expect(refreshed.tool).toEqual(tool);
    expect(readRenderer.mock.calls[0]![0]).toEqual(binding);
  });
  it("reauthorizes refresh and refuses guest-selected continuation", async () => {
    const readRenderer = vi.fn(async (_request: typeof binding, _signal?: AbortSignal) => ({
      html: "<html></html>",
    }));
    const reads = createToolRendererReads({ readRenderer }, binding, tool);
    await reads.load(new AbortController().signal);
    readRenderer.mockRejectedValueOnce(new Error("withdrawn"));
    await expect(reads.read(undefined, new AbortController().signal)).rejects.toThrow("withdrawn");
    await expect(reads.read("foreign call", new AbortController().signal)).rejects.toThrow(
      "no continuation",
    );
    expect(readRenderer).toHaveBeenCalledTimes(2);
  });
  it("refuses a refresh whose release resource changed", async () => {
    const readRenderer = vi.fn(async () => ({ html: "<html>original</html>" }));
    const reads = createToolRendererReads({ readRenderer }, binding, tool);
    await reads.load(new AbortController().signal);
    readRenderer.mockResolvedValueOnce({ html: "<html>replacement</html>" });
    await expect(reads.read(undefined, new AbortController().signal)).rejects.toThrow(
      "renderer resource changed",
    );
  });
});
