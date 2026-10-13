import type { AgentMemoryGateway } from "../application/ports/agentMemoryGateway";
import type { AgentMemoryItem, FlameClient } from "@flame/runtime-contract/client";
import type { AgentMemoryEntry } from "../application/workspaceQueries";
import {
  addInstalledMemory,
  deleteInstalledMemory,
  reviewInstalledMemory,
  updateInstalledMemory,
} from "../public/memory";
import { AgentMemoryMutationOwner } from "../application/agentMemoryMutationOwner";

export function agentMemoryEntry(item: AgentMemoryItem): AgentMemoryEntry {
  return {
    id: item.id,
    scope: item.scope,
    content: item.content,
    origin: item.origin,
    status: item.status,
    pinned: item.pinned,
    ...(item.sessionId ? { sessionId: item.sessionId } : {}),
    createdAt: item.createdAt,
    updatedAt: item.updatedAt,
  };
}

function runtimeAgentMemoryGateway(client: FlameClient): AgentMemoryGateway {
  return {
    async review(id, decision) {
      await reviewInstalledMemory(client, id, decision);
    },
    async updateContent(id, content) {
      return agentMemoryEntry(await updateInstalledMemory(client, { ID: id, Content: content }));
    },
    async setPinned(id, pinned) {
      return agentMemoryEntry(await updateInstalledMemory(client, { ID: id, Pinned: pinned }));
    },
    async delete(id) {
      await deleteInstalledMemory(client, id);
    },
    async add(input) {
      return agentMemoryEntry(await addInstalledMemory(client, input));
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
