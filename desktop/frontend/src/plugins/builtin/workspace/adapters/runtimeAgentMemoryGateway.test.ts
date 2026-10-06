import { afterEach, describe, expect, it, vi } from "vitest";
import { validateWire, type WireTypeName } from "@flame/runtime-contract/validate";
import type { FlameClient } from "@flame/runtime-contract/client";
import { queryClient } from "@/lib/queryClient";
import {
  addAgentMemory,
  agentMemoryQuery,
  setAgentMemoryPinned,
} from "../application/agentMemoryConfig";
import { WORKSPACE_AGENT_MEMORY_KEY, type AgentMemoryEntry } from "../application/workspaceQueries";
import { installAgentMemoryGateway } from "./runtimeAgentMemoryGateway";
import { rejected } from "@/test/rejected";

const MEMORY_ID = "mem_0123456789abcdef0123456789abcdef";

let uninstall: (() => void) | undefined;

function expectSendable(shape: WireTypeName, call: ReturnType<typeof vi.fn>): void {
  expect(validateWire(shape, call.mock.calls[0]?.[0])).toEqual([]);
}

afterEach(() => {
  uninstall?.();
  uninstall = undefined;

  queryClient.removeQueries({ queryKey: [WORKSPACE_AGENT_MEMORY_KEY] });
});

describe("runtimeAgentMemoryGateway", () => {
  it("captures a replacement client at the Runtime generation boundary", async () => {
    const response = Promise.withResolvers<ReturnType<typeof memoryItem>>();
    const retiredUpdate = vi.fn(() => response.promise);
    const successorUpdate = vi.fn().mockResolvedValue(memoryItem({ pinned: true }));
    let client = { agentMemory: { update: retiredUpdate } } as unknown as FlameClient;
    const installation = installAgentMemoryGateway(() => client);
    uninstall = installation.dispose;

    const retired = rejected(setAgentMemoryPinned(MEMORY_ID, true));
    await vi.waitFor(() => expect(retiredUpdate).toHaveBeenCalledOnce());
    client = { agentMemory: { update: successorUpdate } } as unknown as FlameClient;
    installation.replaceRuntimeGeneration();
    await expect(retired).resolves.toMatchObject({
      message: "agent_memory_mutation_generation_retired",
    });
    await setAgentMemoryPinned(MEMORY_ID, true);
    expect(successorUpdate).toHaveBeenCalledExactlyOnceWith({ id: MEMORY_ID, pinned: true });
    expect(retiredUpdate).toHaveBeenCalledOnce();
    response.resolve(memoryItem());
  });

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
    const runtimeClient = () => ({ agentMemory: { add, update } }) as unknown as FlameClient;
    uninstall = installAgentMemoryGateway(() => runtimeClient()).dispose;

    await expect(addAgentMemory({ scope: "user", content: item.content })).resolves.toMatchObject({
      id: MEMORY_ID,
      scope: "user",
    });
    await expect(setAgentMemoryPinned(MEMORY_ID, true)).resolves.toBeUndefined();
    expect(update).toHaveBeenCalledWith({ id: MEMORY_ID, pinned: true });

    expectSendable("AgentMemoryAddRequest", add);
    expectSendable("AgentMemoryUpdateRequest", update);
  });

  it("retires in-flight and queued commands before a successor gateway is installed", async () => {
    const query = agentMemoryQuery("user");
    const retiredUpdate = Promise.withResolvers<ReturnType<typeof memoryItem>>();
    const updateRetired = vi.fn(() => retiredUpdate.promise);
    const updateSuccessor = vi
      .fn()
      .mockResolvedValue(
        memoryItem({ content: "successor", pinned: false, updatedAt: "2026-08-17T12:00:02Z" }),
      );
    let runtimeClient = () =>
      ({ agentMemory: { update: updateRetired } }) as unknown as FlameClient;
    const retiredInstallation = installAgentMemoryGateway(() => runtimeClient());
    queryClient.setQueryData([WORKSPACE_AGENT_MEMORY_KEY, query], [memoryEntry()]);

    const inFlight = setAgentMemoryPinned(MEMORY_ID, true);
    const queued = setAgentMemoryPinned(MEMORY_ID, false);
    const inFlightSettlement = rejected(inFlight);
    const queuedSettlement = rejected(queued);
    await vi.waitFor(() => expect(updateRetired).toHaveBeenCalledOnce());

    runtimeClient = () => ({ agentMemory: { update: updateSuccessor } }) as unknown as FlameClient;
    const successorInstallation = installAgentMemoryGateway(() => runtimeClient());
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
    ...overrides,
  };
}
