import { stubInstance } from "@/test/stubInstance";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient({ runtimeClient: getRuntimeClient });
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, describe, expect, it, vi } from "vitest";
import { act } from "@testing-library/react";
import { queryClient } from "@/lib/queryClient";
import { type AgentRuntimeGateway } from "@/features/agent/application/runtimeGateway";
import { useToggleFavorite } from "@/features/agent/application/session/favoriteSession";
import { useRenameSession } from "@/features/agent/application/session/renameSession";
import {
  AGENT_SESSIONS_KEY,
  type AgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";
import { RpcError, type FlameClient, type Session } from "@flame/runtime-client";
let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};

const getRuntimeClient = () => runtimeClient();

let restoreRuntime: (() => void) | undefined;

function session(): AgentSessionSummary {
  return {
    id: "ses_deleted",
    revision: 3,
    title: "before",
    status: "idle",
    provider: "openai",
    model: "gpt-5",
    workspace: { path: "/repo", availability: "available" },
    time: "2026-08-12T00:00:00Z",
  };
}

afterEach(() => {
  restoreRuntime?.();
  restoreRuntime = undefined;
  vi.restoreAllMocks();
  queryClient.removeQueries({ queryKey: [AGENT_SESSIONS_KEY] });
});

describe("Session summary mutations", () => {
  it("preserves the captured revision when the cache has advanced before rename submission", async () => {
    queryClient.setQueryData(
      [AGENT_SESSIONS_KEY],
      [{ ...session(), revision: 4, title: "Remote title" }],
    );
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const updateSession = vi
      .fn()
      .mockRejectedValue(
        new RpcError({ message: "revision conflict", data: { type: "revision_conflict" } }),
      );
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      updateSession,
    } as unknown as AgentRuntimeGateway);
    const { result } = testClient.renderHook(() => useRenameSession());
    let saved: boolean | undefined;
    await act(async () => {
      saved = await result.current("ses_deleted", 3, "Local draft");
    });
    expect(saved).toBe(false);
    expect(updateSession).toHaveBeenCalledExactlyOnceWith({
      sessionId: "ses_deleted",
      expectedRevision: 3,
      title: "Local draft",
    });
    expect(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])).toEqual([
      { ...session(), revision: 4, title: "Remote title" },
    ]);
  });

  it.each([
    {
      name: "rename",
      run: async () => {
        const { result } = testClient.renderHook(() => useRenameSession());
        await result.current("ses_deleted", 3, "after");
      },
    },
    {
      name: "favorite",
      run: async () => {
        const { result } = testClient.renderHook(() => useToggleFavorite());
        await result.current("ses_deleted", 3, true);
      },
    },
  ])("revalidates authoritative membership when $name loses a delete race", async ({ run }) => {
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session()]);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      updateSession: vi.fn().mockRejectedValue(new Error("deleted concurrently")),
    } as unknown as AgentRuntimeGateway);

    await run();

    expect(invalidate).toHaveBeenCalledWith({ queryKey: [AGENT_SESSIONS_KEY] });
  });

  it("serializes captured preconditions without rebasing a later command", async () => {
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session()]);
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const rename = Promise.withResolvers<AgentSessionSummary>();
    const updateSession = vi
      .fn()
      .mockImplementationOnce(() => rename.promise)
      .mockRejectedValueOnce(new Error("revision conflict"));
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      updateSession,
    } as unknown as AgentRuntimeGateway);
    const renameHook = testClient.renderHook(() => useRenameSession());
    const favoriteHook = testClient.renderHook(() => useToggleFavorite());
    let renaming!: Promise<boolean>;
    let favoriting!: Promise<void>;
    await act(async () => {
      renaming = renameHook.result.current("ses_deleted", 3, "after");
      favoriting = favoriteHook.result.current("ses_deleted", 3, true);
      await vi.waitFor(() => expect(updateSession).toHaveBeenCalledTimes(1));
    });
    expect(updateSession).toHaveBeenNthCalledWith(1, {
      sessionId: "ses_deleted",
      expectedRevision: 3,
      title: "after",
    });
    expect(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])).toEqual([
      session(),
    ]);
    const normalized = { ...session(), title: "after (normalized)", revision: 4 };
    await act(async () => {
      rename.resolve(normalized);
      await Promise.all([renaming, favoriting]);
    });
    expect(updateSession).toHaveBeenNthCalledWith(2, {
      sessionId: "ses_deleted",
      expectedRevision: 3,
      favorite: true,
    });
    expect(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])).toEqual([
      normalized,
    ]);
    expect(await renaming).toBe(true);
    const saved = { ...normalized, favorite: true, revision: 5 };
    updateSession.mockResolvedValueOnce(saved);
    await act(async () => {
      await favoriteHook.result.current("ses_deleted", normalized.revision, true);
    });
    expect(updateSession).toHaveBeenNthCalledWith(3, {
      sessionId: "ses_deleted",
      expectedRevision: normalized.revision,
      favorite: true,
    });
    expect(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])).toEqual([saved]);
  });

  it("leaves the saved summary in place when a following write is refused", async () => {
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session()]);
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const renamed = { ...session(), title: "after", revision: 4 };
    const updateSession = vi
      .fn()
      .mockResolvedValueOnce(renamed)
      .mockRejectedValueOnce(new Error("remote writer won"));
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      updateSession,
    } as unknown as AgentRuntimeGateway);
    const renameHook = testClient.renderHook(() => useRenameSession());
    const favoriteHook = testClient.renderHook(() => useToggleFavorite());

    const renaming = renameHook.result.current("ses_deleted", 3, "after");
    const favoriting = favoriteHook.result.current("ses_deleted", 3, true);
    await act(async () => Promise.all([renaming, favoriting]));

    expect(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])).toEqual([
      renamed,
    ]);
  });

  it("drops a retired generation's late result and starts the successor queue independently", async () => {
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session()]);
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    const retiredUpdate = Promise.withResolvers<AgentSessionSummary>();
    const retiredUpdateCall = vi.fn(() => retiredUpdate.promise);
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      updateSession: retiredUpdateCall,
    } as unknown as AgentRuntimeGateway);
    const renameHook = testClient.renderHook(() => useRenameSession());
    const favoriteHook = testClient.renderHook(() => useToggleFavorite());

    const renaming = renameHook.result.current("ses_deleted", 3, "retired title");
    await vi.waitFor(() => expect(retiredUpdateCall).toHaveBeenCalledTimes(1));

    const successorUpdate = vi
      .fn()
      .mockResolvedValue(runtimeSession({ favorite: true, revision: 4 }));
    runtimeClient = () => ({ sessions: { update: successorUpdate } }) as unknown as FlameClient;
    restoreRuntime();
    restoreRuntime = undefined;
    testClient.agent.replaceRuntimeGeneration();
    try {
      const favoriting = favoriteHook.result.current("ses_deleted", 3, true);
      await vi.waitFor(() => expect(successorUpdate).toHaveBeenCalledTimes(1));
      retiredUpdate.resolve({ ...session(), title: "retired title", revision: 99 });
      await act(async () => Promise.all([renaming, favoriting]));

      expect(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])).toEqual([
        { ...session(), favorite: true, revision: 4 },
      ]);
    } finally {
      testClient.agent.dispose();
    }
  });
});

function runtimeSession(change: { favorite?: boolean; revision: number }): Session {
  const summary = session();
  return {
    id: summary.id,
    revision: change.revision,
    title: summary.title,
    status: summary.status,
    provider: summary.provider,
    model: summary.model,
    workspace: {
      ref: { path: summary.workspace.path },
      availability: summary.workspace.availability,
    },
    ...(change.favorite !== undefined ? { favorite: change.favorite } : {}),
    createdAt: summary.time,
    updatedAt: summary.time,
  } as Session;
}
