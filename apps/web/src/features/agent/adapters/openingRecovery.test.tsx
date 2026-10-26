import { createAgentGatewayFixture } from "@/test/agentOwners";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { loadPluginsForTest } from "@/test/kernel";
import { type RuntimeAgentDriver } from "@/features/agent/adapters/runtimeDriver";
import { type FlameClient } from "@flame/runtime-client";
import { createMethods } from "@flame/runtime-client";
import { type RpcClient, type RpcCallOptions } from "@flame/runtime-client";
import { createMutationJournal, type MutationJournalStorage } from "@flame/runtime-client";
import { RpcTransportError } from "@flame/runtime-client";
import {
  type StartRunRequest,
  type ResumeRunRequest,
  type ServerCapabilities,
  type RunRef,
  type Session,
  type SessionSnapshot,
} from "@flame/runtime-client/wire";
import discovery from "@flame/runtime-client/testing/samples/method.discover.resp.json";
import originalRun from "@flame/runtime-client/testing/samples/runref.full.json";
import originalSession from "@flame/runtime-client/testing/samples/session.json";
import { useAgentSession } from "@/features/agent/adapters/useAgentSession";

import { agentTextInput } from "@/features/agent/domain/input";
import { revalidateAgentSessionMaterial } from "@/features/agent/application/session/refreshSessionProjection";
import { runtimeSessionMaterial } from "@/features/agent/adapters/runtimeSessionMaterial";
import { contentBlocksToAgentInput } from "@/features/agent/adapters/wireInput";
const SID = "ses_cold_opening";

const prefix = "cold-opening-test:";

let disposeGateway: ReturnType<typeof createAgentGatewayFixture> | undefined;

const journals: Array<ReturnType<typeof createMutationJournal>> = [];

function storage(): MutationJournalStorage {
  return {
    get: (key) => {
      const value = localStorage.getItem(prefix + key);
      return value === null ? undefined : JSON.parse(value);
    },
    set: (key, value) => localStorage.setItem(prefix + key, JSON.stringify(value)),
    remove: (key) => localStorage.removeItem(prefix + key),
    keys: () =>
      Object.keys(localStorage)
        .filter((key) => key.startsWith(prefix))
        .map((key) => key.slice(prefix.length)),
  };
}

const requests = {
  start: {
    sessionId: SID,
    input: [{ type: "text", text: "original start" }],
    provider: "original-provider",
    model: "original-model",
  } satisfies StartRunRequest,
  resume: {
    runId: "run_original",
    input: [{ type: "text", text: "original extra input" }],
    responses: [
      {
        itemId: "item_reviewed",
        response: {
          type: "approval",
          decision: "approve",
          editedArgs: { path: "/reviewed" },
          reason: "original reason",
          remember: { scope: "project" },
        },
      },
      { itemId: "item_question", response: { type: "answer", answers: [["original answer"]] } },
    ],
  } satisfies ResumeRunRequest,
};

function open(call: RpcClient["call"]) {
  const journal = createMutationJournal({
    storage: storage(),
    scope: () => ({ namespace: "idp_opening_original", retentionSeconds: 86400 }),
  });
  journals.push(journal);
  const close = vi.fn();
  const methods = createMethods(
    {
      call,
      subscribe: vi.fn(() => close),
      onStreamEnd: vi.fn(() => close),
    } as unknown as RpcClient,
    {
      mutationJournal: journal,
      capabilities: () => discovery.capabilities as ServerCapabilities,
    },
  );
  return { journal, methods, close };
}

async function seed(operation: "start" | "resume") {
  const first = open(
    vi.fn(async (method: string, _params: unknown, options?: RpcCallOptions) => {
      if (method === "runs.getAcceptance") return { type: "notRecorded" };
      options?.onRequestRpcId?.("request_lost");
      throw new RpcTransportError("application exited after dispatch");
    }) as RpcClient["call"],
  );
  const body = structuredClone(requests[operation]);
  const pending =
    operation === "start"
      ? first.methods.runs.start(body as StartRunRequest)
      : first.methods.runs.resume(body as ResumeRunRequest);
  await expect(pending).rejects.toBeInstanceOf(RpcTransportError);
  first.journal.dispose();
  return pending.idempotencyKey;
}

