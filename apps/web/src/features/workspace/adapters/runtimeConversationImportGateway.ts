import { type FlameClient, type SessionArtifact } from "@flame/runtime-client";
import { type ConversationImport } from "@/features/workspace/application/conversationImport";
import { type ConversationImportGateway } from "@/features/workspace/application/ports/conversationImportGateway";
import { type FileTransferPort } from "@/features/workspace/application/ports/fileTransfer";
function runtimeConversationImportGateway(client: FlameClient): ConversationImportGateway {
  return {
    async importConversation(artifact) {
      const { session } = await client.sessions.import(artifact as SessionArtifact);
      return {
        id: session.id,
        title: session.title,
      };
    },
  };
}

export function createConversationImportGateway(
  imports: ConversationImport,
  runtimeClient: () => FlameClient,
  files: FileTransferPort,
) {
  const owner = imports.install({
    gateway: runtimeConversationImportGateway(runtimeClient()),
    files,
  });
  return {
    replaceRuntimeGeneration: () =>
      owner.replaceRuntimeGeneration(() => runtimeConversationImportGateway(runtimeClient())),
    dispose: () => owner.dispose(),
  };
}
