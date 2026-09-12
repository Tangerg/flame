import { beforeEach, describe, expect, it } from "vitest";
import type { AgentItem as Item, AgentStreamEvent as StreamEvent } from "@/plugins/sdk";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { foldTestEvent as reduce } from "./reducer.fixtures";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

function item(partial: Record<string, unknown>): Item {
  return {
    runId: "run_1",
    status: "running",
    createdAt: "2026-06-03T00:00:00Z",
    ...partial,
  } as Item;
}
const started = (i: Item): StreamEvent => ({ type: "item.started", item: i });
const completed = (i: Item): StreamEvent => ({ type: "item.completed", item: i });
const delta = (itemId: string, d: Record<string, unknown>): StreamEvent =>
  ({ type: "item.delta", itemId, delta: d }) as StreamEvent;

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPluginsForTest(spec);
});

const cmd = (result: Record<string, unknown>) => ({
  name: "shell",
  arguments: { command: "pwd", description: "Print the working directory" },
  ...(Object.keys(result).length > 0 ? { result } : {}),
});

describe("reducer — commandExecution output durability", () => {
  it("history replay (completed-only, no deltas) renders output from tool.output", () => {
    const s = reduce(
      EMPTY_AGENT_SESSION_VIEW,
      completed(
        item({
          id: "t1",
          status: "completed",
          type: "toolCall",
          tool: cmd({ output: "/Users/tangerg\n", exitCode: 0 }),
        }),
      ),
    );
    expect(s.toolCalls["t1"]?.result).toBe("/Users/tangerg\n");
    expect(s.toolCalls["t1"]?.exitCode).toBe(0);
  });

  it("completed `output` is authoritative — overrides an incomplete delta preview", () => {
    let s: AgentSessionView = EMPTY_AGENT_SESSION_VIEW;
    s = reduce(s, started(item({ id: "t1", type: "toolCall", tool: cmd({}) })));
    s = reduce(s, delta("t1", { type: "toolOutput", text: "/Users" }));
    expect(s.toolCalls["t1"]?.result).toBe("/Users");
    s = reduce(
      s,
      completed(
        item({
          id: "t1",
          status: "completed",
          type: "toolCall",
          tool: cmd({ output: "/Users/tangerg\n", exitCode: 0 }),
        }),
      ),
    );
    expect(s.toolCalls["t1"]?.result).toBe("/Users/tangerg\n");
  });

  it("while running the toolOutput delta is the live preview (no settled fields yet)", () => {
    let s: AgentSessionView = EMPTY_AGENT_SESSION_VIEW;
    s = reduce(s, started(item({ id: "t1", type: "toolCall", tool: cmd({}) })));
    s = reduce(s, delta("t1", { type: "toolOutput", text: "/Users/tan" }));
    s = reduce(s, delta("t1", { type: "toolOutput", text: "gerg\n" }));
    expect(s.toolCalls["t1"]?.status).toBe("running");
    expect(s.toolCalls["t1"]?.result).toBe("/Users/tangerg\n");
  });

  it("ignores a delta that arrives after the tool has settled", () => {
    let s: AgentSessionView = EMPTY_AGENT_SESSION_VIEW;
    s = reduce(s, started(item({ id: "t1", type: "toolCall", tool: cmd({}) })));
    s = reduce(s, delta("t1", { type: "toolOutput", text: "partial" }));
    s = reduce(
      s,
      completed(
        item({
          id: "t1",
          status: "completed",
          type: "toolCall",
          tool: cmd({ output: "/Users/tangerg\n", exitCode: 0 }),
        }),
      ),
    );

    const settled = reduce(s, delta("t1", { type: "toolOutput", text: " and more" }));

    expect(settled.toolCalls["t1"]?.result).toBe("/Users/tangerg\n");
  });

  it("carries the command itself alongside the description-derived label", () => {
    const s = reduce(
      EMPTY_AGENT_SESSION_VIEW,
      completed(
        item({
          id: "t1",
          status: "completed",
          type: "toolCall",
          tool: cmd({ output: "/Users/tangerg\n", exitCode: 0 }),
        }),
      ),
    );
    expect(s.toolCalls["t1"]).toMatchObject({
      fn: "Print the working directory",
      command: "pwd",
      result: "/Users/tangerg\n",
    });
  });
});
