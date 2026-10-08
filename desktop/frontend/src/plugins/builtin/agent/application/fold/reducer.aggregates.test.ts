import { describe, expect, it } from "vitest";
import type { AgentItem as Item, AgentStreamEvent as StreamEvent } from "@/plugins/sdk";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { foldTestEvent as reduce } from "./reducer.fixtures";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";

function item(partial: Record<string, unknown>): Item {
  return {
    runId: "r1",
    status: "running",
    ...(partial.type === "toolCall"
      ? {
          startedAt: "2026-06-03T00:00:02Z",
          ...(partial.status === "running" || partial.status === undefined
            ? {}
            : { finishedAt: "2026-06-03T00:00:03Z" }),
        }
      : { createdAt: "2026-06-03T00:00:00Z" }),
    ...partial,
  } as Item;
}
const completed = (i: Item): StreamEvent => ({ type: "item.completed", item: i });

describe("reducer — plan", () => {
  const plan = (revision: number, description: string): StreamEvent => ({
    type: "plan.updated",
    plan: {
      revision,
      steps: [{ id: "1", text: description, status: "pending" }],
    },
  });

  it("a plan update replaces the Plan wholesale", () => {
    const s = reduce(EMPTY_AGENT_SESSION_VIEW, plan(1, "first"));
    expect(s.plan).toMatchObject({ revision: 1, steps: [{ text: "first" }] });
  });

  it("an older revision does not overwrite a newer one", () => {
    let s = reduce(EMPTY_AGENT_SESSION_VIEW, plan(4, "current"));
    s = reduce(s, plan(2, "stale"));
    expect(s.plan).toMatchObject({ revision: 4, steps: [{ text: "current" }] });
  });

  it("a duplicate revision is a no-op even if a drifted replay arrives", () => {
    const current = reduce(EMPTY_AGENT_SESSION_VIEW, plan(4, "current"));
    const duplicate = reduce(current, plan(4, "drifted replay"));

    expect(duplicate).toBe(current);
    expect(duplicate.plan).toMatchObject({
      revision: 4,
      steps: [{ text: "current" }],
    });
  });
});

describe("reducer — durable history hydration", () => {
  it("item.completed without a prior item.started upserts the block (items.list replay)", () => {
    let s: AgentSessionView = EMPTY_AGENT_SESSION_VIEW;
    s = reduce(
      s,
      completed(
        item({
          id: "u1",
          type: "userMessage",
          status: "completed",
          content: [{ type: "text", text: "hi" }],
        }),
      ),
    );
    s = reduce(
      s,
      completed(
        item({
          id: "a1",
          type: "agentMessage",
          status: "completed",
          content: [{ type: "text", text: "hello" }],
        }),
      ),
    );
    expect(s.messages.map((m) => m.role)).toEqual(["user", "assistant"]);
    expect(s.messages[1]!.blocks[0]).toMatchObject({
      kind: "text",
      text: "hello",
      status: "complete",
    });
  });
});
