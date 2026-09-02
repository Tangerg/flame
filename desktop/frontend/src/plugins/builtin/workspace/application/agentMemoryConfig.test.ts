import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import {
  addAgentMemory,
  agentMemoryQuery,
  deleteAgentMemory,
  setAgentMemoryPinned,
} from "./agentMemoryConfig";
import type { AgentMemoryGateway } from "./ports/agentMemoryGateway";
import { WORKSPACE_AGENT_MEMORY_KEY, type AgentMemoryEntry } from "./workspaceQueries";
import { AgentMemoryMutationOwner } from "./agentMemoryMutationOwner";

const MEMORY_ID = "mem_0123456789abcdef0123456789abcdef";

function memory(overrides: Partial<AgentMemoryEntry> = {}): AgentMemoryEntry {
  return {
    id: MEMORY_ID,
    scope: "user",
    content: "Remember this",
    origin: "user",
    status: "active",
    pinned: false,
    sessionId: "",
    day: "",
    createdAt: "2026-08-12T12:00:00Z",
    updatedAt: "2026-08-12T12:00:00Z",
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

let uninstall: (() => void) | undefined;

function installGateway(gateway: AgentMemoryGateway): void {
  const owner = AgentMemoryMutationOwner.install(gateway);
  uninstall = () => owner.dispose();
}

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  queryClient.removeQueries({ queryKey: [WORKSPACE_AGENT_MEMORY_KEY] });
});

describe("agent memory configuration", () => {
  it("canonicalizes user memory independently of the active workspace", () => {
    expect(agentMemoryQuery("user", "/repo-a")).toEqual({ scope: "user" });
    expect(agentMemoryQuery("user", "/repo-b")).toEqual({ scope: "user" });
    expect(agentMemoryQuery("project", "/repo-a")).toEqual({
      scope: "project",
      cwd: "/repo-a",
    });
  });

  it("commits the returned item into the exact scope cache", async () => {
    const query = agentMemoryQuery("user", "/ignored");
    queryClient.setQueryData([WORKSPACE_AGENT_MEMORY_KEY, query], []);
    const saved = memory();
    installGateway({
      add: vi.fn().mockResolvedValue(saved),
    } as unknown as AgentMemoryGateway);

    await expect(
      addAgentMemory({ scope: "user", cwd: "/ignored", content: saved.content }),
    ).resolves.toEqual(saved);
    expect(queryClient.getQueryData([WORKSPACE_AGENT_MEMORY_KEY, query])).toEqual([saved]);
  });

  it("serializes updates to one item and commits the last returned fact", async () => {
    const query = agentMemoryQuery("user");
    queryClient.setQueryData([WORKSPACE_AGENT_MEMORY_KEY, query], [memory()]);
    const first = deferred<AgentMemoryEntry>();
    const second = deferred<AgentMemoryEntry>();
    const setPinned = vi
      .fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    installGateway({ setPinned } as unknown as AgentMemoryGateway);

    const pinned = setAgentMemoryPinned(MEMORY_ID, true);
    const unpinned = setAgentMemoryPinned(MEMORY_ID, false);
    await vi.waitFor(() => expect(setPinned).toHaveBeenCalledTimes(1));

    first.resolve(memory({ pinned: true, updatedAt: "2026-08-12T12:00:01Z" }));
    await pinned;
    await Promise.resolve();
    expect(setPinned).toHaveBeenNthCalledWith(2, MEMORY_ID, false);

    second.resolve(memory({ pinned: false, updatedAt: "2026-08-12T12:00:02Z" }));
    await unpinned;
    expect(
      queryClient.getQueryData<AgentMemoryEntry[]>([WORKSPACE_AGENT_MEMORY_KEY, query])?.[0],
    ).toMatchObject({ pinned: false, updatedAt: "2026-08-12T12:00:02Z" });
  });

  it("orders deletion after an in-flight update to the same item", async () => {
    const query = agentMemoryQuery("user");
    queryClient.setQueryData([WORKSPACE_AGENT_MEMORY_KEY, query], [memory()]);
    const first = deferred<AgentMemoryEntry>();
    const setPinned = vi.fn().mockReturnValue(first.promise);
    const remove = vi.fn().mockResolvedValue(undefined);
    installGateway({
      setPinned,
      delete: remove,
    } as unknown as AgentMemoryGateway);

    const pinned = setAgentMemoryPinned(MEMORY_ID, true);
    const deleted = deleteAgentMemory(MEMORY_ID);
    await vi.waitFor(() => expect(setPinned).toHaveBeenCalledOnce());
    expect(remove).not.toHaveBeenCalled();

    first.resolve(memory({ pinned: true, updatedAt: "2026-08-12T12:00:01Z" }));
    await pinned;
    await deleted;
    expect(remove).toHaveBeenCalledWith(MEMORY_ID);
    expect(queryClient.getQueryData([WORKSPACE_AGENT_MEMORY_KEY, query])).toEqual([]);
  });
});
