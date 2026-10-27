import { beforeEach, describe, expect, it, onTestFinished, vi } from "vitest";
import { RpcTransportError, type RunRef } from "@flame/runtime-client";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { runtimeRunFact } from "@/features/agent/adapters/runtimeAgentFacts";
import { agentTextInput } from "@/features/agent/domain/input";
import { type AgentInputSettlement } from "./inputSubmission";
import {
  type AgentSessionMaterialRead,
  type AgentSteerSubmission,
} from "@/features/agent/application/runtimeGateway";
import { submitAgentInput, submitCapturedAgentInput } from "./submitAgentInput";

const sessionId = "ses_original";
const accepted = {
  status: "accepted" as const,
  sessionId,
  runId: "run_new",
  userItemId: "item_new",
};
let client: TestClient;
beforeEach(() => {
  client = createTestClient();
  client.agent.projection.store.getState().ensureSession(sessionId);
  onTestFinished(() => {
    vi.restoreAllMocks();
    client.dispose();
  });
});

function material(overrides: Partial<AgentSessionMaterialRead> = {}): AgentSessionMaterialRead {
  return {
    snapshot: { items: [], runs: [], pendingInterruptSets: [] },
    projectAssociatedSharedMaterial: (shared) => shared,
    ...overrides,
  };
}

function root(status: "running" | "waiting"): RunRef {
  return {
    id: "run_current",
    sessionId,
    status,
    ...(status === "running" ? { activeSegmentId: "seg_current" } : {}),
    provider: "provider",
    model: "model",
    createdAt: "2026-10-27T00:00:00Z",
    metrics: { steps: 0, activeDurationMillis: 0 },
    protocolProfile: { interruptTypes: [], requiredFeatures: [] },
  };
}

function allowSend() {
  const send = vi.fn(async (): Promise<AgentInputSettlement> => accepted);
  client.agent.projection.store.getState().setSend(sessionId, send);
  return send;
}

