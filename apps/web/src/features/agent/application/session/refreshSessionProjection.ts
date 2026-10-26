import { type AgentAccess } from "@/features/agent/public/agent";
import { type AgentSessionView } from "@/features/agent/public/sessionView";
import { ASYNC_OWNERSHIP_RETIRED as ABORTED, settleBeforeAbort } from "@/lib/asyncOwnership";
import { type SessionProjectionSynchronizationOwnership } from "@/features/agent/application/session/sessionAccess";
import { type AgentSessionMaterialRead } from "@/features/agent/application/runtimeGateway";

import { projectAgentSessionSnapshot } from "@/features/agent/application/session/sessionSnapshot";
import { reconcileSteerMessages } from "@/features/agent/application/view/viewMutations";
import { restoreUnresolvedSteers } from "@/features/agent/application/input/steering";
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
  agentAccess: AgentAccess,
  sessionId: string,
  options: RefreshSessionProjectionOptions = {},
): Promise<AgentSessionView | null> {
  const result = await revalidateAgentSessionProjection(agentAccess, sessionId, options);
  return result?.committed ? result.authoritativeView : null;
}

export async function revalidateAgentSessionProjection(
  agentAccess: AgentAccess,
  sessionId: string,
  options: RefreshSessionProjectionOptions = {},
): Promise<AgentSessionProjectionRevalidation | null> {
  return revalidateAgentSessionMaterial(
    agentAccess,
    sessionId,
    () => agentAccess.runtime.loadSessionSnapshot(sessionId, options.signal),
    options,
  );
}

export async function revalidateAgentSessionMaterial(
  agentAccess: AgentAccess,
  sessionId: string,
  readMaterial: () => Promise<AgentSessionMaterialRead | null>,
  options: RefreshSessionProjectionOptions = {},
): Promise<AgentSessionProjectionRevalidation | null> {
  const viewPort = agentAccess.view;
  const token = viewPort.beginViewRefresh(sessionId, options.invalidateQueuedRunEvents ?? false);
  if (!token) return null;

  const read = readMaterial();
  const material = options.signal ? await settleBeforeAbort(read, options.signal) : await read;
  if (material === ABORTED) return null;
  if (!material || (options.canCommit && !options.canCommit())) return null;
  const projected = projectAgentSessionSnapshot(material.snapshot);
  const shared = material.projectAssociatedSharedMaterial(projected.shared);
  const view = shared === projected.shared ? projected : { ...projected, shared };
  const previous = viewPort.getSession(sessionId)?.view;
  const reconciled = previous ? reconcileSteerMessages(previous, view) : view;
  const committed = viewPort.commitViewRefresh(
    sessionId,
    token,
    reconciled,
    material.unresolvedOpenings,
  );
  if (committed && material.unresolvedSteers?.length)
    restoreUnresolvedSteers(agentAccess, sessionId, material.unresolvedSteers);
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
  agentAccess: AgentAccess,
  sessionId: string,
  ownership: SessionProjectionSynchronizationOwnership,
): Promise<boolean> {
  const entry = agentAccess.view.getSession(sessionId);
  if (!entry) return Promise.resolve(false);
  if (entry.synchronize) return entry.synchronize(ownership);
  if (ownership === "retire-live") return Promise.resolve(false);
  return refreshAgentSessionProjection(agentAccess, sessionId).then((view) => view !== null);
}

export function synchronizeMountedAgentSessions(
  agentAccess: AgentAccess,
  request: MountedAgentSessionSynchronization,
): readonly string[] {
  const sessions = agentAccess.view.getSessions();
  const mountedIds = Object.keys(sessions);
  const requested = request.sessionIds ? new Set(request.sessionIds) : null;
  const targets = requested
    ? mountedIds.filter((sessionId) => requested.has(sessionId))
    : mountedIds;
  if (request.ownership === "replace-live" || request.ownership === "retire-live") {
    agentAccess.view.retireProjectionGeneration(targets);
  }
  if (request.ownership === "replace-server") {
    agentAccess.view.replaceServerScope(targets);
  }
  for (const sessionId of targets) {
    void synchronizeMountedAgentSession(agentAccess, sessionId, request.ownership);
  }
  return targets;
}
