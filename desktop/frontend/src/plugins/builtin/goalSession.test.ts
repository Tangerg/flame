import { installAgentRuntimeGateway } from "./agent/adapters/agentRuntimeGateway";
import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentDriver } from "@/plugins/sdk/types";
import type { FlameClient, RunEvent, RunRef } from "@flame/runtime-contract/client";
import { useAgentStore } from "./agent/adapters/agentStore";
import { useAgentSessionStore } from "./agent/adapters/agentSessionStore";
import { useAgentSession } from "./agent/adapters/useAgentSession";
import { selectCurrentRootRun } from "./agent/application/view/runTree";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

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
        runs: { subscribe },
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
