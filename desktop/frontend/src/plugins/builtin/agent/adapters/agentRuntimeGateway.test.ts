import { afterEach, describe, expect, it, vi } from "vitest";
import {
  RpcError,
  RpcTransportError,
  MUTATION_ATTEMPT_TIMEOUT_MS,
  type FlameClient,
  type Methods,
  type MutationPromise,
  MutationSettlementClosedError,
} from "@flame/runtime-contract/client";
import { asRunId, asSegmentId, asSessionId } from "@flame/runtime-contract/client";
import { createMutationPromise } from "@flame/runtime-contract/client/mutation";
import * as runtimeCapabilities from "@/plugins/builtin/runtime/public/capabilities";
import { agentRuntime } from "../application/ports/runtimeGateway";
import { installAgentRuntimeGateway } from "./agentRuntimeGateway";
import { registerAgentSessionSharedMaterial } from "../application/ports/sessionSharedMaterial";

let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};
const getRuntimeClient = () => runtimeClient();

let uninstall: ReturnType<typeof installAgentRuntimeGateway> | undefined;
let uninstallMaterialCommitter: (() => void) | undefined;

afterEach(() => {
  uninstall?.dispose();
  uninstall = undefined;
  uninstallMaterialCommitter?.();
  uninstallMaterialCommitter = undefined;

  vi.restoreAllMocks();
  vi.useRealTimers();
});

function createdSession(id: string) {
  return {
    id: asSessionId(id),
    revision: 1,
    title: "",
    status: "idle" as const,
    provider: "openai",
    model: "gpt-5",
    workspace: { ref: { path: "/repo" }, availability: "available" as const },
    createdAt: "2026-08-20T00:00:00Z",
    updatedAt: "2026-08-20T00:00:00Z",
  };
}

