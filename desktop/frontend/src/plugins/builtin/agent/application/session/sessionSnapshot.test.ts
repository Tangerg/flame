import { beforeAll, describe, expect, it } from "vitest";
import type { AgentSessionSnapshot } from "../ports/runtimeGateway";
import { projectAgentSessionSnapshot } from "./sessionSnapshot";
import { selectAwaitingInterrupts } from "../view/awaitingInterrupts";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { foldTestEvent } from "../fold/reducer.fixtures";

const SESSION_ID = "ses_snapshot";
const ROOT_RUN_ID = "run_root";
const CHILD_RUN_ID = "run_child";
const RUNNING_CHILD_RUN_ID = "run_child_running";
const LOST_RUN_ID = "run_lost";
const usage = { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 };

beforeAll(async () => {
  await loadPluginsForTest();
});

describe("projectAgentSessionSnapshot", () => {
  it.each(["approve", "deny"] as const)(
    "rebuilds the Runtime %s verdict identically after resume and reload",
    (approvalDecision) => {
      const run: AgentSessionSnapshot["runs"][number] = {
        id: ROOT_RUN_ID,
        sessionId: SESSION_ID,
        status: "running",
        parentRunId: null,
        rootRunId: ROOT_RUN_ID,
        spawnedByItemId: null,
        activeSegmentId: "seg_resume",
        outcome: null,
        finishedAt: null,
        createdAt: "2026-07-30T01:00:00.000Z",
        metrics: { steps: 2, activeDurationMillis: 20, usage },
      };
      const tool: Extract<AgentSessionSnapshot["items"][number], { type: "toolCall" }> = {
        type: "toolCall",
        id: "item_approval",
        runId: ROOT_RUN_ID,
        status: "running",
        startedAt: "2026-07-30T01:00:01.000Z",
        tool: { name: "shell", arguments: { command: "npm test" } },
      };
      const snapshot: AgentSessionSnapshot = {
        runs: [run],
        items: [tool],
        pendingInterruptSets: [],
      };
      const withoutVerdict = projectAgentSessionSnapshot(snapshot);
      expect(withoutVerdict.messages.flatMap((message) => message.blocks)).toEqual([
        { kind: "tool", toolCallId: tool.id },
      ]);

      let live = projectAgentSessionSnapshot({
        ...snapshot,
        runs: [{ ...run, status: "waiting", activeSegmentId: null }],
        pendingInterruptSets: [
          {
            rootRunId: ROOT_RUN_ID,
            sessionId: SESSION_ID,
            createdAt: tool.startedAt,
            interrupts: [
              {
                type: "approval",
                itemId: tool.id,
                runId: tool.runId,
                payload: { tool: tool.tool, reason: "Verify the change", rememberable: true },
              },
            ],
          },
        ],
      });
      live = foldTestEvent(live, { type: "segment.started", run }, ROOT_RUN_ID, "seg_resume");
      expect(live.toolCalls[tool.id]?.approvalDecision).toBeUndefined();
      expect(selectAwaitingInterrupts(live).size).toBe(0);

      const decided: typeof tool =
        approvalDecision === "approve"
          ? { ...tool, approvalDecision }
          : {
              ...tool,
              approvalDecision,
              status: "completed",
              finishedAt: "2026-07-30T01:00:02.000Z",
              error: { code: "denied_by_user" },
            };
      const event =
        decided.status === "running"
          ? ({ type: "item.started", item: decided } as const)
          : ({ type: "item.completed", item: decided } as const);
      live = foldTestEvent(live, event, ROOT_RUN_ID, "seg_resume");
      const reloaded = projectAgentSessionSnapshot({ ...snapshot, items: [decided] });
      expect(reloaded.toolCalls[tool.id]?.approvalDecision).toBe(approvalDecision);
      expect(reloaded.messages.flatMap((message) => message.blocks)).toEqual([
        { kind: "tool", toolCallId: tool.id },
        expect.objectContaining({ kind: "approval", itemId: tool.id }),
      ]);
      expect(reloaded.messages).toEqual(live.messages);
      expect(reloaded.toolCalls).toEqual(live.toolCalls);
      expect(reloaded.pendingInterrupts).toEqual(live.pendingInterrupts);
      expect(foldTestEvent(live, event, ROOT_RUN_ID, "seg_resume").messages).toEqual(live.messages);
    },
  );

  it("rebuilds one complete source-owned view off-store", () => {
    const snapshot: AgentSessionSnapshot = {
      runs: [
        {
          id: LOST_RUN_ID,
          sessionId: SESSION_ID,
          status: "finished",
          parentRunId: null,
          rootRunId: LOST_RUN_ID,
          spawnedByItemId: null,
          activeSegmentId: null,
          createdAt: "2026-07-30T00:59:00.000Z",
          finishedAt: "2026-07-30T00:59:30.000Z",
          outcome: {
            type: "lost",
            error: {
              code: "run_lost",
              message: "runtime restarted before the run settled",
            },
          },
          metrics: { steps: 3, activeDurationMillis: 30, usage },
        },
        {
          id: ROOT_RUN_ID,
          sessionId: SESSION_ID,
          status: "waiting",
          parentRunId: null,
          rootRunId: ROOT_RUN_ID,
          spawnedByItemId: null,
          activeSegmentId: null,
          outcome: null,
          finishedAt: null,
          createdAt: "2026-07-30T01:00:00.000Z",
          metrics: { steps: 2, activeDurationMillis: 20, usage },
        },
        {
          id: CHILD_RUN_ID,
          sessionId: SESSION_ID,
          status: "waiting",
          activeSegmentId: null,
          outcome: null,
          finishedAt: null,
          createdAt: "2026-07-30T01:00:01.000Z",
          metrics: { steps: 1, activeDurationMillis: 10, usage },
          parentRunId: ROOT_RUN_ID,
          rootRunId: ROOT_RUN_ID,
          spawnedByItemId: "item_spawn",
        },
        {
          id: RUNNING_CHILD_RUN_ID,
          sessionId: SESSION_ID,
          status: "running",
          activeSegmentId: "seg_child_running",
          outcome: null,
          finishedAt: null,
          createdAt: "2026-07-30T01:00:01.500Z",
          metrics: { steps: 1, activeDurationMillis: 5, usage },
          parentRunId: ROOT_RUN_ID,
          rootRunId: ROOT_RUN_ID,
          spawnedByItemId: "item_spawn_running",
        },
      ],
      items: [
        {
          type: "userMessage",
          id: "item_user",
          runId: ROOT_RUN_ID,
          status: "completed",
          createdAt: "2026-07-30T01:00:00.100Z",
          content: [{ type: "text", text: "delegate this" }],
        },
        {
          type: "agentMessage",
          id: "item_child_reply",
          runId: CHILD_RUN_ID,
          status: "completed",
          createdAt: "2026-07-30T01:00:01.100Z",
          content: [{ type: "text", text: "working" }],
        },
      ],
      pendingInterruptSets: [
        {
          rootRunId: ROOT_RUN_ID,
          sessionId: SESSION_ID,
          createdAt: "2026-07-30T01:00:02.000Z",
          interrupts: [
            {
              type: "approval",
              itemId: "item_approval",
              runId: CHILD_RUN_ID,
              payload: {
                tool: { name: "shell", arguments: { command: "npm test" } },
              },
            },
          ],
        },
      ],
      plan: {
        revision: 4,
        steps: [{ id: "step_1", text: "Verify", status: "active" }],
      },
    };

    const view = projectAgentSessionSnapshot(snapshot);

    expect(view.runsById[ROOT_RUN_ID]).toMatchObject({
      parentRunId: null,
      rootRunId: ROOT_RUN_ID,
      status: "waiting",
    });
    expect(view.runsById[CHILD_RUN_ID]).toMatchObject({
      parentRunId: ROOT_RUN_ID,
      rootRunId: ROOT_RUN_ID,
      status: "waiting",
    });
    expect(view.runsById[RUNNING_CHILD_RUN_ID]).toMatchObject({
      parentRunId: ROOT_RUN_ID,
      rootRunId: ROOT_RUN_ID,
      status: "running",
      activeSegmentId: "seg_child_running",
    });
    expect(view.runsById[LOST_RUN_ID]).toMatchObject({
      status: "finished",
      outcome: {
        type: "lost",
        error: {
          code: "run_lost",
          message: "runtime restarted before the run settled",
        },
      },
    });
    expect(view.messages.map(({ id, runId }) => ({ id, runId }))).toEqual([
      { id: "item_user", runId: ROOT_RUN_ID },
      { id: "turn:item_child_reply", runId: CHILD_RUN_ID },
    ]);
    expect(
      view.messages.flatMap((message) => message.blocks).find((block) => block.kind === "approval"),
    ).toMatchObject({ itemId: "item_approval" });
    expect(selectAwaitingInterrupts(view).get("item_approval")).toBe(ROOT_RUN_ID);
    expect(view.pendingInterrupts).toEqual([
      {
        runId: CHILD_RUN_ID,
        interrupts: [{ itemId: "item_approval", kind: "approval" }],
      },
    ]);
    expect(view.plan).toMatchObject({ revision: 4 });
  });
});
