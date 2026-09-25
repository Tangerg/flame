import { service } from "dougong";

export interface WorkspaceScope {
  adoptSessionScope: (sessionId: string) => void;
  activateSessionScope: (sessionId: string) => void;
  forgetSessionScopes: (openSessionIds: string[]) => void;
}

export const WORKSPACE_SCOPE = service<WorkspaceScope>("flame.workspace.scope");

export interface WorkspaceMutationLifecycle {
  replaceRuntimeGeneration(): void;
}

export const WORKSPACE_MUTATION_LIFECYCLE = service<WorkspaceMutationLifecycle>(
  "flame.workspace.mutationLifecycle",
);
