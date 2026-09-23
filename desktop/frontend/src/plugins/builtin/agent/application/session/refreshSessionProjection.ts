import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { ASYNC_OWNERSHIP_RETIRED as ABORTED, settleBeforeAbort } from "@/lib/asyncOwnership";
import type { SessionProjectionSynchronizationOwnership } from "../ports/sessionView";
import { agentRuntime, type AgentSessionMaterialRead } from "../ports/runtimeGateway";
import { agentSessionView } from "../ports/sessionView";
import { projectAgentSessionSnapshot } from "./sessionSnapshot";

interface RefreshSessionProjectionOptions {
  invalidateQueuedRunEvents?: boolean;
  canCommit?: () => boolean;
  signal?: AbortSignal;
}

export interface AgentSessionProjectionRevalidation {
  authoritativeView: AgentSessionView;
  committed: boolean;
}

export async function refreshAgentSessionProjection(
  sessionId: string,
  options: RefreshSessionProjectionOptions = {},
): Promise<AgentSessionView | null> {
  const result = await revalidateAgentSessionProjection(sessionId, options);
  return result?.committed ? result.authoritativeView : null;
}

export async function revalidateAgentSessionProjection(
  sessionId: string,
  options: RefreshSessionProjectionOptions = {},
): Promise<AgentSessionProjectionRevalidation | null> {
  return revalidateAgentSessionMaterial(
    sessionId,
    () => agentRuntime().loadSessionSnapshot(sessionId, options.signal),
    options,
  );
}

export async function revalidateAgentSessionMaterial(
  sessionId: string,
  readMaterial: () => Promise<AgentSessionMaterialRead | null>,
  options: RefreshSessionProjectionOptions = {},
): Promise<AgentSessionProjectionRevalidation | null> {
  const viewPort = agentSessionView();
  const token = viewPort.beginViewRefresh(sessionId, options.invalidateQueuedRunEvents ?? false);
  if (!token) return null;

  const read = readMaterial();
  const material = options.signal ? await settleBeforeAbort(read, options.signal) : await read;
  if (material === ABORTED) return null;
  if (!material || (options.canCommit && !options.canCommit())) return null;
  const projected = projectAgentSessionSnapshot(material.snapshot);
  const shared = material.projectAssociatedSharedMaterial(projected.shared);
  const view = shared === projected.shared ? projected : { ...projected, shared };
  const committed = viewPort.commitViewRefresh(sessionId, token, view);
  return {
    authoritativeView: view,
    committed,
  };
}

export interface MountedAgentSessionSynchronization {
  sessionIds?: readonly string[];
  ownership: SessionProjectionSynchronizationOwnership;
}

export function synchronizeMountedAgentSession(
  sessionId: string,
  ownership: SessionProjectionSynchronizationOwnership,
): Promise<boolean> {
  const entry = agentSessionView().getSession(sessionId);
  if (!entry) return Promise.resolve(false);
  if (entry.synchronize) return entry.synchronize(ownership);
  if (ownership === "retire-live") return Promise.resolve(false);
  return refreshAgentSessionProjection(sessionId).then((view) => view !== null);
}

export function synchronizeMountedAgentSessions(
  request: MountedAgentSessionSynchronization,
): readonly string[] {
  const sessions = agentSessionView().getSessions();
  const mountedIds = Object.keys(sessions);
  const requested = request.sessionIds ? new Set(request.sessionIds) : null;
  const targets = requested
    ? mountedIds.filter((sessionId) => requested.has(sessionId))
    : mountedIds;
  if (request.ownership === "replace-live" || request.ownership === "retire-live") {
    agentSessionView().retireProjectionGeneration(targets);
  }
  if (request.ownership === "replace-server") {
    agentSessionView().replaceServerScope(targets);
  }
  for (const sessionId of targets) {
    void synchronizeMountedAgentSession(sessionId, request.ownership);
  }
  return targets;
}