function mountedClient(
  operation: "start" | "resume",
  options: {
    accepted?: boolean;
    queryFailure?: boolean;
    retired?: boolean;
    observationFailure?: boolean;
  } = {},
) {
  let accepted = options.accepted ?? false;
  const run: RunRef = {
    ...(originalRun as RunRef),
    id: "run_original",
    sessionId: SID,
    parentRunId: undefined,
    outcome: undefined,
    finishedAt: undefined,
    status: accepted ? "running" : "waiting",
    activeSegmentId: accepted ? "seg_current" : undefined,
  };
  const snapshot = vi.fn(async (): Promise<SessionSnapshot> => {
    if (accepted && options.observationFailure)
      throw new RpcTransportError("accepted observation unavailable");
    return {
      session: {
        ...(originalSession as Session),
        id: SID,
        status:
          options.retired || (!accepted && operation === "start")
            ? "idle"
            : accepted
              ? "running"
              : "waiting",
      },
      items:
        accepted && !options.retired
          ? [
              {
                id: "item_original",
                runId: run.id,
                type: "userMessage",
                status: "completed",
                createdAt: run.createdAt,
                content: requests[operation].input,
              },
            ]
          : [],
      runs:
        options.retired || (!accepted && operation === "start")
          ? []
          : [
              {
                ...run,
                status: accepted ? "running" : "waiting",
                activeSegmentId: accepted ? "seg_current" : undefined,
              },
            ],
      interrupts: [],
    };
  });
  const call = vi.fn(async (method: string, _params: unknown, callOptions?: RpcCallOptions) => {
    if (method === "runs.getAcceptance") {
      if (options.queryFailure) throw new RpcTransportError("original receipt unavailable");
      return accepted
        ? {
            type: options.retired ? "retired" : "accepted",
            sessionId: SID,
            runId: run.id,
            segmentId: "seg_original",
            userItemId: "item_original",
            ...(!options.retired
              ? { run: { ...run, status: "running", activeSegmentId: "seg_current" } }
              : {}),
          }
        : { type: "notRecorded" };
    }
    accepted = true;
    callOptions?.onRequestRpcId?.("request_retried");
    return { runId: run.id, segmentId: "seg_original", userItemId: "item_original" };
  });
  const cold = open(call as RpcClient["call"]);
  const subscribe = vi.fn(async (_params: unknown, signal: AbortSignal) => ({
    result: { runId: run.id, segmentId: "seg_current", snapshot: await snapshot() },
    events: {
      async *[Symbol.asyncIterator]() {
        await new Promise<void>((resolve) => {
          if (signal.aborted) resolve();
          else signal.addEventListener("abort", () => resolve(), { once: true });
        });
        yield* [];
      },
    },
  }));
  const client = {
    ...cold.methods,
    sessions: { snapshot },
    runs: { ...cold.methods.runs, subscribe },
  } as unknown as FlameClient;
  disposeGateway = createAgentGatewayFixture(testClient, () => client);
  const driver = { start: vi.fn(), resume: vi.fn() } as unknown as RuntimeAgentDriver;
  const readClient = () => client;
  const hook = testClient.renderHook(() =>
    useAgentSession(testClient.sessionDependencies, readClient, () => driver, SID, vi.fn()),
  );
  return { ...cold, call, snapshot, subscribe, driver, hook };
}

beforeEach(async () => {
  await loadPluginsForTest();
  testClient.navigation.go({ session: SID });
  testClient.agent.projection.store.getState().dropSession(SID);
});

afterEach(() => {
  disposeGateway?.dispose();
  disposeGateway = undefined;
  for (const journal of journals.splice(0)) journal.dispose();
  for (const key of storage().keys()) storage().remove(key);
  testClient.agent.projection.store.getState().dropSession(SID);
  vi.restoreAllMocks();
});

