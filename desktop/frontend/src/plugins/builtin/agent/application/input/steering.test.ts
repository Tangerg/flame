import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  RpcError,
  RpcTransportError,
  type FlameClient,
  type RunRef,
} from "@flame/runtime-contract/client";
import { createMutationPromise } from "@flame/runtime-contract/client/mutation";
import { installAgentRuntimeGateway } from "../../adapters/agentRuntimeGateway";
import { useAgentStore } from "../../adapters/agentStore";
import { runtimeRunFact } from "../../adapters/runtimeAgentFacts";
import { agentSessionView } from "../ports/sessionView";
import { revalidateAgentSessionMaterial } from "../session/refreshSessionProjection";
import { retrySteerInput, steerRunningTurn } from "./steering";

const sessionId = "ses_steer";
const runId = "run_original";
const segmentId = "seg_original";
const root: RunRef = {
  id: runId,
  sessionId,
  status: "running",
  activeSegmentId: segmentId,
  provider: "openai",
  model: "gpt-5",
  createdAt: "2026-10-14T00:00:00Z",
  metrics: { steps: 0, activeDurationMillis: 0 },
  protocolProfile: { interruptTypes: [], requiredFeatures: [] },
};

let gateway: ReturnType<typeof installAgentRuntimeGateway>;

beforeEach(() => {
  useAgentStore.setState({ sessions: {} });
  useAgentStore.getState().ensureSession(sessionId);
  useAgentStore.getState().applyRunSnapshot(sessionId, root);
  useAgentStore.getState().setSynchronize(sessionId, async () => false);
});

afterEach(() => {
  gateway?.dispose();
  vi.restoreAllMocks();
});

const messages = () => agentSessionView().getSession(sessionId)!.view.messages;

describe("steer command observation", () => {
  it("retains a lost ACK and retries the original command after a cold read and Segment change", async () => {
    const accepted = new Map<string, string>();
    let admissions = 0;
    const execute = vi.fn(async (key: string) => {
      if (!accepted.has(key)) {
        accepted.set(key, "item_original");
        admissions++;
      }
      if (execute.mock.calls.length <= 2) throw new RpcTransportError("original ACK lost");
      return { userItemId: accepted.get(key)! };
    });
    const steer = vi.fn(() => createMutationPromise(execute, "original-command"));
    gateway = installAgentRuntimeGateway(() => ({ runs: { steer } }) as unknown as FlameClient);
    const input = { parts: [{ kind: "text" as const, text: "original instruction" }] };
    expect(steerRunningTurn({ sessionId, runId, segmentId, input })).toBe(true);
    input.parts[0]!.text = "a changed draft";
    await vi.waitFor(() => expect(messages()[0]?.steer?.receipt).toBe("unknown"));
    const localId = messages()[0]!.id;

    await revalidateAgentSessionMaterial(sessionId, async () => ({
      snapshot: {
        runs: [runtimeRunFact({ ...root, activeSegmentId: "seg_next" })],
        items: [],
        pendingInterruptSets: [],
      },
      projectAssociatedSharedMaterial: (shared) => shared,
    }));
    expect(messages()).toHaveLength(1);
    expect(messages()[0]).toMatchObject({
      id: localId,
      runId: null,
      steer: { runId, receipt: "unknown" },
    });
    expect(messages()[0]!.blocks).toMatchObject([{ kind: "text", text: "original instruction" }]);

    expect(retrySteerInput(localId)).toBe(true);
    expect(retrySteerInput(localId)).toBe(false);
    await vi.waitFor(() => expect(messages()[0]?.id).toBe("item_original"));
    expect(messages()[0]?.steer).toEqual({ runId, receipt: "accepted" });
    expect(admissions).toBe(1);
    expect(execute.mock.calls.map(([key]) => key)).toEqual([
      "original-command",
      "original-command",
      "original-command",
    ]);
    expect(steer).toHaveBeenCalledExactlyOnceWith(runId, segmentId, [
      { type: "text", text: "original instruction" },
    ]);
  });

  it("merges an Item that arrives before the original receipt without matching content", async () => {
    const receipt = Promise.withResolvers<{ userItemId: string }>();
    const steer = vi.fn(() => createMutationPromise(() => receipt.promise, "item-first"));
    gateway = installAgentRuntimeGateway(() => ({ runs: { steer } }) as unknown as FlameClient);
    steerRunningTurn({
      sessionId,
      runId,
      segmentId,
      input: { parts: [{ kind: "text", text: "same text" }] },
    });
    useAgentStore.getState().applyRunEvents(sessionId, [
      {
        eventId: "evt_original",
        runId,
        segmentId,
        timestamp: "2026-10-14T00:00:01Z",
        event: {
          type: "item.completed",
          item: {
            id: "item_original",
            runId,
            status: "completed",
            type: "userMessage",
            createdAt: "2026-10-14T00:00:01Z",
            content: [{ type: "text", text: "same text" }],
          },
        },
      },
    ]);
    expect(messages()).toHaveLength(2);
    receipt.resolve({ userItemId: "item_original" });
    await vi.waitFor(() => expect(messages()).toHaveLength(1));
    expect(messages()[0]).toMatchObject({
      id: "item_original",
      runId,
      steer: { receipt: "accepted" },
    });
  });

  it("removes provisional input only after a definitive Runtime refusal", async () => {
    const refusal = new RpcError({ message: "stale segment", data: { type: "stale_segment" } });
    const steer = vi.fn(() =>
      createMutationPromise(async () => {
        throw refusal;
      }, "refused"),
    );
    gateway = installAgentRuntimeGateway(() => ({ runs: { steer } }) as unknown as FlameClient);
    steerRunningTurn({
      sessionId,
      runId,
      segmentId,
      input: { parts: [{ kind: "text", text: "refused input" }] },
    });
    await vi.waitFor(() => expect(messages()).toHaveLength(0));
    expect(steer).toHaveBeenCalledOnce();
  });

  it("retires original observation and does not let a late receipt change the successor view", async () => {
    const receipt = Promise.withResolvers<{ userItemId: string }>();
    const steer = vi.fn(() => createMutationPromise(() => receipt.promise, "retired"));
    gateway = installAgentRuntimeGateway(() => ({ runs: { steer } }) as unknown as FlameClient);
    steerRunningTurn({
      sessionId,
      runId,
      segmentId,
      input: { parts: [{ kind: "text", text: "retired input" }] },
    });
    const localId = messages()[0]!.id;
    gateway.replaceRuntimeGeneration();
    expect(messages()).toHaveLength(0);
    expect(retrySteerInput(localId)).toBe(false);
    receipt.resolve({ userItemId: "item_retired" });
    await receipt.promise;
    await Promise.resolve();
    expect(messages()).toHaveLength(0);
  });
});
