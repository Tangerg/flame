import { type WorkspaceNavigationPort } from "@/features/workspace/application/navigationState";
import { type LocalWorkspace } from "@/features/workspace/application/localWorkspace";
import { type WorkspaceErrorClassifier } from "@/features/workspace/application/fileRead";
import { type ConversationImport } from "@/features/workspace/application/conversationImport";
import { type FlameClient } from "@flame/runtime-client";
export interface WorkspaceAccess {
  readonly navigation: WorkspaceNavigationPort;
  readonly local: LocalWorkspace;
  readonly errors: WorkspaceErrorClassifier;
  readonly conversationImport: Pick<ConversationImport, "importConversationJson">;
  taskWorktrees(): FlameClient["workspaces"]["tasks"];
}
