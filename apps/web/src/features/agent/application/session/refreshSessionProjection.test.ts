import { createAgentStateFixture } from "@/test/agentOwners";
import { stubInstance } from "@/test/stubInstance";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  type AgentSessionMaterialRead,
  type AgentSessionSnapshot,
  type AgentRuntimeGateway,
} from "@/features/agent/application/runtimeGateway";
import { steerInputFeedback } from "@/features/agent/application/view/steerInputFeedback";

import { useNotificationStore } from "@/features/notifications/public/notifications";
import {
  refreshAgentSessionProjection,
  revalidateAgentSessionProjection,
  synchronizeMountedAgentSession,
  synchronizeMountedAgentSessions,
} from "@/features/agent/application/session/refreshSessionProjection";
const SESSION_ID = "ses_refresh";

function snapshot(revision: number): AgentSessionSnapshot {
  return {
    items: [],
    runs: [],
    pendingInterruptSets: [],
    plan: {
      revision,
      steps: [],
    },
  };
}

function material(
  value: AgentSessionSnapshot,
  projectAssociatedSharedMaterial: AgentSessionMaterialRead["projectAssociatedSharedMaterial"] = (
    shared,
  ) => shared,
): AgentSessionMaterialRead {
  return { snapshot: value, projectAssociatedSharedMaterial };
}

function companion(label: string) {
  return vi.fn((shared: Record<string, unknown>) => ({ ...shared, companion: label }));
}

let restoreRuntime: (() => void) | undefined;

beforeEach(() => {
  testClient.agent.projection.store.getState().dropSession(SESSION_ID);
  testClient.agent.projection.store.getState().ensureSession(SESSION_ID);
});

afterEach(() => {
  restoreRuntime?.();
  restoreRuntime = undefined;
  testClient.agent.projection.store.getState().dropSession(SESSION_ID);
});

