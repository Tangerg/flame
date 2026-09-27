import {
  asSessionId,
  type FlameClient,
  type SessionArtifact,
} from "@flame/runtime-contract/client";
import { ConversationArchiveOwner } from "../application/conversationExport";
import type { ConversationArchiveGateway } from "../application/ports/conversationArchiveGateway";
import { browserFileTransfer } from "./browserFileTransfer";

function runtimeConversationArchiveGateway(client: FlameClient): ConversationArchiveGateway {
  return {
    async exportConversation(sessionId, format) {
      return client.sessions.export(asSessionId(sessionId), format);
    },
    async importConversation(artifact) {
      const { session } = await client.sessions.import(artifact as SessionArtifact);
      return {
        id: session.id,
        title: session.title,
      };
    },
  };
}

export function installConversationArchiveGateway(runtimeClient: () => FlameClient) {
  const owner = ConversationArchiveOwner.install({
    gateway: runtimeConversationArchiveGateway(runtimeClient()),
    files: browserFileTransfer(),
  });
  return {
    replaceRuntimeGeneration: () =>
      owner.replaceRuntimeGeneration(() => runtimeConversationArchiveGateway(runtimeClient())),
    dispose: () => owner.dispose(),
  };
}
