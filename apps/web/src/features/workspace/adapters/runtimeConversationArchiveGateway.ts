import { asSessionId, type FlameClient, type SessionArtifact } from "@flame/runtime-client";
import { ConversationArchiveOwner } from "@/features/workspace/application/conversationExport";
import { type ConversationArchiveGateway } from "@/features/workspace/application/ports/conversationArchiveGateway";
import { type FileTransferPort } from "@/features/workspace/application/ports/fileTransfer";
function runtimeConversationArchiveGateway(client: FlameClient): ConversationArchiveGateway {
  return {
    async exportConversation(sessionId, format) {
      const response = await client.sessions.export(asSessionId(sessionId), format);
      if (format === "md" && response.markdown !== undefined) return response.markdown;
      if (format === "json" && response.artifact !== undefined) {
        return JSON.stringify(response.artifact, null, 2);
      }
      throw new Error(`runtime contract violation: sessions.export omitted its ${format} result`);
    },
    async exportTrajectory(sessionId) {
      const response = await client.sessions.exportTrajectory({
        sessionId: asSessionId(sessionId),
      });
      return JSON.stringify(response.trajectory, null, 2);
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

export function installConversationArchiveGateway(
  runtimeClient: () => FlameClient,
  files: FileTransferPort,
) {
  const owner = ConversationArchiveOwner.install({
    gateway: runtimeConversationArchiveGateway(runtimeClient()),
    files,
  });
  return {
    replaceRuntimeGeneration: () =>
      owner.replaceRuntimeGeneration(() => runtimeConversationArchiveGateway(runtimeClient())),
    dispose: () => owner.dispose(),
  };
}
