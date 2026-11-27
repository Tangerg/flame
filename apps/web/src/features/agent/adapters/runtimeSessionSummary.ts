import { asSessionId, isErrorType, type FlameClient, type Session } from "@flame/runtime-client";
import { type AgentSessionSummary } from "@/features/agent/application/session/sessionQueries";

export async function readRuntimeSessionSummaries(
  client: FlameClient,
  signal?: AbortSignal,
): Promise<AgentSessionSummary[]> {
  return (await client.sessions.list(undefined, signal).autoPagingToArray()).map(
    toAgentSessionSummary,
  );
}

export async function readRuntimeSessionSummary(
  client: FlameClient,
  sessionId: string,
  signal?: AbortSignal,
): Promise<AgentSessionSummary | null> {
  try {
    return toAgentSessionSummary(await client.sessions.get(asSessionId(sessionId), signal));
  } catch (error) {
    if (isErrorType(error, "session_not_found")) return null;
    throw error;
  }
}

export function toAgentSessionSummary(session: Session): AgentSessionSummary {
  return {
    id: session.id,
    revision: session.revision,
    title: session.title,
    status: session.status,
    provider: session.provider,
    model: session.model,
    ...(session.reasoningEffort ? { reasoningEffort: session.reasoningEffort } : {}),
    workspace: {
      path: session.workspace.ref.path,
      availability: session.workspace.availability,
    },
    ...(session.projectId ? { projectId: session.projectId } : {}),
    ...(session.favorite !== undefined ? { favorite: session.favorite } : {}),
    time: session.updatedAt || session.createdAt,
  };
}
