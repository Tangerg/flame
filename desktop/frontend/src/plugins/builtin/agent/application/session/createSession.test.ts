import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import { navigator } from "@/lib/navigation";
import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import type { FlameClient, Methods } from "@flame/runtime-contract/client";
import { asSessionId } from "@flame/runtime-contract/client";
import { useAgentSessionStore } from "@/plugins/builtin/agent/adapters/agentSessionStore";
import { useAgentStore } from "@/plugins/builtin/agent/adapters/agentStore";
import { installAgentRuntimeGateway } from "@/plugins/builtin/agent/adapters/agentRuntimeGateway";
import { createSession, type CreateSessionOptions, useCreateSession } from "./createSession";
import { AGENT_SESSIONS_KEY, type AgentSessionSummary } from "./sessionQueries";

let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};
const getRuntimeClient = () => runtimeClient();

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

function stubCreate(create: Methods["sessions"]["create"]) {
  runtimeClient = () => ({ sessions: { create } }) as unknown as FlameClient;
}

afterEach(() => {
  queryClient.clear();
  navigator().go({ session: "" });
  useAgentSessionStore.setState({
    openSessionIds: [],
    lastSessionId: "",
  });
});

const fakeSession = (id: string, path = "/tmp/proj") => ({
  id: asSessionId(id),
  revision: 1,
  title: "",
  status: "idle" as const,
  provider: "openai",
  model: "gpt-4o",
  workspace: { ref: { path }, availability: "available" as const },
  createdAt: "2026-08-20T00:00:00Z",
  updatedAt: "2026-08-20T00:00:00Z",
});

function summary(id: string, cwd: string): AgentSessionSummary {
  return {
    id,
    revision: 1,
    title: "Current",
    status: "idle",
    provider: "openai",
    model: "gpt-5",
    workspace: { path: cwd, availability: "available" },
    time: "2026-08-20T00:00:00Z",
  };
}

describe("useCreateSession", () => {
  it("creates a Session in the chosen exact directory, opens it and lists what the Runtime created", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("new-cwd"));
    stubCreate(create);
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [summary("existing", "/tmp/proj")]);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    const id = await result.current({ cwd: "/tmp/proj" });

    expect(id).toBe("new-cwd");
    expect(create).toHaveBeenCalledWith(
      { workspace: { path: "/tmp/proj" } },
      expect.any(AbortSignal),
    );
    expect(navigator().get().session).toBe("new-cwd");
    expect(useAgentSessionStore.getState().openSessionIds).toContain("new-cwd");
    expect(
      queryClient
        .getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])
        ?.map((session) => session.id),
    ).toEqual(["new-cwd", "existing"]);
  });

  it("never delegates an empty working directory to the Runtime default", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("implicit-home"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await expect(result.current({ cwd: "" })).resolves.toBeNull();

    expect(create).not.toHaveBeenCalled();
    expect(navigator().get().session).toBe("");
  });

  it("reuses the active Session only while its loaded view holds no messages", async () => {
    const create = vi
      .fn()
      .mockResolvedValueOnce(fakeSession("new-1"))
      .mockResolvedValueOnce(fakeSession("new-2"))
      .mockResolvedValueOnce(fakeSession("new-3"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });
    const destination = {
      cwd: "/tmp/current-project",
      reuseEmptySession: true,
    } satisfies CreateSessionOptions;

    const first = await result.current(destination);
    const unloaded = await result.current(destination);
    useAgentStore.getState().ensureSession(unloaded!);
    const reused = await result.current(destination);
    const explicit = await result.current({ cwd: "/tmp/other" });

    expect(first).toBe("new-1");
    expect(unloaded).toBe("new-2");
    expect(reused).toBe("new-2");
    expect(explicit).toBe("new-3");
    expect(create).toHaveBeenCalledTimes(3);
  });

  it("joins only an in-flight create for the same exact cwd", async () => {
    let release: ((session: ReturnType<typeof fakeSession>) => void) | undefined;
    const create = vi
      .fn()
      .mockImplementationOnce(
        () => new Promise<ReturnType<typeof fakeSession>>((resolve) => (release = resolve)),
      )
      .mockResolvedValue(fakeSession("second"));
    stubCreate(create as unknown as Methods["sessions"]["create"]);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    const first = result.current({ cwd: "/tmp/a" });
    const joined = result.current({ cwd: "/tmp/a" });
    const distinct = result.current({ cwd: "/tmp/b" });
    release?.(fakeSession("first"));

    expect(await first).toBe("first");
    expect(await joined).toBe("first");
    expect(await distinct).toBe("second");
    expect(create).toHaveBeenCalledTimes(2);
  });

  it("returns null without moving selection when create fails", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    stubCreate(vi.fn().mockRejectedValue(new Error("boom")));
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await expect(result.current({ cwd: "/tmp/project" })).resolves.toBeNull();
    expect(navigator().get().session).toBe("");
  });

  it("does not join or publish a create owned by a replaced Plugin Host", async () => {
    let releaseRetired!: (value: ReturnType<typeof fakeSession>) => void;
    const retiredCreate = vi.fn(
      () =>
        new Promise<ReturnType<typeof fakeSession>>((resolve) => {
          releaseRetired = resolve;
        }),
    );
    stubCreate(retiredCreate as unknown as Methods["sessions"]["create"]);
    const { result } = renderHook(() => useCreateSession(), { wrapper });
    const retired = result.current({ cwd: "/tmp/retired" });

    const successorCreate = vi.fn().mockResolvedValue(fakeSession("successor"));
    stubCreate(successorCreate);
    const disposeSuccessor = installAgentRuntimeGateway(getRuntimeClient);
    const successor = result.current({ cwd: "/tmp/successor" });

    await Promise.resolve();
    const successorStartedBeforeRetiredSettlement = successorCreate.mock.calls.length;
    let retiredSettled = false;
    void retired.then(() => {
      retiredSettled = true;
    });
    await flushMicrotasks();
    const retiredSettledBeforeOldRPC = retiredSettled;
    releaseRetired(fakeSession("retired"));
    try {
      await expect(retired).resolves.toBeNull();
      await expect(successor).resolves.toBe("successor");
      expect(successorStartedBeforeRetiredSettlement).toBe(1);
      expect(retiredSettledBeforeOldRPC).toBe(true);
      expect(navigator().get().session).toBe("successor");
    } finally {
      disposeSuccessor.dispose();
    }
  });
});

describe("imperative New", () => {
  it("inherits the exact active Session cwd", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("next"));
    stubCreate(create);
    navigator().go({ session: "current" });
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [summary("current", "/tmp/current")]);

    await expect(createSession()).resolves.toBe("next");

    expect(create).toHaveBeenCalledWith(
      { workspace: { path: "/tmp/current" } },
      expect.any(AbortSignal),
    );
  });

  it("does not mutate when no active Session or authoritative cwd exists", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("implicit-home"));
    stubCreate(create);

    await expect(createSession()).resolves.toBeNull();
    navigator().go({ session: "unresolved" });
    await expect(createSession()).resolves.toBeNull();

    expect(create).not.toHaveBeenCalled();
  });
});

async function flushMicrotasks(): Promise<void> {
  for (let index = 0; index < 8; index += 1) await Promise.resolve();
}

beforeEach(() => {
  installAgentRuntimeGateway(getRuntimeClient);
});
