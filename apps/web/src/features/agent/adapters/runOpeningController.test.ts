import { afterEach, describe, expect, it, vi } from "vitest";
import {
  RpcError,
  RpcTransportError,
  asRunId,
  asSegmentId,
  type RunEvent,
  type RunOpeningResult,
} from "@flame/runtime-client";
import { type AgentProblem } from "@/features/agent/public/sessionView";
import { createRunOpeningController } from "@/features/agent/adapters/runOpeningController";
afterEach(() => {
  vi.restoreAllMocks();
});

describe("run opening controller", () => {
  it("returns the real acceptance while the existing owner continues observing the Run", async () => {
    const observed = Promise.withResolvers<void>();
    const controller = createRunOpeningController({
      sessionId: "ses_1",
      isCancelled: () => false,
      markInteracted: vi.fn(),
      setAbortController: vi.fn(),
      abortCurrent: vi.fn(),
      setStartError: vi.fn(),
      onRejected: vi.fn(),
    });
    const opening = Promise.withResolvers<
      RunOpeningResult<{
        runId: ReturnType<typeof asRunId>;
        segmentId: ReturnType<typeof asSegmentId>;
      }>
    >();
    const observe = vi.fn(() => observed.promise);
    const receipt = controller.begin(() => opening.promise, { observe });
    const received = vi.fn();
    void receipt.then(received);
    await Promise.resolve();
    expect(received).not.toHaveBeenCalled();
    opening.resolve({
      result: { runId: asRunId("run_1"), segmentId: asSegmentId("seg_1") },
      observation: { type: "stream", events: emptyEvents() },
    });
    await expect(receipt).resolves.toMatchObject({
      result: { runId: "run_1", segmentId: "seg_1" },
    });
    expect(observe).toHaveBeenCalledOnce();
    expect(controller.isStarting()).toBe(true);
    observed.resolve();
    await vi.waitFor(() => expect(controller.isStarting()).toBe(false));
    controller.retire();
  });

  it("retires an opening that ignores cancellation and disposes its late stream", async () => {
    const opening = Promise.withResolvers<
      RunOpeningResult<{
        runId: ReturnType<typeof asRunId>;
        segmentId: ReturnType<typeof asSegmentId>;
      }>
    >();
    const abortCurrent = vi.fn();
    const close = vi.fn(async () => ({ value: undefined, done: true }) as const);
    const onResult = vi.fn();
    const pump = vi.fn();
    const controller = createRunOpeningController({
      sessionId: "ses_1",
      isCancelled: () => false,
      markInteracted: vi.fn(),
      setAbortController: vi.fn(),
      abortCurrent,
      onRejected: vi.fn(),
      setStartError: vi.fn(),
    });

    controller.begin(() => opening.promise, { onResult, observe: pump });
    expect(controller.isStarting()).toBe(true);
    controller.retire();

    expect(controller.isStarting()).toBe(false);
    expect(abortCurrent).toHaveBeenCalledTimes(2);

    opening.resolve({
      result: { runId: asRunId("run_late"), segmentId: asSegmentId("seg_late") },
      observation: {
        type: "stream",
        events: {
          [Symbol.asyncIterator]: () => ({
            next: async () => ({ value: undefined, done: true }) as const,
            return: close,
          }),
        },
      },
    });

    await vi.waitFor(() => expect(close).toHaveBeenCalledOnce());
    expect(onResult).not.toHaveBeenCalled();
    expect(pump).not.toHaveBeenCalled();
  });

  it("reports a rejected opening even when its draft callback returns true", async () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const setStartError = vi.fn<(problem: AgentProblem) => void>();
    const onStartError = vi.fn(() => true);
    const rejected = new RpcError({
      code: -32002,
      message: "interrupt already consumed",
      data: { type: "interrupt_not_open" },
    });
    const controller = createRunOpeningController({
      sessionId: "ses_1",
      isCancelled: () => false,
      markInteracted: vi.fn(),
      setAbortController: vi.fn(),
      abortCurrent: vi.fn(),
      setStartError,
      onRejected: vi.fn(),
    });

    controller.begin(() => Promise.reject(rejected), { observe: vi.fn(), onStartError });

    await vi.waitFor(() => expect(onStartError).toHaveBeenCalledOnce());
    expect(error).toHaveBeenCalled();
    expect(setStartError).toHaveBeenCalled();
  });

  it("keeps a post-accept stream failure out of the opening failure channel", async () => {
    const warning = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const setStartError = vi.fn<(problem: AgentProblem) => void>();
    const onResult = vi.fn();
    const onStartError = vi.fn();
    const streamFailure = new RpcError({
      code: -32002,
      message: "run finished",
      data: { type: "run_finished", detail: "the accepted run moved before reattach" },
    });
    const controller = createRunOpeningController({
      sessionId: "ses_1",
      isCancelled: () => false,
      markInteracted: vi.fn(),
      setAbortController: vi.fn(),
      abortCurrent: vi.fn(),
      setStartError,
      onRejected: vi.fn(),
    });

    controller.begin(
      async () => ({
        result: { runId: asRunId("run_1"), segmentId: asSegmentId("seg_1") },
        observation: { type: "stream", events: emptyEvents() },
      }),
      {
        onResult,
        onStartError,
        observe: async () => {
          throw streamFailure;
        },
      },
    );

    await vi.waitFor(() => expect(onResult).toHaveBeenCalledTimes(1));
    await vi.waitFor(() =>
      expect(warning).toHaveBeenCalledWith(
        "[agent] accepted run stream failed:",
        "ses_1",
        streamFailure,
      ),
    );
    expect(setStartError).not.toHaveBeenCalled();
    expect(onStartError).not.toHaveBeenCalled();
  });

  it("queries the original command after an uncertain opening instead of offering a new command", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const setStartError = vi.fn<(problem: AgentProblem) => void>();
    const onRejected = vi.fn();
    const controller = createRunOpeningController({
      sessionId: "ses_1",
      isCancelled: () => false,
      markInteracted: vi.fn(),
      setAbortController: vi.fn(),
      abortCurrent: vi.fn(),
      setStartError,
      onRejected,
    });

    controller.begin(() => Promise.reject(new RpcTransportError("connection unavailable")), {
      observe: vi.fn(),
    });

    await vi.waitFor(() => expect(onRejected).toHaveBeenCalledOnce());
    expect(setStartError).not.toHaveBeenCalled();
  });
});

async function* emptyEvents(): AsyncIterable<RunEvent> {}
