import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RpcConnectionError, RpcError, RpcProtocolError, type FlameClient } from "@/rpc";
import { asRunId, asSegmentId } from "@/rpc";
import type { RunStream, RunStreamPosition } from "./agentRunPump";
import { createRunStreamReattach } from "./runStreamReattach";

import { useAgentStore } from "./agentStore";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

beforeEach(async () => {
  await loadPluginsForTest();
  useAgentStore.getState().ensureSession("ses_1");
});
afterEach(() => {
  useAgentStore.getState().dropSession("ses_1");
  vi.restoreAllMocks();
});

const RUN = asRunId("run_1");
const SEGMENT = asSegmentId("seg_1");

function emptyStream(): RunStream {
  return {
    result: { runId: RUN, segmentId: SEGMENT },
    events: (async function* () {})(),
  };
}

function position(recovery: RunStreamPosition["recovery"]): RunStreamPosition {
  return {
    runId: RUN,
    segmentId: SEGMENT,
    lastEventId: "evt_7",
    recovery,
  };
}

function runClient(subscribe: FlameClient["runs"]["subscribe"]): Pick<FlameClient, "runs"> {
  return { runs: { subscribe } as FlameClient["runs"] };
}

describe("run stream reattach", () => {
  it("propagates invalid acknowledgements to the pump synchronization diagnostic", async () => {
    const error = new RpcProtocolError("runs.subscribe result", [], "request_invalid_ack");
    const subscribe = vi.fn<FlameClient["runs"]["subscribe"]>().mockRejectedValue(error);
    const recoverProjection = vi.fn(async () => {});
    const reattach = createRunStreamReattach({
      sessionId: "ses_1",
      client: () => runClient(subscribe),
      isCancelled: () => false,
      recoverProjection,
    });
    await expect(reattach(position("replay"), new AbortController().signal)).rejects.toBe(error);
    expect(recoverProjection).not.toHaveBeenCalled();
  });

  it("commits the snapshot supplied with the cold tail and returns its successor cursor", async () => {
    useAgentStore.getState().setCommandError("ses_1", { code: "old" });
    const recoverProjection = vi.fn(async (_signal: AbortSignal) => {});
    const subscribe = vi.fn<FlameClient["runs"]["subscribe"]>(async () => {
      const stream = emptyStream();
      return {
        ...stream,
        result: {
          ...stream.result,
          headEventId: "evt_new",
          snapshot: { items: [], runs: [], interrupts: [] },
        },
      };
    });
    const reattach = createRunStreamReattach({
      sessionId: "ses_1",
      client: () => runClient(subscribe),
      isCancelled: () => false,
      recoverProjection,
    });
    const signal = new AbortController().signal;
    const result = await reattach(position("cold"), signal);
    expect(result?.cursor).toBe("evt_new");
    expect(useAgentStore.getState().sessions.ses_1!.view.commandError).toBeNull();
    expect(recoverProjection).not.toHaveBeenCalled();
    expect(subscribe).toHaveBeenCalledWith(
      { runId: RUN, segmentId: SEGMENT, snapshot: true },
      signal,
    );
  });

  it("replays from the last folded event while the cursor remains trustworthy", async () => {
    const recoverProjection = vi.fn(async (_signal: AbortSignal) => {});
    const subscribe = vi.fn<FlameClient["runs"]["subscribe"]>(async () => emptyStream());
    const reattach = createRunStreamReattach({
      sessionId: "ses_1",
      client: () => runClient(subscribe),
      isCancelled: () => false,
      recoverProjection,
    });
    const signal = new AbortController().signal;

    await expect(reattach(position("replay"), signal)).resolves.not.toBeNull();

    expect(recoverProjection).not.toHaveBeenCalled();
    expect(subscribe).toHaveBeenCalledWith({ runId: RUN, segmentId: SEGMENT }, signal, {
      lastEventId: "evt_7",
    });
  });

  it("rebuilds durable state when the addressed run finishes before replay attach", async () => {
    const recoverProjection = vi.fn(async (_signal: AbortSignal) => {});
    const subscribe = vi.fn<FlameClient["runs"]["subscribe"]>().mockRejectedValue(
      new RpcError({
        code: -32002,
        message: "run finished",
        data: { type: "run_finished" },
      }),
    );
    const reattach = createRunStreamReattach({
      sessionId: "ses_1",
      client: () => runClient(subscribe),
      isCancelled: () => false,
      recoverProjection,
    });

    await expect(reattach(position("replay"), new AbortController().signal)).resolves.toBeNull();

    expect(recoverProjection).toHaveBeenCalledTimes(1);
  });

  it("does not warn when a cold projection read proves the run already moved", async () => {
    const warning = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const recoverProjection = vi.fn(async (_signal: AbortSignal) => {});
    const subscribe = vi.fn<FlameClient["runs"]["subscribe"]>().mockRejectedValue(
      new RpcError({
        code: -32002,
        message: "run waiting",
        data: { type: "run_waiting" },
      }),
    );
    const reattach = createRunStreamReattach({
      sessionId: "ses_1",
      client: () => runClient(subscribe),
      isCancelled: () => false,
      recoverProjection,
    });

    await expect(reattach(position("cold"), new AbortController().signal)).resolves.toBeNull();

    expect(recoverProjection).toHaveBeenCalledTimes(1);
    expect(warning).not.toHaveBeenCalled();
  });

  it("does not diagnose a disappeared Runtime as a reattach failure", async () => {
    const warning = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const recoverProjection = vi.fn(async (_signal: AbortSignal) => {});
    const subscribe = vi
      .fn<FlameClient["runs"]["subscribe"]>()
      .mockRejectedValue(new RpcConnectionError("fetch failed"));
    const reattach = createRunStreamReattach({
      sessionId: "ses_1",
      client: () => runClient(subscribe),
      isCancelled: () => false,
      recoverProjection,
    });

    await expect(reattach(position("replay"), new AbortController().signal)).resolves.toBeNull();

    expect(recoverProjection).not.toHaveBeenCalled();
    expect(warning).not.toHaveBeenCalled();
  });
});
