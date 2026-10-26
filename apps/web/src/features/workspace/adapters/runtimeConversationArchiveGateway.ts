import { asSessionId, type FlameClient, type SessionArtifact } from "@flame/runtime-client";
import { type ConversationArchive } from "@/features/workspace/application/conversationExport";
import { type ConversationArchiveGateway } from "@/features/workspace/application/ports/conversationArchiveGateway";
import { type FileTransferPort } from "@/features/workspace/application/ports/fileTransfer";
import { prettyJSON } from "@/lib/json";
function runtimeConversationArchiveGateway(client: FlameClient): ConversationArchiveGateway {
  return {
    async exportConversation(sessionId, format) {
      const response = await client.sessions.export(asSessionId(sessionId), format);
      if (format === "md" && response.markdown !== undefined) return response.markdown;
      if (format === "json" && response.artifact !== undefined) {
        return prettyJSON(response.artifact);
      }
      throw new Error(`runtime contract violation: sessions.export omitted its ${format} result`);
    },
    async exportTrajectory(sessionId) {
      const response = await client.sessions.exportTrajectory({
        sessionId: asSessionId(sessionId),
      });
      return prettyJSON(response.trajectory);
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

export function createConversationArchiveGateway(
  archive: ConversationArchive,
  runtimeClient: () => FlameClient,
  files: FileTransferPort,
) {
  const owner = archive.install({
    gateway: runtimeConversationArchiveGateway(runtimeClient()),
    files,
  });
  return {
    replaceRuntimeGeneration: () =>
      owner.replaceRuntimeGeneration(() => runtimeConversationArchiveGateway(runtimeClient())),
    dispose: () => owner.dispose(),
  };
}