describe("refreshAgentSessionProjection", () => {
  it.each([false, true])(
    "renders non-application only from the committed terminal input snapshot (superseded=%s)",
    async (superseded) => {
      testClient.agent.projection.store.getState().appendLocalMessage(SESSION_ID, {
        id: "reserved_user_item",
        role: "user",
        runId: null,
        blocks: [{ kind: "text", text: "Use the attached reference", status: "complete" }],
        steer: { runId: "run_steered", receipt: "accepted" },
      });
      const read = Promise.withResolvers<AgentSessionMaterialRead>();
      const terminal: AgentSessionSnapshot = {
        steerInputs: [
          {
            runId: "run_steered",
            userItemId: "reserved_user_item",
            receipt: "accepted",
            input: { parts: [{ kind: "text", text: "Use the attached reference" }] },
          },
        ],
        items: [],
        pendingInterruptSets: [],
        runs: [
          {
            id: "run_steered",
            sessionId: SESSION_ID,
            parentRunId: null,
            rootRunId: "run_steered",
            spawnedByItemId: null,
            status: "finished",
            activeSegmentId: null,
            outcome: { type: "completed" },
            metrics: {
              steps: 1,
              activeDurationMillis: 1,
              usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
            },
            createdAt: "2026-09-26T00:00:00Z",
            finishedAt: "2026-09-26T00:00:01Z",
          },
        ],
      };
      restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
        loadSessionSnapshot: () => read.promise,
      } as unknown as AgentRuntimeGateway);
      const refreshing = refreshAgentSessionProjection(testClient.agent.dependencies, SESSION_ID);
      expect(useNotificationStore.getState().log).toHaveLength(0);
      if (superseded) writeNewerProjectionFact();
      read.resolve(material(terminal));
      await refreshing;
      expect(useNotificationStore.getState().log).toHaveLength(0);
      const view = testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view;
      expect(view.messages).toHaveLength(superseded ? 2 : 1);
      const input = view.messages.find((message) => message.id === "reserved_user_item")!;
      expect(steerInputFeedback(input)).toBe(superseded ? "accepted" : "notApplied");
    },
  );

  it("preserves the mounted owner's shared settlement for repeated notifications", async () => {
    const pending = Promise.withResolvers<boolean>();
    testClient.agent.projection.store.getState().setSynchronize(SESSION_ID, () => pending.promise);
    try {
      expect(
        synchronizeMountedAgentSession(testClient.agent.dependencies, SESSION_ID, "after-live"),
      ).toBe(pending.promise);
    } finally {
      pending.resolve(false);
    }
  });

  it("lets a command await the mounted Session lifecycle owner", async () => {
    const synchronize = vi.fn().mockResolvedValue(true);
    testClient.agent.projection.store.getState().setSynchronize(SESSION_ID, synchronize);

    await expect(
      synchronizeMountedAgentSession(testClient.agent.dependencies, SESSION_ID, "after-live"),
    ).resolves.toBe(true);
    expect(synchronize).toHaveBeenCalledWith("after-live");
  });

  it("reports an unmounted Session instead of creating a second repair path", async () => {
    testClient.agent.projection.store.getState().dropSession(SESSION_ID);

    await expect(
      synchronizeMountedAgentSession(testClient.agent.dependencies, SESSION_ID, "after-live"),
    ).resolves.toBe(false);
  });

  it("keeps the old projection visible until the complete read commits", async () => {
    writeNewerProjectionFact();
    const visible = testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view;
    const read = Promise.withResolvers<AgentSessionMaterialRead>();
    const projectCompanion = companion("complete");
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(testClient.agent.dependencies, SESSION_ID);
    expect(testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view).toBe(visible);

    read.resolve(material(snapshot(1), projectCompanion));
    await expect(refreshing).resolves.toMatchObject({
      messages: [],
      plan: { revision: 1 },
      shared: { companion: "complete" },
    });
    expect(projectCompanion).toHaveBeenCalledWith({});
  });

  it("discards an older read when a newer refresh starts", async () => {
    const older = Promise.withResolvers<AgentSessionMaterialRead>();
    const newer = Promise.withResolvers<AgentSessionMaterialRead>();
    const projectOlder = companion("older");
    const projectNewer = companion("newer");
    const loadSessionSnapshot = vi
      .fn()
      .mockImplementationOnce(() => older.promise)
      .mockImplementationOnce(() => newer.promise);
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot,
    } as unknown as AgentRuntimeGateway);

    const olderRefresh = refreshAgentSessionProjection(testClient.agent.dependencies, SESSION_ID);
    const newerRefresh = refreshAgentSessionProjection(testClient.agent.dependencies, SESSION_ID);
    newer.resolve(material(snapshot(2), projectNewer));
    await expect(newerRefresh).resolves.toMatchObject({
      plan: { revision: 2 },
      shared: { companion: "newer" },
    });
    older.resolve(material(snapshot(1), projectOlder));
    await expect(olderRefresh).resolves.toBeNull();
    expect(testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view).toMatchObject({
      plan: { revision: 2 },
      shared: { companion: "newer" },
    });
    expect(projectNewer).toHaveBeenCalledOnce();
    expect(projectOlder).toHaveBeenCalledOnce();
  });

  it("discards a read that raced with a live projection write", async () => {
    const read = Promise.withResolvers<AgentSessionMaterialRead>();
    const projectCompanion = companion("raced");
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(testClient.agent.dependencies, SESSION_ID);
    writeNewerProjectionFact();
    read.resolve(material(snapshot(1), projectCompanion));

    await expect(refreshing).resolves.toBeNull();
    expect(
      testClient.agent.projection.store
        .getState()
        .sessions[SESSION_ID]!.view.messages.map((m) => m.id),
    ).toEqual(["newer_fact"]);
    expect(
      testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view.shared.companion,
    ).toBeUndefined();
    expect(projectCompanion).toHaveBeenCalledOnce();
  });

  it("returns authoritative facts even when a newer local write rejects their commit", async () => {
    const read = Promise.withResolvers<AgentSessionMaterialRead>();
    const projectCompanion = companion("authoritative-only");
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const revalidating = revalidateAgentSessionProjection(
      testClient.agent.dependencies,
      SESSION_ID,
    );
    writeNewerProjectionFact();
    read.resolve(material(snapshot(4), projectCompanion));

    await expect(revalidating).resolves.toMatchObject({
      committed: false,
      authoritativeView: {
        plan: { revision: 4 },
        shared: { companion: "authoritative-only" },
      },
    });
    expect(
      testClient.agent.projection.store
        .getState()
        .sessions[SESSION_ID]!.view.messages.map((m) => m.id),
    ).toEqual(["newer_fact"]);
    expect(
      testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view.shared.companion,
    ).toBeUndefined();
  });

  it("leaves the prior projection intact when the durable read fails", async () => {
    writeNewerProjectionFact();
    const visible = testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view;
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi.fn().mockRejectedValue(new Error("offline")),
    } as unknown as AgentRuntimeGateway);

    await expect(
      refreshAgentSessionProjection(testClient.agent.dependencies, SESSION_ID),
    ).rejects.toThrow("offline");
    expect(testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view).toBe(visible);
  });

  it("treats an authoritatively missing session as no applicable projection", async () => {
    writeNewerProjectionFact();
    const visible = testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view;
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi.fn().mockResolvedValue(null),
    } as unknown as AgentRuntimeGateway);

    await expect(
      refreshAgentSessionProjection(testClient.agent.dependencies, SESSION_ID),
    ).resolves.toBeNull();
    expect(testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view).toBe(visible);
  });

  it("retires a non-cooperative snapshot read without allowing a late commit", async () => {
    const read = Promise.withResolvers<AgentSessionMaterialRead>();
    const projectCompanion = companion("aborted");
    const controller = new AbortController();
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(testClient.agent.dependencies, SESSION_ID, {
      signal: controller.signal,
    });
    controller.abort();
    await expect(refreshing).resolves.toBeNull();

    read.resolve(material(snapshot(9), projectCompanion));
    await Promise.resolve();
    const plan = testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view.plan;
    expect(plan?.revision).not.toBe(9);
    expect(projectCompanion).not.toHaveBeenCalled();
  });

  it("revokes an unsignaled snapshot at the Runtime connection boundary", async () => {
    const read = Promise.withResolvers<AgentSessionMaterialRead>();
    const projectCompanion = companion("retired");
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(testClient.agent.dependencies, SESSION_ID);
    synchronizeMountedAgentSessions(testClient.agent.dependencies, { ownership: "retire-live" });
    read.resolve(material(snapshot(11), projectCompanion));

    await expect(refreshing).resolves.toBeNull();
    const plan = testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view.plan;
    expect(plan?.revision).not.toBe(11);
    expect(
      testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view.shared.companion,
    ).toBeUndefined();
    expect(projectCompanion).toHaveBeenCalledOnce();
  });

  it("clears the old server projection before admitting its successor read", async () => {
    const predecessor = Promise.withResolvers<AgentSessionMaterialRead>();
    const successor = Promise.withResolvers<AgentSessionMaterialRead>();
    const loadSessionSnapshot = vi
      .fn()
      .mockImplementationOnce(() => predecessor.promise)
      .mockImplementationOnce(() => successor.promise);
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot,
    } as unknown as AgentRuntimeGateway);
    testClient.agent.projection.store
      .getState()
      .setCommandError(SESSION_ID, { code: "old-server" });

    const retiredRead = refreshAgentSessionProjection(testClient.agent.dependencies, SESSION_ID);
    synchronizeMountedAgentSessions(testClient.agent.dependencies, { ownership: "replace-server" });

    expect(
      testClient.agent.projection.store.getState().sessions[SESSION_ID]!.problem.commandError,
    ).toBeNull();
    expect(loadSessionSnapshot).toHaveBeenCalledTimes(2);
    successor.resolve(material(snapshot(22)));
    await vi.waitFor(() =>
      expect(
        testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view.plan,
      ).toMatchObject({
        revision: 22,
      }),
    );

    predecessor.resolve(material(snapshot(11)));
    await expect(retiredRead).resolves.toBeNull();
    expect(
      testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view.plan,
    ).toMatchObject({
      revision: 22,
    });
  });

  it("rejects an old port's snapshot and companion material after adapter replacement", async () => {
    const read = Promise.withResolvers<AgentSessionMaterialRead>();
    const projectCompanion = companion("retired-port");
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);
    const disposeRetiredState = createAgentStateFixture(testClient).dispose;
    let disposeSuccessorState: (() => void) | undefined;
    try {
      const refreshing = refreshAgentSessionProjection(testClient.agent.dependencies, SESSION_ID);
      disposeSuccessorState = createAgentStateFixture(testClient).dispose;

      read.resolve(material(snapshot(10), projectCompanion));

      await expect(refreshing).resolves.toBeNull();
      const plan = testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view.plan;
      expect(plan?.revision).not.toBe(10);
      expect(
        testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view.shared.companion,
      ).toBeUndefined();
      expect(projectCompanion).toHaveBeenCalledOnce();
    } finally {
      disposeRetiredState();
      disposeSuccessorState?.();
    }
  });
});

function writeNewerProjectionFact(): void {
  testClient.agent.projection.store.getState().appendLocalMessage(SESSION_ID, {
    id: "newer_fact",
    role: "user",
    runId: null,
    blocks: [],
  });
}
