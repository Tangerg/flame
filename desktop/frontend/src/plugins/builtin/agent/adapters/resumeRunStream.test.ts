import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  asRunId,
  asSegmentId,
  RpcConnectionError,
  RpcError,
  type FlameClient,
  type RunEvent,
  type SessionSnapshot,
} from "@flame/runtime-contract/client";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { useAgentStore } from "./agentStore";
import { installAgentRuntimeGateway } from "./agentRuntimeGateway";
import { replaceResumedRunStream } from "./resumeRunStream";

const SESSION = "ses_resume";
const RUN = asRunId("run_resume");
const SEGMENT = asSegmentId("seg_resume");

beforeEach(async () => {
  await loadPluginsForTest();
  useAgentStore.getState().dropSession(SESSION);
  useAgentStore.getState().ensureSession(SESSION);
});

function acceptedStream() {
  const close = vi.fn(async () => ({ done: true, value: undefined }) as const);
  return {
    close,
    stream: {
      result: { runId: RUN, segmentId: SEGMENT },
      events: {
        [Symbol.asyncIterator]: () => ({
          next: () => new Promise<IteratorResult<RunEvent>>(() => {}),
          return: close,
        }),
      },
    },
  };
}

function material(): SessionSnapshot {
  return {
    runs: [
      {
        id: RUN,
        sessionId: SESSION,
        provider: "test",
        model: "test",
        status: "running",
        activeSegmentId: SEGMENT,
        createdAt: "2026-09-28T01:00:00Z",
        metrics: { steps: 0, activeDurationMillis: 0 },
        protocolProfile: { interruptTypes: ["approval", "question"], requiredFeatures: [] },
      },
    ],
    items: [
      {
        id: "item_answer",
        runId: RUN,
        type: "question",
        status: "completed",
        createdAt: "2026-09-28T01:00:02Z",
        question: {
          fields: [{ type: "text", prompt: "Which database?" }],
          answers: [["Postgres"]],
        },
      },
      {
        id: "item_tool",
        runId: RUN,
        type: "toolCall",
        status: "running",
        startedAt: "2026-09-28T01:00:03Z",
        approvalDecision: "approve",
        tool: { name: "shell", arguments: { command: "pwd" } },
      },
    ],
    interrupts: [],
  };
}

describe("accepted resume stream", () => {
  it("replaces the accepted transport with an atomic snapshot and its tail", async () => {
    const accepted = acceptedStream();
    const snapshot = material();
    const events = (async function* (): AsyncIterable<RunEvent> {})();
    const subscribe = vi.fn(async () => ({
      result: { runId: RUN, segmentId: SEGMENT, headEventId: "evt_snapshot", snapshot },
      events,
    }));
    const client = { runs: { subscribe } } as unknown as FlameClient;
    const signal = new AbortController().signal;

    const tail = await replaceResumedRunStream({
      client,
      sessionId: SESSION,
      stream: accepted.stream,
      signal,
      isCancelled: () => false,
    });

    const view = useAgentStore.getState().sessions[SESSION]!.view;
    expect(view.pendingInterrupts).toEqual([]);
    expect(view.messages.flatMap((message) => message.blocks)).toContainEqual(
      expect.objectContaining({ kind: "question", answered: true, answers: [["Postgres"]] }),
    );
    expect(view.toolCalls.item_tool).toMatchObject({
      status: "running",
      approvalDecision: "approved",
    });
    expect(accepted.close).toHaveBeenCalledOnce();
    expect(subscribe).toHaveBeenCalledWith(
      { runId: RUN, segmentId: SEGMENT, snapshot: true },
      signal,
    );
    expect(tail).toMatchObject({ result: { headEventId: "evt_snapshot" }, events });
  });

  it("reads the completed authority if execution finishes before the snapshot subscription", async () => {
    const accepted = acceptedStream();
    const snapshot = material();
    snapshot.runs = snapshot.runs.map((run) => ({
      ...run,
      status: "finished",
      activeSegmentId: undefined,
      finishedAt: "2026-09-28T01:00:04Z",
      outcome: { type: "completed" },
    }));
    const subscribe = vi
      .fn()
      .mockRejectedValue(
        new RpcError({ code: -32002, message: "run finished", data: { type: "run_finished" } }),
      );
    const read = vi.fn().mockResolvedValue(snapshot);
    const client = { runs: { subscribe }, sessions: { snapshot: read } } as unknown as FlameClient;
    const gateway = installAgentRuntimeGateway(() => client);
    try {
      const tail = await replaceResumedRunStream({
        client,
        sessionId: SESSION,
        stream: accepted.stream,
        signal: new AbortController().signal,
        isCancelled: () => false,
      });
      expect(read).toHaveBeenCalledOnce();
      expect(tail).toBeNull();
      expect(useAgentStore.getState().sessions[SESSION]!.view.runsById[RUN]?.status).toBe(
        "finished",
      );
    } finally {
      gateway.dispose();
    }
  });

  it("retires a late snapshot without publishing into a successor view", async () => {
    const accepted = acceptedStream();
    const pending = Promise.withResolvers<Awaited<ReturnType<FlameClient["runs"]["subscribe"]>>>();
    const client = { runs: { subscribe: () => pending.promise } } as unknown as FlameClient;
    const controller = new AbortController();
    const following = replaceResumedRunStream({
      client,
      sessionId: SESSION,
      stream: accepted.stream,
      signal: controller.signal,
      isCancelled: () => false,
    });
    controller.abort();
    await expect(following).resolves.toBeNull();
    const late = acceptedStream();
    pending.resolve({
      ...late.stream,
      result: { ...late.stream.result, snapshot: material() },
    });
    await vi.waitFor(() => expect(late.close).toHaveBeenCalledOnce());
    expect(useAgentStore.getState().sessions[SESSION]!.view.messages).toEqual([]);
  });

  it("surfaces snapshot failure after detaching the accepted command exactly once", async () => {
    const accepted = acceptedStream();
    const error = new RpcConnectionError("connection lost after acceptance");
    const subscribe = vi.fn().mockRejectedValue(error);
    const client = { runs: { subscribe } } as unknown as FlameClient;
    await expect(
      replaceResumedRunStream({
        client,
        sessionId: SESSION,
        stream: accepted.stream,
        signal: new AbortController().signal,
        isCancelled: () => false,
      }),
    ).rejects.toBe(error);
    expect(accepted.close).toHaveBeenCalledOnce();
    expect(subscribe).toHaveBeenCalledOnce();
  });
});
