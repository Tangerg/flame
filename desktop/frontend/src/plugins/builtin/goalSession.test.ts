import { installAgentRuntimeGateway } from "./agent/adapters/agentRuntimeGateway";
import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentDriver } from "@/plugins/sdk/types";
import type {
  FlameClient,
  RunEvent,
  RunRef,
  ServerCapabilities,
} from "@flame/runtime-contract/client";
import { useAgentStore } from "./agent/adapters/agentStore";
import { useAgentSessionStore } from "./agent/adapters/agentSessionStore";
import { useAgentSession } from "./agent/adapters/useAgentSession";
import { selectCurrentRootRun } from "./agent/application/view/runTree";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { useRuntimeConnectionStore } from "./runtime/adapters/runtimeConnectionProjection";
import { synchronizeMountedAgentSession } from "./agent/public/session";
import { getAgentSessionSharedMaterial } from "./agent/public/sessionMaterial";
import type { GoalState } from "./chat/goal/application/goalReadModel";
import discovery from "@flame/runtime-contract/samples/method.discover.resp.json";
import { useNotificationStore } from "@/plugins/sdk/notifications";
import { t } from "@/lib/i18n";

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
  installAgentRuntimeGateway(getRuntimeClient);
});
afterEach(async () => {
  useAgentStore.getState().dropSession(SID);
  useAgentSessionStore.setState({ openSessionIds: [], lastSessionId: "" });
});

describe("Goal and mounted Session integration", () => {
  it("retains the owner stop reason across cold remount and refreshes it from the next snapshot", async () => {
    const { installGoalRuntimeAdapter } =
      await import("@/plugins/builtin/chat/goal/adapters/runtimeGoalCommandsGateway");
    const capabilities = discovery.capabilities as ServerCapabilities;
    useRuntimeConnectionStore.setState({
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
    const adapter = installGoalRuntimeAdapter(getRuntimeClient, true);
    const driver = { start, resume } as unknown as AgentDriver;
    const mount = () =>
      renderHook(() => useAgentSession(getRuntimeClient, () => driver, SID, vi.fn()));
    const read = () => getAgentSessionSharedMaterial<GoalState>(SID, "goal");
    let mounted = mount();
    try {
      await waitFor(() => expect(read()?.goal?.reason).toEqual(goal.reason));
      mounted.unmount();
      useAgentStore.getState().dropSession(SID);
      mounted = mount();
      await waitFor(() => expect(read()?.goal?.reason).toEqual(goal.reason));
      expect(snapshot).toHaveBeenCalledTimes(2);

      snapshot.mockRejectedValueOnce(new Error("Goal snapshot unavailable"));
      await synchronizeMountedAgentSession(SID, "replace-live");
      expect(read()?.goal?.reason).toEqual(goal.reason);
      expect(
        useNotificationStore
          .getState()
          .log.some((notice) => notice.message === t("agent.synchronizationIncomplete")),
      ).toBe(true);

      snapshot.mockResolvedValue({ ...base, goal: { ...goal, reason: { code: "awaitingInput" } } });
      await synchronizeMountedAgentSession(SID, "replace-live");
      expect(read()?.goal?.reason).toEqual({ code: "awaitingInput" });

      const { reason: _reason, ...active } = goal;
      snapshot.mockResolvedValue({ ...base, goal: { ...active, status: "active" } });
      await synchronizeMountedAgentSession(SID, "replace-live");
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
      await import("@/plugins/builtin/chat/goal/adapters/runtimeGoalCommandsGateway");
    const { updateGoal, stopGoal } =
      await import("@/plugins/builtin/chat/goal/application/goalCommands");
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
    const adapter = installGoalRuntimeAdapter(getRuntimeClient, true);
    const driver = { start: vi.fn(), resume: vi.fn() } as unknown as AgentDriver;
    const mounted = renderHook(() => useAgentSession(getRuntimeClient, () => driver, SID, vi.fn()));
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
      expect(selectCurrentRootRun(useAgentStore.getState().sessions[SID]!.view)?.status).toBe(
        "running",
      );
      await stopGoal(SID);
      expect(stop).toHaveBeenCalledOnce();
      expect(snapshot).toHaveBeenCalledTimes(3);
    } finally {
      adapter.dispose();
      mounted.unmount();
    }
  });
});
