import { type FlameClient, type SessionArtifact } from "@flame/runtime-client";
import { type RuntimeServerScope } from "@/features/connection/public/services";
import { type ConversationImport } from "@/features/workspace/application/conversationImport";
import { type ConversationImportGateway } from "@/features/workspace/application/ports/conversationImportGateway";

function runtimeConversationImportGateway(
  runtimeClient: () => FlameClient,
): ConversationImportGateway {
  return {
    async importConversation(artifact) {
      const { session } = await runtimeClient().sessions.import(artifact as SessionArtifact);
      return {
        id: session.id,
        title: session.title,
      };
    },
  };
}

export function bindConversationImportRuntime(
  imports: ConversationImport,
  runtimeClient: () => FlameClient,
  scope: RuntimeServerScope,
): () => void {
  const replaceTarget = () =>
    imports.replaceTarget(runtimeConversationImportGateway(runtimeClient));
  replaceTarget();
  const unsubscribe = scope.subscribeReplacement(replaceTarget);
  return () => {
    unsubscribe();
    imports.retireTarget();
  };
}
