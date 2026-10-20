import { useModels } from "@/features/providers/public/queries";
import { useAgentSessions, type AgentSessionSummary } from "@/features/agent/public/session";
import { useActiveSessionId } from "@/features/agent/public/session";
import {
  resolveComposerModelSelection,
  type ComposerSessionModelSelection,
} from "@/features/composer/application/modelSelection";
import { useComposerModelPreference } from "@/features/composer/public/modelPreference";
function activeSessionModelSelection(
  activeSessionId: string,
  sessions: readonly AgentSessionSummary[] | undefined,
): ComposerSessionModelSelection | null | undefined {
  if (!activeSessionId) return null;
  if (sessions === undefined) return undefined;
  const session = sessions.find((candidate) => candidate.id === activeSessionId);
  return session
    ? {
        provider: session.provider,
        model: session.model,
        reasoningEffort: session.reasoningEffort,
      }
    : null;
}

export function useSelectedModelSelection() {
  const { data: models = [] } = useModels();
  const preference = useComposerModelPreference();
  const activeSessionId = useActiveSessionId();
  const { data: sessions } = useAgentSessions();
  return resolveComposerModelSelection(
    models,
    preference,
    activeSessionModelSelection(activeSessionId, sessions),
  );
}

export function useSelectedModel() {
  return useSelectedModelSelection()?.model;
}
