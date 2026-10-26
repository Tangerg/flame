import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RuntimeConnectionGeneration } from "@/features/connection/public/services";
import {
  invalidateWorkspaceEvent,
  invalidateWorkspaceEverything,
  replaceWorkspaceServerScope,
  retireWorkspaceReadModels,
} from "@/features/workspace/adapters/queryInvalidation";
import { createWorkspaceEventLoop } from "@/features/workspace/application/workspaceEventLoop";
const { cancelQueries, invalidateQueries, resetQueries, synchronizeMountedAgentSessions } =
  vi.hoisted(() => ({
    cancelQueries: vi.fn().mockResolvedValue(undefined),
    invalidateQueries: vi.fn().mockResolvedValue(undefined),
    resetQueries: vi.fn().mockResolvedValue(undefined),
    synchronizeMountedAgentSessions: vi.fn(),
  }));

vi.mock("@/lib/queryClient", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/queryClient")>();
  vi.spyOn(actual.queryClient, "cancelQueries").mockImplementation(cancelQueries);
  vi.spyOn(actual.queryClient, "invalidateQueries").mockImplementation(invalidateQueries);
  vi.spyOn(actual.queryClient, "resetQueries").mockImplementation(resetQueries);
  return actual;
});

vi.mock("@/features/agent/public/session", () => ({ AGENT_SESSIONS_KEY: "agent-sessions" }));
prepareTestClient(() => {
  Object.assign(testClient.agent.access, { synchronizeMountedAgentSessions });
});

vi.mock("@/features/agent/public/approvalPolicy", () => ({
  APPROVAL_MODE_KEY: "approval-mode",
  APPROVAL_RULES_KEY: "approval-rules",
}));

vi.mock("@/features/providers/public/queries", () => ({
  EMBEDDING_ROLE_KEY: "embedding-role",
  MODELS_KEY: "models",
  PROVIDERS_KEY: "providers",
  UTILITY_ROLE_KEY: "utility-role",
}));

beforeEach(() => {
  cancelQueries.mockClear();
  invalidateQueries.mockClear();
  resetQueries.mockClear();
  synchronizeMountedAgentSessions.mockReset();
});

function expectWorkspaceReadScope(options: {
  predicate: (query: { queryKey: readonly unknown[] }) => boolean;
}) {
  const anotherClient = createTestClient({ start: false });
  finishTestClient(() => anotherClient.dispose());
  const anotherOwner = anotherClient.agent.access;
  expect(
    options.predicate({ queryKey: testClient.agent.dependencies.usage.queryKey("ses_shared") }),
  ).toBe(true);
  expect(options.predicate({ queryKey: anotherOwner.sessionUsageQueryKey("ses_shared") })).toBe(
    false,
  );
  expect(options.predicate({ queryKey: ["shared-workspace-read"] })).toBe(true);
}

