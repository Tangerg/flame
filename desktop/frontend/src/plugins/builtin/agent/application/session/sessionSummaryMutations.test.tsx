import { afterEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { queryClient } from "@/lib/queryClient";
import { configureAgentRuntimeGateway, type AgentRuntimeGateway } from "../ports/runtimeGateway";
import { useToggleFavorite } from "./favoriteSession";
import { useRenameSession } from "./renameSession";
import { AGENT_SESSIONS_KEY, type AgentSessionSummary } from "./sessionQueries";
import type { FlameClient, Session } from "@flame/runtime-contract/client";
import { installAgentRuntimeGateway } from "../../adapters/agentRuntimeGateway";

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
  it.each([
    {
      name: "rename",
      run: async () => {
        const { result } = renderHook(() => useRenameSession());
        await result.current("ses_deleted", 3, "after");
      },
    },
    {
      name: "favorite",
      run: async () => {
        const { result } = renderHook(() => useToggleFavorite());
        await result.current("ses_deleted", 3, true);
      },
    },
  ])("revalidates authoritative membership when $name loses a delete race", async ({ run }) => {
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session()]);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    restoreRuntime = configureAgentRuntimeGateway({
      updateSession: vi.fn().mockRejectedValue(new Error("deleted concurrently")),
    } as unknown as AgentRuntimeGateway);

    await run();

    expect(invalidate).toHaveBeenCalledWith({ queryKey: [AGENT_SESSIONS_KEY] });
  });

  it("serializes conditional writes and projects only what the Runtime saved", async () => {
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session()]);
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    const rename = Promise.withResolvers<AgentSessionSummary>();
    const favorite = Promise.withResolvers<AgentSessionSummary>();
    const updateSession = vi
      .fn()
      .mockImplementationOnce(() => rename.promise)
      .mockImplementationOnce(() => favorite.promise);
    restoreRuntime = configureAgentRuntimeGateway({
      updateSession,
    } as unknown as AgentRuntimeGateway);
    const renameHook = renderHook(() => useRenameSession());
    const favoriteHook = renderHook(() => useToggleFavorite());

    let renaming!: Promise<void>;
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

    rename.resolve({ ...session(), title: "after (normalized)", revision: 4 });
    await vi.waitFor(() => expect(updateSession).toHaveBeenCalledTimes(2));
    expect(updateSession).toHaveBeenNthCalledWith(2, {
      sessionId: "ses_deleted",
      expectedRevision: 4,
      favorite: true,
    });
    const saved = { ...session(), title: "after (normalized)", favorite: true, revision: 5 };
    favorite.resolve(saved);
    await act(async () => Promise.all([renaming, favoriting]));

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
    restoreRuntime = configureAgentRuntimeGateway({
      updateSession,
    } as unknown as AgentRuntimeGateway);
    const renameHook = renderHook(() => useRenameSession());
    const favoriteHook = renderHook(() => useToggleFavorite());

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
    restoreRuntime = configureAgentRuntimeGateway({
      updateSession: retiredUpdateCall,
    } as unknown as AgentRuntimeGateway);
    const renameHook = renderHook(() => useRenameSession());
    const favoriteHook = renderHook(() => useToggleFavorite());

    const renaming = renameHook.result.current("ses_deleted", 3, "retired title");
    await vi.waitFor(() => expect(retiredUpdateCall).toHaveBeenCalledTimes(1));

    const successorUpdate = vi
      .fn()
      .mockResolvedValue(runtimeSession({ favorite: true, revision: 4 }));
    runtimeClient = () => ({ sessions: { update: successorUpdate } }) as unknown as FlameClient;
    const disposeSuccessor = installAgentRuntimeGateway(getRuntimeClient);
    try {
      const favoriting = favoriteHook.result.current("ses_deleted", 3, true);
      await vi.waitFor(() => expect(successorUpdate).toHaveBeenCalledTimes(1));
      retiredUpdate.resolve({ ...session(), title: "retired title", revision: 99 });
      await act(async () => Promise.all([renaming, favoriting]));

      expect(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])).toEqual([
        { ...session(), favorite: true, revision: 4 },
      ]);
    } finally {
      disposeSuccessor.dispose();
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
