import { createAgentGatewayFixture } from "@/test/agentOwners";
import { stubInstance } from "@/test/stubInstance";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient({ runtimeClient: getRuntimeClient });
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { type AgentRunFact as RunRef } from "@/features/agent/public/events";
import { type FlameClient } from "@flame/runtime-client";

import {
  type AgentRuntimeGateway,
  type AgentSessionMaterialRead,
  type AgentSessionSnapshot,
} from "@/features/agent/application/runtimeGateway";
import { type AgentSessionViewPort } from "@/features/agent/application/session/sessionAccess";
import {
  forkAgentSessionAtRun,
  rollbackSessionToBeforeRun,
} from "@/features/agent/application/session/historyActions";
import { loadPluginsForTest } from "@/test/kernel";
let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};

const getRuntimeClient = () => runtimeClient();

let restoreRuntime: (() => void) | undefined;

let restoreView: (() => void) | undefined;

beforeAll(async () => {
  await loadPluginsForTest();
});

afterEach(() => {
  restoreRuntime?.();
  restoreView?.();
  restoreRuntime = undefined;
  restoreView = undefined;
});

describe("rollbackSessionToBeforeRun", () => {
  it("does not issue a rollback when the session is authoritatively absent", async () => {
    const rollbackSession = vi.fn();
    restoreRuntime = stubInstance(testClient.agent.access.runtime, {
      loadSessionSnapshot: vi.fn().mockResolvedValue(null),
      rollbackSession,
    } as unknown as AgentRuntimeGateway);

    await expect(
      rollbackSessionToBeforeRun(testClient.agent.access, "ses_gone", "run_gone"),
    ).resolves.toEqual({
      status: "unavailable",
    });
    expect(rollbackSession).not.toHaveBeenCalled();
  });

  it("does not issue a rollback for a child in the requested Session", async () => {
    const child = {
      ...run("run_child", "2026-08-12T00:00:01.000Z"),
      parentRunId: "run_1",
      rootRunId: "run_1",
      spawnedByItemId: "item_delegate",
    };
    const rollbackSession = vi.fn();
    restoreRuntime = stubInstance(testClient.agent.access.runtime, {
      loadSessionSnapshot: vi
        .fn()
        .mockResolvedValue(material({ ...snapshot(), runs: [...snapshot().runs, child] })),
      rollbackSession,
    } as unknown as AgentRuntimeGateway);

    await expect(
      rollbackSessionToBeforeRun(testClient.agent.access, "ses_1", child.id),
    ).resolves.toEqual({
      status: "unavailable",
    });
    expect(rollbackSession).not.toHaveBeenCalled();
  });

  it("returns the Runtime-authored input dropped by the committed rollback", async () => {
    const synchronize = vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true);
    restoreView = stubInstance(testClient.agent.access.view, {
      getSession: () => ({ synchronize }),
    } as unknown as AgentSessionViewPort);
    const rollbackSession = vi.fn().mockResolvedValue({
      droppedRuns: [
        {
          runId: "run_2",
          userInput: {
            parts: [
              { kind: "text", text: "authoritative prompt" },
              { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
            ],
          },
        },
      ],
    });
    restoreRuntime = stubInstance(testClient.agent.access.runtime, {
      loadSessionSnapshot: vi.fn().mockResolvedValue(material(snapshot())),
      rollbackSession,
    } as unknown as AgentRuntimeGateway);

    await expect(
      rollbackSessionToBeforeRun(testClient.agent.access, "ses_1", "run_2"),
    ).resolves.toEqual({
      status: "committed",
      userInput: {
        parts: [
          { kind: "text", text: "authoritative prompt" },
          { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
        ],
      },
    });
    expect(rollbackSession).toHaveBeenCalledWith({ sessionId: "ses_1", toRunId: "run_1" });
    expect(synchronize).toHaveBeenCalledTimes(2);
  });

  it("admits only one destructive history rewrite per Session", async () => {
    let release!: (value: AgentSessionMaterialRead) => void;
    const firstRead = new Promise<AgentSessionMaterialRead>((resolve) => {
      release = resolve;
    });
    const rollbackSession = vi.fn().mockResolvedValue({ droppedRuns: [] });
    restoreRuntime = stubInstance(testClient.agent.access.runtime, {
      loadSessionSnapshot: vi.fn(() => firstRead),
      rollbackSession,
    } as unknown as AgentRuntimeGateway);
    restoreView = stubInstance(testClient.agent.access.view, {
      getSession: () => ({ synchronize: vi.fn().mockResolvedValue(true) }),
    } as unknown as AgentSessionViewPort);

    const first = rollbackSessionToBeforeRun(testClient.agent.access, "ses_1", "run_2");
    await expect(
      rollbackSessionToBeforeRun(testClient.agent.access, "ses_1", "run_2"),
    ).resolves.toEqual({
      status: "inFlight",
    });
    release(material(snapshot()));
    await expect(first).resolves.toMatchObject({ status: "committed" });
    expect(rollbackSession).toHaveBeenCalledOnce();
  });

  it("does not attach an old snapshot inspection to the successor rollback writer", async () => {
    const read = Promise.withResolvers<AgentSessionMaterialRead>();
    restoreRuntime = stubInstance(testClient.agent.access.runtime, {
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);
    restoreView = stubInstance(testClient.agent.access.view, {
      getSession: () => ({ synchronize: vi.fn().mockResolvedValue(true) }),
    } as unknown as AgentSessionViewPort);
    const successorRollback = vi.fn().mockResolvedValue({ droppedRuns: [] });
    runtimeClient = () => ({ sessions: { rollback: successorRollback } }) as unknown as FlameClient;

    const retired = rollbackSessionToBeforeRun(testClient.agent.access, "ses_1", "run_2");
    const disposeSuccessor = createAgentGatewayFixture(testClient, getRuntimeClient);
    let retiredSettled = false;
    void retired.then(() => {
      retiredSettled = true;
    });
    await flushMicrotasks();
    const retiredSettledBeforeOldRead = retiredSettled;
    read.resolve(material(snapshot()));
    try {
      await expect(retired).resolves.toEqual({ status: "unavailable" });
      expect(retiredSettledBeforeOldRead).toBe(true);
      expect(successorRollback).not.toHaveBeenCalled();
    } finally {
      disposeSuccessor.dispose();
    }
  });

  it.each(["files", "both"] as const)(
    "does not degrade first-turn %s restore into destructive history rollback",
    async (restoreType) => {
      const rollbackSession = vi.fn();
      restoreRuntime = stubInstance(testClient.agent.access.runtime, {
        loadSessionSnapshot: vi.fn().mockResolvedValue(
          material({
            runs: [run("run_1", "2026-08-12T00:00:00.000Z")],
            items: [],
            pendingInterruptSets: [],
          }),
        ),
        rollbackSession,
      } as unknown as AgentRuntimeGateway);

      await expect(
        rollbackSessionToBeforeRun(testClient.agent.access, "ses_1", "run_1", restoreType),
      ).resolves.toEqual({
        status: "unavailable",
      });
      expect(rollbackSession).not.toHaveBeenCalled();
    },
  );
});

describe("forkAgentSessionAtRun", () => {
  it("does not join an old fork or let its response navigate the successor", async () => {
    const retiredFork = Promise.withResolvers<{ id: string }>();
    restoreRuntime = stubInstance(testClient.agent.access.runtime, {
      forkSession: vi.fn(() => retiredFork.promise),
    } as unknown as AgentRuntimeGateway);
    const successorFork = vi.fn().mockResolvedValue({ id: "fork_successor" });
    runtimeClient = () => ({ sessions: { fork: successorFork } }) as unknown as FlameClient;

    const retired = forkAgentSessionAtRun(testClient.agent.access, "ses_1", "run_1");
    const disposeSuccessor = createAgentGatewayFixture(testClient, getRuntimeClient);
    const successor = forkAgentSessionAtRun(testClient.agent.access, "ses_1", "run_1");
    await Promise.resolve();
    const successorStartedBeforeRetiredSettlement = successorFork.mock.calls.length;
    let retiredSettled = false;
    void retired.then(() => {
      retiredSettled = true;
    });
    await flushMicrotasks();
    const retiredSettledBeforeOldRPC = retiredSettled;
    retiredFork.resolve({ id: "fork_retired" });
    try {
      await Promise.all([retired, successor]);
      expect(successorStartedBeforeRetiredSettlement).toBe(1);
      expect(retiredSettledBeforeOldRPC).toBe(true);
      expect(testClient.navigation.get().session).toBe("fork_successor");
    } finally {
      disposeSuccessor.dispose();
    }
  });
});

function snapshot(): AgentSessionSnapshot {
  return {
    runs: [run("run_1", "2026-08-12T00:00:00.000Z"), run("run_2", "2026-08-12T00:01:00.000Z")],
    items: [],
    pendingInterruptSets: [],
  };
}

function material(value: AgentSessionSnapshot): AgentSessionMaterialRead {
  return { snapshot: value, projectAssociatedSharedMaterial: (shared) => shared };
}

async function flushMicrotasks(): Promise<void> {
  for (let index = 0; index < 8; index += 1) await Promise.resolve();
}

function run(id: string, createdAt: string): RunRef {
  return {
    id,
    sessionId: "ses_1",
    status: "finished",
    parentRunId: null,
    rootRunId: id,
    spawnedByItemId: null,
    activeSegmentId: null,
    createdAt,
    finishedAt: createdAt,
    outcome: { type: "completed" },
    metrics: {
      steps: 1,
      activeDurationMillis: 1,
      usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
    },
  };
}
