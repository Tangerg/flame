import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  RpcError,
  RpcTransportError,
  type FlameClient,
  type RunRef,
  type SteerMutationPromise,
} from "@flame/runtime-contract/client";
import { createMutationPromise } from "@flame/runtime-contract/client/mutation";
import {
  createMutationJournal,
  type MutationJournalStorage,
} from "@flame/runtime-contract/client/mutationJournal";
import { createMethods } from "@flame/runtime-contract/client/methods";
import type { RpcClient } from "@flame/runtime-contract/client/client";
import { asRunId, asSegmentId } from "@flame/runtime-contract/client/ids";
import { useNotificationStore } from "@/plugins/sdk/notifications";
import { agentRuntime } from "../ports/runtimeGateway";
import { installAgentRuntimeGateway } from "../../adapters/agentRuntimeGateway";
import { useAgentStore } from "../../adapters/agentStore";
import { runtimeRunFact } from "../../adapters/runtimeAgentFacts";
import { agentSessionView } from "../ports/sessionView";
import { revalidateAgentSessionMaterial } from "../session/refreshSessionProjection";
import { steerInputFeedback } from "../view/steerInputFeedback";
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

function nominatedSubmission(
  execute: (key: string) => Promise<{ userItemId: string }>,
  key: string,
  userItemId = "item_original",
): SteerMutationPromise {
  const expose = (
    mutation: ReturnType<typeof createMutationPromise<{ userItemId: string }>>,
  ): SteerMutationPromise =>
    Object.defineProperties(
      mutation.then((result) => ({ type: "accepted" as const, ...result })),
      {
        idempotencyKey: { value: mutation.idempotencyKey },
        inputIdentity: { value: Promise.resolve(userItemId) },
        retry: { value: () => expose(mutation.retry()) },
      },
    ) as SteerMutationPromise;
  return expose(createMutationPromise(execute, key));
}

let gateway: ReturnType<typeof installAgentRuntimeGateway>;