describe.each(["start", "resume"] as const)("Desktop cold original %s", (operation) => {
  it("settles acceptance before observing and never restores uncertainty after an observation failure", async () => {
    const key = await seed(operation);
    const options = { observationFailure: true };
    const cold = mountedClient(operation, options);
    await waitFor(() =>
      expect(
        testClient.agent.projection.store.getState().sessions[SID]!.openingCommands,
      ).toHaveLength(1),
    );
    await act(async () => {});
    act(() => {
      expect(testClient.agent.projection.store.getState().sessions[SID]!.retryOpening!(key)).toBe(
        true,
      );
    });
    await waitFor(() =>
      expect(testClient.agent.projection.store.getState().sessions[SID]!.openingCommands).toEqual(
        [],
      ),
    );
    expect(storage().keys()).toEqual([]);
    expect(
      testClient.agent.projection.store.getState().sessions[SID]!.problem.commandError,
    ).toBeNull();
    act(() => {
      expect(
        testClient.agent.projection.store.getState().sessions[SID]!.send!(
          agentTextInput("changed draft"),
        ),
      ).toBe(false);
    });
    options.observationFailure = false;
    await act(async () => {
      await testClient.agent.projection.store.getState().sessions[SID]!.synchronize!("after-live");
    });
    await waitFor(() => expect(cold.subscribe).toHaveBeenCalledOnce());
    expect(cold.call.mock.calls.filter(([method]) => method === `runs.${operation}`)).toHaveLength(
      1,
    );
    expect(testClient.agent.projection.store.getState().sessions[SID]!.openingCommands).toEqual([]);
    cold.hook.unmount();
  });
  it("recreates all owners, shows the frozen original, and explicitly retries it once before following the current Segment", async () => {
    const key = await seed(operation);
    const cold = mountedClient(operation);
    await waitFor(() =>
      expect(
        testClient.agent.projection.store.getState().sessions[SID]!.openingCommands,
      ).toHaveLength(1),
    );
    const pending = testClient.agent.projection.store.getState().sessions[SID]!.openingCommands[0]!;
    expect(pending).toMatchObject({
      operation,
      idempotencyKey: key,
      input: contentBlocksToAgentInput(requests[operation].input),
      receipt: "notRecorded",
    });
    expect(pending.operation === "start" ? pending.options : pending.responses).toEqual(
      operation === "start"
        ? {
            provider: requests.start.provider,
            model: requests.start.model,
            reasoningEffort: undefined,
          }
        : requests.resume.responses,
    );
    expect(cold.call.mock.calls.map(([method]) => method)).toEqual(["runs.getAcceptance"]);
    expect(storage().keys()).toHaveLength(1);
    await revalidateAgentSessionMaterial(testClient.agent.access, SID, async () =>
      runtimeSessionMaterial(testClient.agent.access.sharedMaterial, SID, await cold.snapshot()),
    );
    expect(testClient.agent.projection.store.getState().sessions[SID]!.openingCommands).toEqual([
      pending,
    ]);
    await act(async () => {});
    act(() => {
      const session = testClient.agent.projection.store.getState().sessions[SID]!;
      expect(session.send!(agentTextInput("replacement draft"))).toBe(false);
      expect(session.resume!("run_replacement", [])).toBe(false);
      expect(session.retryOpening!(key)).toBe(true);
      expect(session.retryOpening!(key)).toBe(false);
    });
    await waitFor(() => expect(cold.subscribe).toHaveBeenCalledOnce());
    expect(cold.call.mock.calls.find(([method]) => method === `runs.${operation}`)).toEqual([
      `runs.${operation}`,
      requests[operation],
      expect.objectContaining({
        idempotencyKey: key,
        idempotencyNamespace: "idp_opening_original",
      }),
    ]);
    expect(cold.subscribe.mock.calls[0]?.[0]).toMatchObject({
      runId: "run_original",
      segmentId: "seg_current",
    });
    expect(cold.driver.start).not.toHaveBeenCalled();
    expect(cold.driver.resume).not.toHaveBeenCalled();
    expect(testClient.agent.projection.store.getState().sessions[SID]!.openingCommands).toEqual([]);
    expect(
      testClient.agent.projection.store
        .getState()
        .sessions[SID]!.view.messages.some((message) => message.id === "item_original"),
    ).toBe(true);
    expect(storage().keys()).toEqual([]);
    cold.hook.unmount();
  });

  it.each([false, true])(
    "folds a committed receipt after restart without another opening (retired=%s)",
    async (retired) => {
      await seed(operation);
      const cold = mountedClient(operation, { accepted: true, retired });
      await waitFor(() =>
        expect(
          testClient.agent.projection.store.getState().sessions[SID]!.authoritativeRevision,
        ).toBeGreaterThan(0n),
      );
      expect(cold.call.mock.calls.every(([method]) => method === "runs.getAcceptance")).toBe(true);
      expect(testClient.agent.projection.store.getState().sessions[SID]!.openingCommands).toEqual(
        [],
      );
      expect(storage().keys()).toEqual([]);
      await waitFor(() => expect(cold.subscribe).toHaveBeenCalledTimes(retired ? 0 : 1));
      cold.hook.unmount();
    },
  );

  it("keeps lookup failure visible and preserves the request without a mutation or a reconstructed driver call", async () => {
    await seed(operation);
    const evidence = storage()
      .keys()
      .map((key) => storage().get(key));
    const cold = mountedClient(operation, { queryFailure: true });
    await waitFor(() =>
      expect(
        testClient.agent.projection.store.getState().sessions[SID]!.openingCommands[0]?.receipt,
      ).toBe("unknown"),
    );
    await act(async () => {});
    act(() => {
      expect(
        testClient.agent.projection.store.getState().sessions[SID]!.retryOpening!(
          testClient.agent.projection.store.getState().sessions[SID]!.openingCommands[0]!
            .idempotencyKey,
        ),
      ).toBe(true);
    });
    await waitFor(() => expect(cold.call.mock.calls.length).toBeGreaterThan(1));
    expect(cold.call.mock.calls.every(([method]) => method === "runs.getAcceptance")).toBe(true);
    expect(
      storage()
        .keys()
        .map((key) => storage().get(key)),
    ).toEqual(evidence);
    expect(cold.driver.start).not.toHaveBeenCalled();
    expect(cold.driver.resume).not.toHaveBeenCalled();
    expect(
      testClient.agent.projection.store.getState().sessions[SID]!.problem.commandError,
    ).toBeNull();
    cold.hook.unmount();
  });
});
