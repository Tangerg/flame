import { createAgentGatewayFixture } from "@/test/agentOwners";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient({ runtimeClient: getRuntimeClient });
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type RuntimeAgentDriver } from "@/features/agent/adapters/runtimeDriver";
import {
  type FlameClient,
  type RunEvent,
  type RunRef,
  type ServerCapabilities,
} from "@flame/runtime-client";

import { useAgentSession } from "@/features/agent/adapters/useAgentSession";
import { selectCurrentRootRun } from "@/features/agent/application/view/runTree";
import { loadPluginsForTest } from "@/test/kernel";

import { type GoalState } from "@/features/goal/application/goalReadModel";
import discovery from "@flame/runtime-client/testing/samples/method.discover.resp.json";
import { useNotificationStore } from "@/features/notifications/public/notifications";
import { t } from "@/lib/i18n/index";
let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};

const getRuntimeClient = () => runtimeClient();

const SID = "ses_goal_live";

const run: RunRef = {
  id: "run_default",
  sessionId: SID,
  status: "running",
  activeSegmentId: "seg_default",
  createdAt: "2026-09-13T00:00:00.000Z",
  metrics: { steps: 0, activeDurationMillis: 0 },
  protocolProfile: { interruptTypes: [], requiredFeatures: [] },
  provider: "openai",
  model: "gpt-5",
};

beforeEach(async () => {
  await loadPluginsForTest();
  createAgentGatewayFixture(testClient, getRuntimeClient);
});

afterEach(async () => {
  testClient.agent.projection.store.getState().dropSession(SID);
  testClient.agent.sessions.store.setState({ openSessionIds: [], lastSessionId: "" });
});

describe("Goal and mounted Session integration", () => {
  it("retains the owner stop reason across cold remount and refreshes it from the next snapshot", async () => {
    const { installGoalRuntimeAdapter } =
      await import("@/features/goal/adapters/runtimeGoalCommandsGateway");
    const capabilities = discovery.capabilities as ServerCapabilities;
    testClient.projection.store.setState({
      capabilities: {
        ...capabilities,
        features: {
          ...capabilities.features,
          goals: { enabled: true, clientOptIn: false, requiredByRunProtocol: false },
        },
      },
    });
    const base = { items: [], runs: [], interrupts: [], plan: { sessionId: SID } };
    const goal = {
      sessionId: SID,
      objective: "Finish the release",
      status: "paused" as const,
      reason: { code: "runtimeRestarted" as const },
      used: { runs: 1, steps: 2 },
      createdAt: "2026-10-14T00:00:00Z",
      updatedAt: "2026-10-14T01:00:00Z",
    };
    const snapshot = vi.fn().mockResolvedValue({ ...base, goal });
    const start = vi.fn();
    const resume = vi.fn();
    runtimeClient = () =>
      ({
        sessions: { snapshot },
        runs: { recoverCommands: async () => [] },
      }) as unknown as FlameClient;
    const adapter = installGoalRuntimeAdapter(
      testClient.agent.access,
      testClient.runtime,
      getRuntimeClient,
      true,
    );
    const driver = { start, resume } as unknown as RuntimeAgentDriver;
    const mount = () =>
      testClient.renderHook(() =>
        useAgentSession(
          testClient.sessionDependencies,
          getRuntimeClient,
          () => driver,
          SID,
          vi.fn(),
        ),
      );
    const read = () =>
      testClient.agent.access.getAgentSessionSharedMaterial<GoalState>(SID, "goal");
    let mounted = mount();
    try {
      await waitFor(() => expect(read()?.goal?.reason).toEqual(goal.reason));
      mounted.unmount();
      testClient.agent.projection.store.getState().dropSession(SID);
      mounted = mount();
      await waitFor(() => expect(read()?.goal?.reason).toEqual(goal.reason));
      expect(snapshot).toHaveBeenCalledTimes(2);

      snapshot.mockRejectedValueOnce(new Error("Goal snapshot unavailable"));
      await testClient.agent.access.synchronizeMountedAgentSession(SID, "replace-live");
      expect(read()?.goal?.reason).toEqual(goal.reason);
      expect(
        useNotificationStore
          .getState()
          .log.some((notice) => notice.message === t("agent.synchronizationIncomplete")),
      ).toBe(true);

      snapshot.mockResolvedValue({ ...base, goal: { ...goal, reason: { code: "awaitingInput" } } });
      await testClient.agent.access.synchronizeMountedAgentSession(SID, "replace-live");
      expect(read()?.goal?.reason).toEqual({ code: "awaitingInput" });

      const { reason: _reason, ...active } = goal;
      snapshot.mockResolvedValue({ ...base, goal: { ...active, status: "active" } });
      await testClient.agent.access.synchronizeMountedAgentSession(SID, "replace-live");
      expect(read()?.goal?.status).toBe("active");
      expect(read()?.goal).not.toHaveProperty("reason");
      expect(start).not.toHaveBeenCalled();
      expect(resume).not.toHaveBeenCalled();
    } finally {
      mounted.unmount();
      adapter.dispose();
    }
  });

  it("settles Goal edits and admits pause while the active run keeps streaming", async () => {
    const { installGoalRuntimeAdapter } =
      await import("@/features/goal/adapters/runtimeGoalCommandsGateway");
    const { updateGoal, stopGoal } = await import("@/features/goal/application/goalCommands");
    const update = vi.fn().mockResolvedValue({ sessionId: SID });
    const stop = vi.fn().mockResolvedValue({ sessionId: SID });
    const material = {
      items: [],
      runs: [run],
      interrupts: [],
      plan: { sessionId: SID },
    };
    const snapshot = vi.fn().mockResolvedValue(material);
    const closeStream = vi.fn().mockResolvedValue({ done: true });
    const subscribe = vi.fn().mockResolvedValue({
      result: { runId: "run_default", segmentId: "seg_default", snapshot: material },
      events: {
        [Symbol.asyncIterator]: () => ({
          next: () => new Promise<IteratorResult<RunEvent>>(() => {}),
          return: closeStream,
        }),
      },
    });
    runtimeClient = () =>
      ({
        sessions: { snapshot },
        runs: { recoverCommands: async () => [], subscribe },
        goals: { update, stop },
      }) as unknown as FlameClient;
    const adapter = installGoalRuntimeAdapter(
      testClient.agent.access,
      testClient.runtime,
      getRuntimeClient,
      true,
    );
    const driver = { start: vi.fn(), resume: vi.fn() } as unknown as RuntimeAgentDriver;
    const mounted = testClient.renderHook(() =>
      useAgentSession(testClient.sessionDependencies, getRuntimeClient, () => driver, SID, vi.fn()),
    );
    try {
      await waitFor(() => expect(subscribe).toHaveBeenCalledOnce());
      let edited = false;
      const editing = updateGoal({ sessionId: SID, objective: "Finish the Go HTTP client" });
      void editing.then(
        () => {
          edited = true;
        },
        () => {},
      );
      await waitFor(() => expect(edited).toBe(true));
      await editing;
      expect(closeStream).toHaveBeenCalledOnce();
      await waitFor(() => expect(subscribe).toHaveBeenCalledTimes(2));
      expect(
        selectCurrentRootRun(testClient.agent.projection.store.getState().sessions[SID]!.view)
          ?.status,
      ).toBe("running");
      await stopGoal(SID);
      expect(stop).toHaveBeenCalledOnce();
      expect(snapshot).toHaveBeenCalledTimes(3);
    } finally {
      adapter.dispose();
      mounted.unmount();
    }
  });
});
