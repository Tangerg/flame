import { afterEach, describe, expect, it, vi } from "vitest";
import { validateWire, type WireTypeName } from "@flame/runtime-contract/validate";
import { resetContainer, setContainer } from "@/main/container";
import type { FlameClient } from "@/rpc";
import { queryClient } from "@/lib/queryClient";
import {
  addAgentMemory,
  agentMemoryQuery,
  setAgentMemoryPinned,
} from "../application/agentMemoryConfig";
import { WORKSPACE_AGENT_MEMORY_KEY, type AgentMemoryEntry } from "../application/workspaceQueries";
import { installAgentMemoryGateway } from "./runtimeAgentMemoryGateway";

const MEMORY_ID = "mem_0123456789abcdef0123456789abcdef";

let uninstall: (() => void) | undefined;

function expectSendable(shape: WireTypeName, call: ReturnType<typeof vi.fn>): void {
  expect(validateWire(shape, call.mock.calls[0]?.[0])).toEqual([]);
}

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
  queryClient.removeQueries({ queryKey: [WORKSPACE_AGENT_MEMORY_KEY] });
});

describe("runtimeAgentMemoryGateway", () => {
  it("maps returned add and update items into the workspace language", async () => {
    const item = {
      id: MEMORY_ID,
      scope: "user",
      content: "Remember this",
      origin: "user",
      status: "active",
      pinned: false,
      createdAt: "2026-08-12T12:00:00Z",
      updatedAt: "2026-08-12T12:00:00Z",
    };
    const add = vi.fn().mockResolvedValue(item);
    const update = vi.fn().mockResolvedValue({
      ...item,
      pinned: true,
      updatedAt: "2026-08-12T12:00:01Z",
    });
    setContainer({
      client: () => ({ agentMemory: { add, update } }) as unknown as FlameClient,
    });
    uninstall = installAgentMemoryGateway().dispose;

    await expect(addAgentMemory({ scope: "user", content: item.content })).resolves.toMatchObject({
      id: MEMORY_ID,
      scope: "user",
      sessionId: "",
      day: "",
    });
    await expect(setAgentMemoryPinned(MEMORY_ID, true)).resolves.toBeUndefined();
    expect(update).toHaveBeenCalledWith({ id: MEMORY_ID, pinned: true });

    // A pin is the one update that carries no content, and the Runtime accepts it only
    // because it asks for content *or* pinned. Nothing in the TypeScript says so.
    expectSendable("AgentMemoryAddRequest", add);
    expectSendable("AgentMemoryUpdateRequest", update);
  });

  it("retires in-flight and queued commands before a successor gateway is installed", async () => {
    const query = agentMemoryQuery("user");
    const retiredUpdate = deferred<ReturnType<typeof memoryItem>>();
    const updateRetired = vi.fn(() => retiredUpdate.promise);
    const updateSuccessor = vi
      .fn()
      .mockResolvedValue(
        memoryItem({ content: "successor", pinned: false, updatedAt: "2026-08-17T12:00:02Z" }),
      );
    setContainer({
      client: () => ({ agentMemory: { update: updateRetired } }) as unknown as FlameClient,
    });
    const retiredInstallation = installAgentMemoryGateway();
    queryClient.setQueryData([WORKSPACE_AGENT_MEMORY_KEY, query], [memoryEntry()]);

    const inFlight = setAgentMemoryPinned(MEMORY_ID, true);
    const queued = setAgentMemoryPinned(MEMORY_ID, false);
    const inFlightSettlement = rejected(inFlight);
    const queuedSettlement = rejected(queued);
    await vi.waitFor(() => expect(updateRetired).toHaveBeenCalledOnce());

    setContainer({
      client: () => ({ agentMemory: { update: updateSuccessor } }) as unknown as FlameClient,
    });
    const successorInstallation = installAgentMemoryGateway();
    uninstall = () => {
      successorInstallation.dispose();
      retiredInstallation.dispose();
    };
    queryClient.setQueryData(
      [WORKSPACE_AGENT_MEMORY_KEY, query],
      [memoryEntry({ content: "successor", pinned: false })],
    );

    retiredUpdate.resolve(
      memoryItem({ content: "retired", pinned: true, updatedAt: "2026-08-17T12:00:01Z" }),
    );

    await expect(inFlightSettlement).resolves.toMatchObject({
      message: "agent_memory_mutation_generation_retired",
    });
    await expect(queuedSettlement).resolves.toMatchObject({
      message: "agent_memory_mutation_generation_retired",
    });
    expect(updateSuccessor).not.toHaveBeenCalled();
    expect(
      queryClient.getQueryData<AgentMemoryEntry[]>([WORKSPACE_AGENT_MEMORY_KEY, query]),
    ).toEqual([memoryEntry({ content: "successor", pinned: false })]);

    const successorCommand = setAgentMemoryPinned(MEMORY_ID, false);
    retiredInstallation.replaceRuntimeGeneration();
    await expect(successorCommand).resolves.toBeUndefined();
    expect(updateSuccessor).toHaveBeenCalledOnce();
  });
});

function memoryItem(overrides: Record<string, unknown> = {}) {
  return {
    id: MEMORY_ID,
    scope: "user" as const,
    content: "Remember this",
    origin: "user" as const,
    status: "active" as const,
    pinned: false,
    createdAt: "2026-08-17T12:00:00Z",
    updatedAt: "2026-08-17T12:00:00Z",
    ...overrides,
  };
}

function memoryEntry(overrides: Partial<AgentMemoryEntry> = {}): AgentMemoryEntry {
  return {
    ...memoryItem(),
    sessionId: "",
    day: "",
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

function rejected(operation: Promise<unknown>): Promise<Error> {
  return operation.then(
    () => {
      throw new Error("operation unexpectedly resolved");
    },
    (error: unknown) => (error instanceof Error ? error : new Error(String(error))),
  );
}
