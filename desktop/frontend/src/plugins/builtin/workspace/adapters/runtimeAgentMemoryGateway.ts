import type { AgentMemoryGateway } from "../application/ports/agentMemoryGateway";
import type { AgentMemoryItem, FlameClient } from "@flame/runtime-contract/client";
import type { AgentMemoryEntry } from "../application/workspaceQueries";
import { AgentMemoryMutationOwner } from "../application/agentMemoryMutationOwner";

function memoryEntry(item: AgentMemoryItem): AgentMemoryEntry {
  return {
    id: item.id,
    scope: item.scope,
    content: item.content,
    origin: item.origin,
    status: item.status,
    pinned: item.pinned,
    sessionId: item.sessionId ?? "",
    day: item.day ?? "",
    createdAt: item.createdAt,
    updatedAt: item.updatedAt,
  };
}

function runtimeAgentMemoryGateway(client: FlameClient): AgentMemoryGateway {
  return {
    async review(id, decision) {
      await client.agentMemory.review(id, decision);
    },
    async updateContent(id, content) {
      return memoryEntry(await client.agentMemory.update({ id, content }));
    },
    async setPinned(id, pinned) {
      return memoryEntry(await client.agentMemory.update({ id, pinned }));
    },
    async delete(id) {
      await client.agentMemory.delete(id);
    },
    async add(input) {
      if (input.scope === "user") {
        return memoryEntry(await client.agentMemory.add({ scope: "user", content: input.content }));
      }
      const workspace = await client.workspaces.open(input.cwd ? { path: input.cwd } : undefined);
      return memoryEntry(await workspace.agentMemory.add(input.content));
    },
  };
}

export function installAgentMemoryGateway(runtimeClient: () => FlameClient) {
  const mutationOwner = AgentMemoryMutationOwner.install(
    runtimeAgentMemoryGateway(runtimeClient()),
  );
  return {
    replaceRuntimeGeneration: () =>
      mutationOwner.replaceRuntimeGeneration(() => runtimeAgentMemoryGateway(runtimeClient())),
    dispose: () => mutationOwner.dispose(),
  };
}
