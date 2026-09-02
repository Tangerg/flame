// What this context PUBLISHES to other plugins, as dougong Services — see
// `agent/public/services` for when a capability is a Service and when it is an
// `application/ports/` inversion instead.

import { service } from "dougong";

export interface WorkspaceScope {
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
