import { type WorkspaceNavigationPort } from "@/features/workspace/application/navigationState";
import { type LocalWorkspace } from "@/features/workspace/application/localWorkspace";
import { type WorkspaceErrorClassifier } from "@/features/workspace/application/fileRead";
import { type ConversationArchive } from "@/features/workspace/application/conversationExport";
export interface WorkspaceAccess {
  readonly navigation: WorkspaceNavigationPort;
  readonly local: LocalWorkspace;
  readonly errors: WorkspaceErrorClassifier;
  readonly archive: ConversationArchive;
}
