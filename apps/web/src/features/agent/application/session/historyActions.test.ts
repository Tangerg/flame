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
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { EMPTY_AGENT_SESSION_VIEW } from "@/features/agent/public/sessionView";
import { type AgentInput } from "@/features/agent/public/input";
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
  regenerateSessionRun,
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
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi.fn().mockResolvedValue(null),
      rollbackSession,
    } as unknown as AgentRuntimeGateway);

    await expect(
      rollbackSessionToBeforeRun(testClient.agent.dependencies, "ses_gone", "run_gone"),
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
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi
        .fn()
        .mockResolvedValue(material({ ...snapshot(), runs: [...snapshot().runs, child] })),
      rollbackSession,
    } as unknown as AgentRuntimeGateway);

    await expect(
      rollbackSessionToBeforeRun(testClient.agent.dependencies, "ses_1", child.id),
    ).resolves.toEqual({
      status: "unavailable",
    });
    expect(rollbackSession).not.toHaveBeenCalled();
  });

  it("returns the Runtime-authored input dropped by the committed rollback", async () => {
    const synchronize = vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true);
    restoreView = stubInstance(testClient.agent.dependencies.view, {
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
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi.fn().mockResolvedValue(material(snapshot())),
      rollbackSession,
    } as unknown as AgentRuntimeGateway);

    await expect(
      rollbackSessionToBeforeRun(testClient.agent.dependencies, "ses_1", "run_2"),
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
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi.fn(() => firstRead),
      rollbackSession,
    } as unknown as AgentRuntimeGateway);
    restoreView = stubInstance(testClient.agent.dependencies.view, {
      getSession: () => ({ synchronize: vi.fn().mockResolvedValue(true) }),
    } as unknown as AgentSessionViewPort);

    const first = rollbackSessionToBeforeRun(testClient.agent.dependencies, "ses_1", "run_2");
    await expect(
      rollbackSessionToBeforeRun(testClient.agent.dependencies, "ses_1", "run_2"),
    ).resolves.toEqual({
      status: "inFlight",
    });
    release(material(snapshot()));
    await expect(first).resolves.toMatchObject({ status: "committed" });
    expect(rollbackSession).toHaveBeenCalledOnce();
  });

  it("does not attach an old snapshot inspection to the successor rollback writer", async () => {
    const read = Promise.withResolvers<AgentSessionMaterialRead>();
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);
    restoreView = stubInstance(testClient.agent.dependencies.view, {
      getSession: () => ({ synchronize: vi.fn().mockResolvedValue(true) }),
    } as unknown as AgentSessionViewPort);
    const successorRollback = vi.fn().mockResolvedValue({ droppedRuns: [] });
    runtimeClient = () => ({ sessions: { rollback: successorRollback } }) as unknown as FlameClient;

    const retired = rollbackSessionToBeforeRun(testClient.agent.dependencies, "ses_1", "run_2");
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
      restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
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
        rollbackSessionToBeforeRun(testClient.agent.dependencies, "ses_1", "run_1", restoreType),
      ).resolves.toEqual({
        status: "unavailable",
      });
      expect(rollbackSession).not.toHaveBeenCalled();
    },
  );
});

