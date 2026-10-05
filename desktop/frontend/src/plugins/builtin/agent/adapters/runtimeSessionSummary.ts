import type { Session } from "@flame/runtime-contract/client";
import type { AgentSessionSummary } from "../application/session/sessionQueries";

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
    ...(session.favorite !== undefined ? { favorite: session.favorite } : {}),
    time: session.updatedAt || session.createdAt,
  };
}