describe("shared Agent input owner", () => {
  it("uses the captured Session rather than current navigation and awaits its actual acceptance", async () => {
    const pending = Promise.withResolvers<AgentInputSettlement>();
    const send = vi.fn(() => pending.promise);
    client.agent.projection.store.getState().setSend(sessionId, send);
    client.navigation.go({ session: "ses_other" });
    vi.spyOn(client.agent.dependencies.runtime, "loadSessionSnapshot").mockResolvedValue(
      material(),
    );
    const input = agentTextInput("captured input");
    const submission = submitCapturedAgentInput(
      client.agent.dependencies,
      sessionId,
      input,
      new AbortController().signal,
    );
    const observed = vi.fn();
    void submission.then(observed);
    await vi.waitFor(() => expect(send).toHaveBeenCalledWith(input, {}));
    expect(observed).not.toHaveBeenCalled();
    pending.resolve(accepted);
    await expect(submission).resolves.toEqual(accepted);
  });

  it("steers the existing current Segment and returns the original uncertain identity", async () => {
    const send = allowSend();
    client.agent.projection.store.getState().applyRunSnapshot(sessionId, root("running"));
    const recovered: AgentSteerSubmission = Object.assign(
      Promise.resolve({ type: "accepted" as const, userItemId: "item_steer" }),
      {
        idempotencyKey: "original-steer",
        inputIdentity: Promise.resolve("item_steer"),
        retry: vi.fn(),
      },
    );
    const retry = vi.fn(() => recovered);
    const steer = vi.spyOn(client.agent.dependencies.runtime, "steerRun").mockImplementation(() =>
      Object.assign(Promise.reject(new RpcTransportError("ack lost")), {
        idempotencyKey: "original-steer",
        inputIdentity: Promise.resolve("item_steer"),
        retry,
      }),
    );
    const result = await submitAgentInput(
      client.agent.dependencies,
      sessionId,
      agentTextInput("continue"),
    );
    expect(steer).toHaveBeenCalledWith("run_current", "seg_current", agentTextInput("continue"));
    expect(send).not.toHaveBeenCalled();
    expect(result).toMatchObject({ status: "unknown", idempotencyKey: "original-steer" });
    if (result?.status !== "unknown") throw new Error("expected unknown input");
    await expect(result.recover()).resolves.toEqual({
      status: "accepted",
      sessionId,
      runId: "run_current",
      userItemId: "item_steer",
    });
    expect(retry).toHaveBeenCalledOnce();
    expect(steer).toHaveBeenCalledOnce();
  });

  it("does not start or steer while the current root is waiting", () => {
    const send = allowSend();
    const steer = vi.spyOn(client.agent.dependencies.runtime, "steerRun");
    client.agent.projection.store.getState().applyRunSnapshot(sessionId, root("waiting"));
    expect(
      submitAgentInput(client.agent.dependencies, sessionId, agentTextInput("wait")),
    ).toBeNull();
    expect(send).not.toHaveBeenCalled();
    expect(steer).not.toHaveBeenCalled();
  });

  it("recovers a reconstructed component's original opening instead of submitting its input again", async () => {
    const send = allowSend();
    const input = agentTextInput("captured input");
    const retry = vi.fn(async () => accepted);
    client.agent.projection.store.getState().setRetryOpening(sessionId, retry);
    vi.spyOn(client.agent.dependencies.runtime, "loadSessionSnapshot").mockResolvedValue(
      material({
        unresolvedOpenings: [
          {
            operation: "start",
            idempotencyKey: "original-start",
            receipt: "unknown",
            input,
            options: {},
          },
        ],
      }),
    );
    const result = await submitCapturedAgentInput(
      client.agent.dependencies,
      sessionId,
      input,
      new AbortController().signal,
    );
    expect(result).toMatchObject({ status: "unknown", idempotencyKey: "original-start" });
    expect(send).not.toHaveBeenCalled();
    expect(retry).not.toHaveBeenCalled();
    if (result.status !== "unknown") throw new Error("expected original opening");
    await expect(result.recover()).resolves.toEqual(accepted);
    expect(retry).toHaveBeenCalledWith("original-start");
  });

  it("does not replace an unresolved input with a changed form", async () => {
    const send = allowSend();
    vi.spyOn(client.agent.dependencies.runtime, "loadSessionSnapshot").mockResolvedValue(
      material({
        unresolvedOpenings: [
          {
            operation: "start",
            idempotencyKey: "original-start",
            receipt: "unknown",
            input: agentTextInput("original"),
            options: {},
          },
        ],
      }),
    );
    await expect(
      submitCapturedAgentInput(
        client.agent.dependencies,
        sessionId,
        agentTextInput("changed"),
        new AbortController().signal,
      ),
    ).rejects.toThrow(/unresolved input/);
    expect(send).not.toHaveBeenCalled();
  });

  it.each(["accepted", "retired"] as const)(
    "projects a %s receipt found during preflight without opening another input",
    async (status) => {
      const send = allowSend();
      const input = agentTextInput("original");
      vi.spyOn(client.agent.dependencies.runtime, "loadSessionSnapshot").mockResolvedValue(
        material({
          recoveredInputs: [
            { idempotencyKey: "original-start", input, acceptance: { ...accepted, status } },
          ],
        }),
      );
      await expect(
        submitCapturedAgentInput(
          client.agent.dependencies,
          sessionId,
          input,
          new AbortController().signal,
        ),
      ).resolves.toEqual({ ...accepted, status });
      expect(send).not.toHaveBeenCalled();
    },
  );

  it("reuses a restored steer handle after the Run has advanced its Segment", async () => {
    const send = allowSend();
    const input = agentTextInput("original steer");
    const retry = vi.fn((): AgentSteerSubmission =>
      Object.assign(Promise.resolve({ type: "accepted" as const, userItemId: "item_original" }), {
        idempotencyKey: "original-steer",
        inputIdentity: Promise.resolve("item_original"),
        retry: vi.fn(),
      }),
    );
    const steer = vi.spyOn(client.agent.dependencies.runtime, "steerRun");
    vi.spyOn(client.agent.dependencies.runtime, "loadSessionSnapshot").mockResolvedValue(
      material({
        snapshot: { items: [], runs: [runtimeRunFact(root("running"))], pendingInterruptSets: [] },
        unresolvedSteers: [
          {
            idempotencyKey: "original-steer",
            runId: "run_current",
            userItemId: "item_original",
            input,
            retry,
          },
        ],
      }),
    );
    const result = await submitCapturedAgentInput(
      client.agent.dependencies,
      sessionId,
      input,
      new AbortController().signal,
    );
    if (result.status !== "unknown") throw new Error("expected original steer");
    await expect(result.recover()).resolves.toEqual({
      status: "accepted",
      sessionId,
      runId: "run_current",
      userItemId: "item_original",
    });
    expect(retry).toHaveBeenCalledOnce();
    expect(steer).not.toHaveBeenCalled();
    expect(send).not.toHaveBeenCalled();
  });
});