describe("regenerateSessionRun", () => {
  const originalInput: AgentInput = {
    parts: [
      { kind: "text", text: "original Runtime input" },
      { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
    ],
  };
  let read: ReturnType<typeof vi.fn>;
  let rollback: ReturnType<typeof vi.fn>;
  let send: ReturnType<typeof vi.fn>;
  let otherSend: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    read = vi.fn().mockResolvedValue(material(snapshot()));
    rollback = vi.fn().mockResolvedValue({
      droppedRuns: [{ runId: "run_2", userInput: originalInput }],
    });
    send = vi.fn().mockResolvedValue({
      status: "accepted",
      sessionId: "ses_1",
      runId: "run_new",
      userItemId: "it_new",
    });
    otherSend = vi.fn();
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: read,
      rollbackSession: rollback,
    } as unknown as AgentRuntimeGateway);
    restoreView = stubInstance(testClient.agent.dependencies.view, {
      getSession: (id: string) => ({
        view: EMPTY_AGENT_SESSION_VIEW,
        openingCommands: [],
        synchronize: vi.fn().mockResolvedValue(true),
        send: id === "ses_1" ? send : otherSend,
      }),
    } as unknown as AgentSessionViewPort);
  });

  it.each(["history", "both"] as const)(
    "submits only original dropped input after committed %s rollback",
    async (restoreType) => {
      await expect(
        regenerateSessionRun(testClient.agent.dependencies, "ses_1", "run_2", restoreType),
      ).resolves.toEqual({ status: "submitted" });
      expect(rollback).toHaveBeenCalledWith({
        sessionId: "ses_1",
        toRunId: "run_1",
        ...(restoreType === "both" ? { restoreType } : {}),
      });
      expect(send.mock.calls).toEqual([[originalInput, {}]]);
      expect(otherSend).not.toHaveBeenCalled();
    },
  );

  it("does not roll back or send before the first file checkpoint", async () => {
    read.mockResolvedValue(material({ ...snapshot(), runs: [snapshot().runs[0]!] }));
    await expect(
      regenerateSessionRun(testClient.agent.dependencies, "ses_1", "run_1", "both"),
    ).resolves.toEqual({ status: "unavailable" });
    expect(rollback).not.toHaveBeenCalled();
    expect(send).not.toHaveBeenCalled();
  });

  it.each(["history", "both"] as const)(
    "does not convert a missing target into appended %s input",
    async (restoreType) => {
      await expect(
        regenerateSessionRun(testClient.agent.dependencies, "ses_1", "run_gone", restoreType),
      ).resolves.toEqual({ status: "unavailable" });
      expect(rollback).not.toHaveBeenCalled();
      expect(send).not.toHaveBeenCalled();
    },
  );

  it("does not synthesize input when the committed result has none", async () => {
    rollback.mockResolvedValue({ droppedRuns: [] });
    await expect(
      regenerateSessionRun(testClient.agent.dependencies, "ses_1", "run_2"),
    ).resolves.toEqual({ status: "noInput" });
    expect(rollback).toHaveBeenCalledOnce();
    expect(send).not.toHaveBeenCalled();
  });

  it("preserves definite rollback failure without sending", async () => {
    const refusal = new Error("checkpoint refused");
    rollback.mockRejectedValue(refusal);
    await expect(
      regenerateSessionRun(testClient.agent.dependencies, "ses_1", "run_2", "both"),
    ).rejects.toBe(refusal);
    expect(send).not.toHaveBeenCalled();
  });

  it("keeps duplicate regeneration out of the original rollback lease", async () => {
    const pending = Promise.withResolvers<AgentSessionMaterialRead>();
    read.mockReturnValue(pending.promise);
    const first = regenerateSessionRun(testClient.agent.dependencies, "ses_1", "run_2");
    await expect(
      regenerateSessionRun(testClient.agent.dependencies, "ses_1", "run_2"),
    ).resolves.toEqual({ status: "inFlight" });
    expect(send).not.toHaveBeenCalled();
    pending.resolve(material(snapshot()));
    await expect(first).resolves.toEqual({ status: "submitted" });
    expect(rollback).toHaveBeenCalledOnce();
    expect(send).toHaveBeenCalledOnce();
  });

  it("keeps the lease until the follow-up is admitted", async () => {
    let duplicate: ReturnType<typeof regenerateSessionRun> | undefined;
    send.mockImplementation(() => {
      duplicate = regenerateSessionRun(testClient.agent.dependencies, "ses_1", "run_2");
      return Promise.resolve({
        status: "accepted",
        sessionId: "ses_1",
        runId: "run_new",
        userItemId: "it_new",
      });
    });
    await expect(
      regenerateSessionRun(testClient.agent.dependencies, "ses_1", "run_2"),
    ).resolves.toEqual({ status: "submitted" });
    await expect(duplicate).resolves.toEqual({ status: "inFlight" });
    expect(send).toHaveBeenCalledOnce();
  });

  it("reports a follow-up refused by the original Session without selecting another", async () => {
    send.mockReturnValue(null);
    await expect(
      regenerateSessionRun(testClient.agent.dependencies, "ses_1", "run_2"),
    ).resolves.toEqual({ status: "notReady" });
    expect(otherSend).not.toHaveBeenCalled();
  });

  it("keeps the captured Session after navigation changes", async () => {
    const pending = Promise.withResolvers<AgentSessionMaterialRead>();
    read.mockReturnValue(pending.promise);
    const result = regenerateSessionRun(testClient.agent.dependencies, "ses_1", "run_2");
    testClient.agent.access.selectAgentSession("ses_other");
    pending.resolve(material(snapshot()));
    await expect(result).resolves.toEqual({ status: "submitted" });
    expect(rollback).toHaveBeenCalledWith({ sessionId: "ses_1", toRunId: "run_1" });
    expect(send.mock.calls).toEqual([[originalInput, {}]]);
    expect(otherSend).not.toHaveBeenCalled();
  });

  it("does not deliver a retired endpoint's follow-up to its successor", async () => {
    const pending = Promise.withResolvers<AgentSessionMaterialRead>();
    read.mockReturnValue(pending.promise);
    const result = regenerateSessionRun(testClient.agent.dependencies, "ses_1", "run_2");
    const successorRollback = vi.fn();
    runtimeClient = () => ({ sessions: { rollback: successorRollback } }) as unknown as FlameClient;
    const successor = createAgentGatewayFixture(testClient, getRuntimeClient);
    try {
      await expect(result).resolves.toEqual({ status: "unavailable" });
      pending.resolve(material(snapshot()));
      await flushMicrotasks();
      expect(rollback).not.toHaveBeenCalled();
      expect(successorRollback).not.toHaveBeenCalled();
      expect(send).not.toHaveBeenCalled();
      expect(otherSend).not.toHaveBeenCalled();
    } finally {
      successor.dispose();
    }
  });
});

describe("forkAgentSessionAtRun", () => {
  it("does not join an old fork or let its response navigate the successor", async () => {
    const retiredFork = Promise.withResolvers<{ id: string }>();
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      forkSession: vi.fn(() => retiredFork.promise),
    } as unknown as AgentRuntimeGateway);
    const successorFork = vi.fn().mockResolvedValue({ id: "fork_successor" });
    runtimeClient = () => ({ sessions: { fork: successorFork } }) as unknown as FlameClient;

    const retired = forkAgentSessionAtRun(testClient.agent.dependencies, "ses_1", "run_1");
    const disposeSuccessor = createAgentGatewayFixture(testClient, getRuntimeClient);
    const successor = forkAgentSessionAtRun(testClient.agent.dependencies, "ses_1", "run_1");
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