beforeEach(() => {
  useAgentStore.setState({ sessions: {} });
  useNotificationStore.setState({ log: [] });
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
  it.each([true, false])(
    "restores accepted guidance and folds only its exact applied Item (applied=%s)",
    async (applied) => {
      const body = [{ type: "text" as const, text: "durable original guidance" }];
      const snapshot = vi.fn().mockResolvedValue({
        items: [],
        runs: [root],
        interrupts: [],
        steerInputs: [{ runId, segmentId, userItemId: "item_accepted", input: body }],
      });
      gateway = installAgentRuntimeGateway(
        () =>
          ({
            runs: { recoverCommands: async () => [] },
            sessions: { snapshot },
          }) as unknown as FlameClient,
      );
      await revalidateAgentSessionMaterial(sessionId, () =>
        agentRuntime().loadSessionSnapshot(sessionId),
      );
      expect(messages()[0]).toMatchObject({
        id: "item_accepted",
        runId: null,
        steer: { runId, receipt: "accepted" },
      });
      snapshot.mockResolvedValue({
        items: applied
          ? [
              {
                id: "item_accepted",
                runId,
                type: "userMessage",
                status: "completed",
                createdAt: root.createdAt,
                content: body,
              },
            ]
          : [],
        runs: [
          {
            ...root,
            status: "finished",
            activeSegmentId: undefined,
            finishedAt: root.createdAt,
            outcome: { type: "completed" },
          },
        ],
        interrupts: [],
        steerInputs: [{ runId, segmentId, userItemId: "item_accepted", input: body }],
      });
      await revalidateAgentSessionMaterial(sessionId, () =>
        agentRuntime().loadSessionSnapshot(sessionId),
      );
      expect(messages()).toHaveLength(1);
      expect(messages()[0]).toMatchObject({
        id: "item_accepted",
        runId: applied ? runId : null,
        steer: { runId, receipt: "accepted" },
      });
      expect(messages()[0]!.blocks).toMatchObject([
        { kind: "text", text: "durable original guidance" },
      ]);
      expect(steerInputFeedback(messages()[0]!)).toBe(applied ? "applied" : "notApplied");
      useAgentStore.setState({ sessions: {} });
      useAgentStore.getState().ensureSession(sessionId);
      await revalidateAgentSessionMaterial(sessionId, () =>
        agentRuntime().loadSessionSnapshot(sessionId),
      );
      expect(messages()[0]?.steer).toEqual({
        runId,
        receipt: "accepted",
        ...(!applied ? { application: "notApplied" } : {}),
      });
      expect(steerInputFeedback(messages()[0]!)).toBe(applied ? "applied" : "notApplied");
    },
  );
  it.each([false, true])(
    "restores the original frozen submission across application restart (query unavailable=%s)",
    async (unavailable) => {
      const values = new Map<string, unknown>();
      const storage: MutationJournalStorage = {
        get: (key) => structuredClone(values.get(key)),
        set: (key, value) => void values.set(key, structuredClone(value)),
        remove: (key) => void values.delete(key),
        keys: () => [...values.keys()],
      };
      const scope = () => ({ namespace: "idp_cold_steer", retentionSeconds: 86400 });
      const lostCall = vi.fn(async (method: string) => {
        if (method === "runs.getAcceptance")
          return { type: "notRecorded", candidateUserItemId: "item_original" };
        throw new RpcTransportError("application died with an unresolved steer");
      });
      const predecessor = createMutationJournal({ storage, scope });
      const first = createMethods({ call: lostCall } as unknown as RpcClient, {
        mutationJournal: predecessor,
      });
      const input = [{ type: "text" as const, text: "the original authored input" }];
      const pending = first.runs.steer(asRunId(runId), asSegmentId(segmentId), input);
      await expect(pending).rejects.toBeInstanceOf(RpcTransportError);
      input[0]!.text = "a changed draft after restart";
      predecessor.dispose();
      useAgentStore.setState({ sessions: {} });
      useAgentStore.getState().ensureSession(sessionId);
      useAgentStore.getState().setSynchronize(sessionId, async () => false);
      const call = vi.fn(async (method: string) => {
        if (method === "runs.getAcceptance") {
          if (unavailable) throw new RpcTransportError("original query unavailable");
          return { type: "notRecorded", candidateUserItemId: "item_original" };
        }
        return { userItemId: "item_original" };
      });
      const successor = createMutationJournal({ storage, scope });
      const methods = createMethods({ call } as unknown as RpcClient, {
        mutationJournal: successor,
      });
      const snapshot = vi.fn().mockResolvedValue({
        items: [],
        runs: [{ ...root, activeSegmentId: "seg_replacement" }],
        interrupts: [],
      });
      gateway = installAgentRuntimeGateway(
        () => ({ runs: methods.runs, sessions: { snapshot } }) as unknown as FlameClient,
      );
      await revalidateAgentSessionMaterial(sessionId, () =>
        agentRuntime().loadSessionSnapshot(sessionId),
      );
      expect(call.mock.calls.map(([method]) => method)).toEqual(["runs.getAcceptance"]);
      expect(messages()).toHaveLength(1);
      const restored = messages()[0]!;
      expect(restored.id).toBe("item_original");
      expect(restored.steer).toEqual({ runId, delivery: "unknown" });
      expect(restored.blocks).toMatchObject([
        { kind: "text", text: "the original authored input" },
      ]);
      unavailable = false;
      expect(retrySteerInput(restored.id)).toBe(true);
      expect(retrySteerInput(restored.id)).toBe(false);
      await vi.waitFor(() => expect(messages()[0]?.steer?.receipt).toBe("accepted"));
      expect(call.mock.calls[2]).toEqual([
        "runs.steer",
        {
          runId,
          expectedSegmentId: segmentId,
          input: [{ type: "text", text: "the original authored input" }],
        },
        expect.objectContaining({
          idempotencyKey: pending.idempotencyKey,
          idempotencyNamespace: "idp_cold_steer",
        }),
      ]);
      expect(values.size).toBe(0);
      successor.dispose();
    },
  );
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
    const steer = vi.fn(() => nominatedSubmission(execute, "original-command"));
    gateway = installAgentRuntimeGateway(() => ({ runs: { steer } }) as unknown as FlameClient);
    const input = { parts: [{ kind: "text" as const, text: "original instruction" }] };
    expect(steerRunningTurn({ sessionId, runId, segmentId, input })).toBe(true);
    input.parts[0]!.text = "a changed draft";
    await vi.waitFor(() => expect(messages()[0]?.steer?.delivery).toBe("unknown"));
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
      steer: { runId, delivery: "unknown" },
    });
    expect(messages()[0]!.blocks).toMatchObject([{ kind: "text", text: "original instruction" }]);

    expect(retrySteerInput(localId)).toBe(true);
    expect(retrySteerInput(localId)).toBe(false);
    await vi.waitFor(() => expect(messages()[0]?.steer?.receipt).toBe("accepted"));
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

  it.each(["ACK", "Item"])(
    "renders one applied input live and after reopening when %s arrives first",
    async (first) => {
      const receipt = Promise.withResolvers<{ userItemId: string }>();
      const steer = vi.fn(() => nominatedSubmission(() => receipt.promise, "item-first"));
      const item = {
        id: "item_original",
        runId,
        status: "completed",
        type: "userMessage",
        createdAt: "2026-10-14T00:00:01Z",
        content: [{ type: "text", text: "same text" }],
      };
      const snapshot = vi.fn(async () => ({
        items: [item],
        runs: [root],
        interrupts: [],
        steerInputs: [{ runId, segmentId, userItemId: item.id, input: item.content }],
      }));
      gateway = installAgentRuntimeGateway(
        () =>
          ({
            runs: { steer, recoverCommands: async () => [] },
            sessions: { snapshot },
          }) as unknown as FlameClient,
      );
      steerRunningTurn({
        sessionId,
        runId,
        segmentId,
        input: { parts: [{ kind: "text", text: "same text" }] },
      });
      await vi.waitFor(() => expect(messages()[0]?.id).toBe("item_original"));
      expect(messages()[0]?.steer?.delivery).toBe("pending");
      expect(steerInputFeedback(messages()[0]!)).toBe("pending");
      if (first === "ACK") {
        receipt.resolve({ userItemId: "item_original" });
        await vi.waitUntil(() => messages()[0]?.steer?.receipt === "accepted");
      }
      expect(steerInputFeedback(messages()[0]!)).toBe(first === "ACK" ? "accepted" : "pending");
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
      expect(messages()).toHaveLength(1);
      expect(steerInputFeedback(messages()[0]!)).toBe("applied");
      receipt.resolve({ userItemId: "item_original" });
      await vi.waitFor(() => expect(messages()[0]?.steer?.receipt).toBe("accepted"));
      expect(messages()).toHaveLength(1);
      expect(messages()[0]).toMatchObject({
        id: "item_original",
        runId,
        steer: { receipt: "accepted" },
      });
      useAgentStore.setState({ sessions: {} });
      useAgentStore.getState().ensureSession(sessionId);
      await revalidateAgentSessionMaterial(sessionId, () =>
        agentRuntime().loadSessionSnapshot(sessionId),
      );
      expect(messages()).toHaveLength(1);
      expect(steerInputFeedback(messages()[0]!)).toBe("applied");
      expect(messages()[0]!.blocks).toMatchObject([{ kind: "text", text: "same text" }]);
    },
  );

  it("removes provisional input only after a definitive Runtime refusal", async () => {
    const refusal = new RpcError({ message: "stale segment", data: { type: "stale_segment" } });
    const steer = vi.fn(() =>
      nominatedSubmission(async () => {
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
    const steer = vi.fn(() =>
      nominatedSubmission(() => receipt.promise, "retired", "item_retired"),
    );
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
  it("does not let a late query failure erase acceptance learned by a cold read", async () => {
    const receipt = Promise.withResolvers<{ userItemId: string }>();
    const steer = vi.fn(() => nominatedSubmission(() => receipt.promise, "late-failure"));
    gateway = installAgentRuntimeGateway(() => ({ runs: { steer } }) as unknown as FlameClient);
    steerRunningTurn({
      sessionId,
      runId,
      segmentId,
      input: { parts: [{ kind: "text", text: "original" }] },
    });
    await vi.waitFor(() => expect(messages()[0]?.id).toBe("item_original"));
    await revalidateAgentSessionMaterial(sessionId, async () => ({
      snapshot: {
        runs: [runtimeRunFact(root)],
        items: [],
        pendingInterruptSets: [],
        steerInputs: [
          {
            runId,
            userItemId: "item_original",
            input: { parts: [{ kind: "text", text: "original" }] },
            receipt: "accepted",
          },
        ],
      },
      projectAssociatedSharedMaterial: (shared) => shared,
    }));
    receipt.reject(new RpcTransportError("ACK and original query unavailable"));
    await vi.waitFor(() => expect(messages()[0]?.steer?.delivery).toBe("unknown"));
    expect(messages()).toHaveLength(1);
    expect(messages()[0]?.steer?.receipt).toBe("accepted");
    expect(steerInputFeedback(messages()[0]!)).toBe("accepted");
    expect(useNotificationStore.getState().log).toHaveLength(0);
  });

  it("keeps equal payloads separate under their original nominated identities", async () => {
    let count = 0;
    const steer = vi.fn(() => {
      const id = `opaque-${++count}`;
      return nominatedSubmission(async () => ({ userItemId: id }), `command-${count}`, id);
    });
    gateway = installAgentRuntimeGateway(() => ({ runs: { steer } }) as unknown as FlameClient);
    for (let n = 0; n < 2; n++)
      steerRunningTurn({
        sessionId,
        runId,
        segmentId,
        input: { parts: [{ kind: "text", text: "same text" }] },
      });
    await vi.waitFor(() =>
      expect(messages().every((message) => message.steer?.receipt === "accepted")).toBe(true),
    );
    expect(messages().map((message) => message.id)).toEqual(["opaque-1", "opaque-2"]);
    expect(messages()[0]?.blocks).toEqual(messages()[1]?.blocks);
  });

  it("preserves a committed Item when its local command observer is retired", async () => {
    const receipt = Promise.withResolvers<{ userItemId: string }>();
    const steer = vi.fn(() =>
      nominatedSubmission(() => receipt.promise, "committed-before-retirement"),
    );
    gateway = installAgentRuntimeGateway(() => ({ runs: { steer } }) as unknown as FlameClient);
    steerRunningTurn({
      sessionId,
      runId,
      segmentId,
      input: { parts: [{ kind: "text", text: "committed" }] },
    });
    await vi.waitFor(() => expect(messages()[0]?.id).toBe("item_original"));
    useAgentStore.getState().applyRunEvents(sessionId, [
      {
        eventId: "evt_committed",
        runId,
        segmentId,
        timestamp: root.createdAt,
        event: {
          type: "item.completed",
          item: {
            id: "item_original",
            runId,
            status: "completed",
            type: "userMessage",
            createdAt: root.createdAt,
            content: [{ type: "text", text: "committed" }],
          },
        },
      },
    ]);
    gateway.replaceRuntimeGeneration();
    receipt.resolve({ userItemId: "item_original" });
    await receipt.promise;
    await Promise.resolve();
    expect(messages()).toHaveLength(1);
    expect(messages()[0]?.runId).toBe(runId);
    expect(steerInputFeedback(messages()[0]!)).toBe("applied");
  });
});