describe("workspace session projection invalidation", () => {
  it.each(["runs.changed", "interrupts.changed", "plan.changed", "goals.changed"] as const)(
    "targets the mounted sessions named by %s",
    (type) => {
      invalidateWorkspaceEvent(testClient.agent.access, {
        type,
        sequence: 1,
        sessionIds: ["ses_a", "ses_b"],
      });

      expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
        sessionIds: ["ses_a", "ses_b"],
        ownership: "after-live",
      });
    },
  );

  it("refreshes every usage projection after a Run moves", () => {
    invalidateWorkspaceEvent(testClient.agent.access, {
      type: "runs.changed",
      sequence: 1,
      sessionIds: ["ses_a"],
    });

    expect(invalidateQueries.mock.calls.map(([options]) => options.queryKey)).toEqual([
      testClient.agent.dependencies.usage.queryKey(),
    ]);
    expect(cancelQueries.mock.calls).toEqual(invalidateQueries.mock.calls);
  });

  it("synchronizes every mounted session after a lossy resync", () => {
    invalidateWorkspaceEverything(testClient.agent.access);

    expect(cancelQueries).toHaveBeenCalledOnce();
    expect(invalidateQueries).toHaveBeenCalledOnce();
    expectWorkspaceReadScope(invalidateQueries.mock.calls[0]![0]);
    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      ownership: "replace-live",
    });
  });

  it("retires prior Runtime writers without starting successor reads", () => {
    retireWorkspaceReadModels(testClient.agent.access);

    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      ownership: "retire-live",
    });
    expect(cancelQueries).toHaveBeenCalledOnce();
    expectWorkspaceReadScope(cancelQueries.mock.calls[0]![0]);
    expect(invalidateQueries).not.toHaveBeenCalled();
  });

  it("replaces mounted projections and cached reads after the server scope commits", () => {
    replaceWorkspaceServerScope(testClient.agent.access);

    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      ownership: "replace-server",
    });
    expect(resetQueries).toHaveBeenCalledOnce();
    expectWorkspaceReadScope(resetQueries.mock.calls[0]![0]);
  });

  it("refreshes every read affected by external settings mutations", () => {
    for (const type of ["models.changed", "approvals.changed", "plugins.dataChanged"] as const) {
      invalidateWorkspaceEvent(testClient.agent.access, { type, sequence: 1 });
    }

    expect(invalidateQueries.mock.calls.map(([options]) => options.queryKey[0])).toEqual([
      "providers",
      "models",
      "utility-role",
      "embedding-role",
      "approval-mode",
      "approval-rules",
    ]);
    expect(cancelQueries.mock.calls).toEqual(invalidateQueries.mock.calls);
  });

  it("keeps a scoped resync inside the reads named by its topics", () => {
    invalidateWorkspaceEvent(testClient.agent.access, {
      type: "resync",
      sequence: 1,
      topics: ["files.changed"],
    });

    expect(invalidateQueries.mock.calls.map(([options]) => options.queryKey[0])).toEqual([
      "files-changed",
      "list-files",
      "read-file",
      "hooks",
    ]);
    expect(cancelQueries.mock.calls).toEqual(invalidateQueries.mock.calls);
    expect(synchronizeMountedAgentSessions).not.toHaveBeenCalled();
  });

  it("rebuilds Goal through the exact mounted Session material named by the event", () => {
    invalidateWorkspaceEvent(testClient.agent.access, {
      type: "goals.changed",
      sequence: 1,
      sessionIds: ["ses_a", "ses_b"],
    });

    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      sessionIds: ["ses_a", "ses_b"],
      ownership: "after-live",
    });
    expect(cancelQueries).not.toHaveBeenCalled();
    expect(invalidateQueries).not.toHaveBeenCalled();
  });

  it("uses one Session writer when a scoped resync names Goal with other material", () => {
    invalidateWorkspaceEvent(testClient.agent.access, {
      type: "resync",
      sequence: 1,
      topics: ["runs.changed", "goals.changed"],
      sessionIds: ["ses_mounted", "ses_unmounted"],
    });

    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      sessionIds: ["ses_mounted", "ses_unmounted"],
      ownership: "after-live",
    });
    expect(synchronizeMountedAgentSessions).toHaveBeenCalledOnce();
    expect(cancelQueries).toHaveBeenCalledOnce();
    expect(invalidateQueries).toHaveBeenCalledOnce();
  });

  it("keeps Goal and mounted HITL/Plan/Run/Tool on one monotonic recovery boundary", async () => {
    const controller = new AbortController();
    let receivedLatest!: () => void;
    const latest = new Promise<void>((resolve) => {
      receivedLatest = resolve;
    });
    const loop = createWorkspaceEventLoop({
      async subscribe({ signal }) {
        return (async function* () {
          yield { type: "goals.changed", sequence: 1, sessionIds: ["ses_a"] } as const;
          yield { type: "runs.changed", sequence: 3, sessionIds: ["ses_a"] } as const;
          yield { type: "interrupts.changed", sequence: 2, sessionIds: ["ses_a"] } as const;
          yield { type: "plan.changed", sequence: 4, sessionIds: ["ses_a"] } as const;
          await new Promise<void>((resolve) => {
            signal.addEventListener("abort", () => resolve(), { once: true });
          });
        })();
      },
      handleEvent(event) {
        invalidateWorkspaceEvent(testClient.agent.access, event);
        if (event.sequence === 4) receivedLatest();
      },
      invalidateAll: () => invalidateWorkspaceEverything(testClient.agent.access),
      reportDisconnect: vi.fn(),
    });

    const run = loop.start(
      controller.signal,
      RuntimeConnectionGeneration.forProcess("connection_1"),
    );
    await latest;
    controller.abort();
    await run;

    expect(synchronizeMountedAgentSessions.mock.calls).toEqual([
      [{ ownership: "replace-live" }],
      [{ sessionIds: ["ses_a"], ownership: "after-live" }],
      [{ ownership: "replace-live" }],
      [{ sessionIds: ["ses_a"], ownership: "after-live" }],
      [{ sessionIds: ["ses_a"], ownership: "after-live" }],
    ]);
    const fullRefreshes = invalidateQueries.mock.calls.filter(([options]) => !options.queryKey);
    expect(fullRefreshes).toHaveLength(2);
    for (const [options] of fullRefreshes) expectWorkspaceReadScope(options);
    expect(cancelQueries.mock.calls).toEqual(invalidateQueries.mock.calls);
    expect(
      invalidateQueries.mock.calls.some(([options]) => options?.queryKey?.[0] === "pending-work"),
    ).toBe(false);
  });
});