describe("agentRuntimeGateway", () => {
  it("does not hand a retained create promise to a replacement adapter generation", async () => {
    const transportFailure = new RpcTransportError("retired response was lost");
    const retiredRetry = vi.fn(
      () =>
        Object.assign(Promise.resolve(createdSession("ses_retired")), {
          idempotencyKey: "retired-create",
          retry: vi.fn(),
        }) as MutationPromise<{ id: ReturnType<typeof asSessionId> }>,
    );
    const retiredCreate = vi.fn(
      () =>
        Object.assign(Promise.reject(transportFailure), {
          idempotencyKey: "retired-create",
          retry: retiredRetry,
        }) as MutationPromise<{ id: ReturnType<typeof asSessionId> }>,
    );
    runtimeClient = () => ({ sessions: { create: retiredCreate } }) as unknown as FlameClient;
    uninstall = installAgentRuntimeGateway(getRuntimeClient);

    await expect(agentRuntime().createSession({ cwd: "/repo" })).rejects.toBe(transportFailure);
    uninstall.dispose();
    uninstall = undefined;

    const successorCreate = vi.fn(
      () =>
        Object.assign(Promise.resolve(createdSession("ses_successor")), {
          idempotencyKey: "successor-create",
          retry: vi.fn(),
        }) as MutationPromise<{ id: ReturnType<typeof asSessionId> }>,
    );
    runtimeClient = () => ({ sessions: { create: successorCreate } }) as unknown as FlameClient;
    uninstall = installAgentRuntimeGateway(getRuntimeClient);

    await expect(agentRuntime().createSession({ cwd: "/repo" })).resolves.toMatchObject({
      id: "ses_successor",
      workspace: { path: "/repo" },
    });
    expect(successorCreate).toHaveBeenCalledOnce();
    expect(retiredRetry).not.toHaveBeenCalled();
  });

  it("refuses a create response that settles after its adapter generation is disposed", async () => {
    let settleRetired!: (value: { id: ReturnType<typeof asSessionId> }) => void;
    const retired = new Promise<{ id: ReturnType<typeof asSessionId> }>((resolve) => {
      settleRetired = resolve;
    });
    let attemptSignal: AbortSignal | undefined;
    const create = vi.fn((_params, signal?: AbortSignal) => {
      attemptSignal = signal;
      return Object.assign(retired, {
        idempotencyKey: "retired-create",
        retry: vi.fn(),
      }) as MutationPromise<{ id: ReturnType<typeof asSessionId> }>;
    });
    runtimeClient = () => ({ sessions: { create } }) as unknown as FlameClient;
    uninstall = installAgentRuntimeGateway(getRuntimeClient);

    const creating = agentRuntime().createSession({ cwd: "/repo" });
    uninstall.dispose();
    uninstall = undefined;

    await expect(creating).rejects.toBeInstanceOf(MutationSettlementClosedError);
    expect(attemptSignal?.aborted).toBe(true);

    settleRetired({ id: asSessionId("ses_retired") });
    await retired;
  });

  it("replays a timed-out create with the same mutation identity and a fresh signal", async () => {
    vi.useFakeTimers();
    const keys: string[] = [];
    const signals: AbortSignal[] = [];
    let executions = 0;
    const create = vi.fn((_params, signal?: AbortSignal) =>
      createMutationPromise(
        async (key, attempt) => {
          keys.push(key);
          signals.push(attempt.signal!);
          executions += 1;
          if (executions === 2) return createdSession("ses_replayed");
          await new Promise<void>((_resolve, reject) => {
            attempt.signal?.addEventListener(
              "abort",
              () => reject(new RpcTransportError("attempt timed out")),
              { once: true },
            );
          });
          throw new Error("unreachable");
        },
        "logical-create",
        { signal },
      ),
    );
    runtimeClient = () => ({ sessions: { create } }) as unknown as FlameClient;
    uninstall = installAgentRuntimeGateway(getRuntimeClient);

    const creating = agentRuntime().createSession({ cwd: "/repo" });
    await vi.advanceTimersByTimeAsync(0);
    expect(executions).toBe(1);
    await vi.advanceTimersByTimeAsync(MUTATION_ATTEMPT_TIMEOUT_MS);

    await expect(creating).resolves.toMatchObject({ id: "ses_replayed" });
    expect(create).toHaveBeenCalledOnce();
    expect(keys).toEqual(["logical-create", "logical-create"]);
    expect(signals).toHaveLength(2);
    expect(signals[0]).not.toBe(signals[1]);
    expect(signals[0]?.aborted).toBe(true);
    expect(signals[1]?.aborted).toBe(false);
  });

  it("forwards the caller snapshot revision and returns the summary the Runtime saved", async () => {
    const get = vi.fn();
    const update = vi.fn().mockResolvedValue({
      id: "ses_1",
      revision: 12,
      title: "saved title",
      status: "idle",
      provider: "openai",
      model: "gpt-5",
      workspace: { ref: { path: "/repo" }, availability: "available" },
      favorite: true,
      createdAt: "2026-08-12T00:00:00Z",
      updatedAt: "2026-08-13T00:00:00Z",
    });
    runtimeClient = () => ({ sessions: { get, update } }) as unknown as FlameClient;
    uninstall = installAgentRuntimeGateway(getRuntimeClient);

    await expect(
      agentRuntime().updateSession({
        sessionId: "ses_1",
        expectedRevision: 11,
        favorite: true,
      }),
    ).resolves.toEqual({
      id: "ses_1",
      revision: 12,
      title: "saved title",
      status: "idle",
      provider: "openai",
      model: "gpt-5",
      workspace: { path: "/repo", availability: "available" },
      favorite: true,
      time: "2026-08-13T00:00:00Z",
    });

    expect(update).toHaveBeenCalledWith({
      sessionId: asSessionId("ses_1"),
      expectedRevision: 11,
      favorite: true,
    } satisfies Parameters<Methods["sessions"]["update"]>[0]);
    expect(get).not.toHaveBeenCalled();
  });

  it("projects the approval mode saved by the Runtime", async () => {
    const saved = {
      mode: "safe",
      modes: [{ mode: "safe", write: "prompt", exec: "prompt", network: "prompt" }],
    };
    const setMode = vi.fn().mockResolvedValue(saved);
    runtimeClient = () => ({ approval: { setMode } }) as unknown as FlameClient;
    uninstall = installAgentRuntimeGateway(getRuntimeClient);

    await expect(agentRuntime().setApprovalMode("safe")).resolves.toEqual(saved);
    expect(setMode).toHaveBeenCalledWith("safe");
  });

  it("translates structured steering input only at the runtime adapter", async () => {
    const steer = vi.fn().mockResolvedValue({ userItemId: "item_steer" });
    runtimeClient = () => ({ runs: { steer } }) as unknown as FlameClient;
    uninstall = installAgentRuntimeGateway(getRuntimeClient);

    const result = await agentRuntime().steerRun("run_1", "seg_1", {
      parts: [
        { kind: "text", text: "compare this" },
        { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
      ],
    });

    expect(result).toEqual({ userItemId: "item_steer" });
    expect(steer).toHaveBeenCalledWith(asRunId("run_1"), asSegmentId("seg_1"), [
      { type: "text", text: "compare this" },
      { type: "image", mime: "image/png", data: "aW1hZ2U=" },
    ] satisfies Parameters<Methods["runs"]["steer"]>[2]);
  });

  it.each([{ supported: false }, { supported: true }])(
    "reads one coherent snapshot with descendants supported=$supported",
    async ({ supported }) => {
      vi.spyOn(runtimeCapabilities, "runtimeCapability").mockReturnValue(supported);
      const stageMaterial = vi.fn(
        (_sessionId: string, material: { goal?: { objective: string } }) =>
          material.goal?.objective,
      );
      uninstallMaterialCommitter = registerAgentSessionSharedMaterial(
        "test.goal-objective",
        stageMaterial,
      );
      const readSnapshot = vi.fn().mockResolvedValue({
        items: [],
        runs: [],
        interrupts: [],
        plan: {
          sessionId: "ses_1",
          state: {
            revision: 4,
            steps: [{ id: "step_1", description: "Verify boundaries", status: "in_progress" }],
            updatedAt: "2026-08-17T00:00:00Z",
          },
        },
        goal: {
          sessionId: "ses_1",
          objective: "Recover every mounted read",
          status: "active",
          used: { runs: 1, costUsd: 0.25, steps: 2 },
          createdAt: "2026-08-17T00:00:00Z",
          updatedAt: "2026-08-17T00:01:00Z",
        },
      });
      runtimeClient = () =>
        ({
          sessions: { snapshot: readSnapshot },
        }) as unknown as FlameClient;
      uninstall = installAgentRuntimeGateway(getRuntimeClient);

      const snapshot = await agentRuntime().loadSessionSnapshot("ses_1");

      expect(readSnapshot).toHaveBeenCalledWith(asSessionId("ses_1"), supported, undefined);
      expect(snapshot?.snapshot.plan).toEqual({
        revision: 4,
        steps: [{ id: "step_1", text: "Verify boundaries", status: "active" }],
      });
      expect(stageMaterial).toHaveBeenCalledWith(
        "ses_1",
        expect.objectContaining({
          goal: expect.objectContaining({ objective: "Recover every mounted read" }),
        }),
      );
      expect(snapshot?.projectAssociatedSharedMaterial({ plan: "kept" })).toEqual({
        plan: "kept",
        "test.goal-objective": "Recover every mounted read",
      });
    },
  );

  it("translates an authoritatively missing session into an absent snapshot", async () => {
    const missing = new RpcError({
      code: -32002,
      message: "session missing",
      data: { type: "session_not_found" },
    });
    runtimeClient = () =>
      ({
        sessions: { snapshot: vi.fn().mockRejectedValue(missing) },
      }) as unknown as FlameClient;
    uninstall = installAgentRuntimeGateway(getRuntimeClient);

    await expect(agentRuntime().loadSessionSnapshot("ses_gone")).resolves.toBeNull();
  });

  it("treats an already missing Session as a completed delete", async () => {
    const missing = new RpcError({
      code: -32002,
      message: "session missing",
      data: { type: "session_not_found" },
    });
    runtimeClient = () =>
      ({ sessions: { delete: vi.fn().mockRejectedValue(missing) } }) as unknown as FlameClient;
    uninstall = installAgentRuntimeGateway(getRuntimeClient);

    await expect(agentRuntime().deleteSession("ses_gone")).resolves.toBeUndefined();
  });

  it("projects rollback dropped input without leaking wire blocks into Application", async () => {
    const rollback = vi.fn().mockResolvedValue({
      session: {},
      droppedRuns: [
        {
          run: { id: "run_dropped", sessionId: "ses_1" },
          userInput: [
            { type: "text", text: "retry this" },
            { type: "image", mime: "image/png", data: "aW1hZ2U=" },
          ],
        },
      ],
    });
    runtimeClient = () => ({ sessions: { rollback } }) as unknown as FlameClient;
    uninstall = installAgentRuntimeGateway(getRuntimeClient);

    await expect(
      agentRuntime().rollbackSession({
        sessionId: "ses_1",
        toRunId: "run_keep",
        restoreType: "both",
      }),
    ).resolves.toEqual({
      droppedRuns: [
        {
          runId: "run_dropped",
          userInput: {
            parts: [
              { kind: "text", text: "retry this" },
              { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
            ],
          },
        },
      ],
    });
    expect(rollback).toHaveBeenCalledWith({
      sessionId: asSessionId("ses_1"),
      toRunId: asRunId("run_keep"),
      restoreType: "both",
    });
  });
});
