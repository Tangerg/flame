import { queryClient } from "@/lib/queryClient";
import { MODELS_KEY, useModels } from "@/features/providers/public/queries";
import { type SelectableModel } from "@/features/providers/public/queries";
import {
  AGENT_SESSIONS_KEY,
  useAgentSessions,
  type AgentSessionSummary,
} from "@/features/agent/public/session";
import { getActiveSessionId, useActiveSessionId } from "@/features/agent/public/session";
import {
  resolveComposerModelSelection,
  type ComposerSessionModelSelection,
} from "@/features/composer/application/modelSelection";
import {
  selectedComposerModelPreference,
  useComposerModelPreference,
} from "@/features/composer/public/modelPreference";
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

export function selectedModelSelection() {
  return resolveComposerModelSelection(
    queryClient.getQueryData<SelectableModel[]>([MODELS_KEY]) ?? [],
    selectedComposerModelPreference(),
    activeSessionModelSelection(
      getActiveSessionId(),
      queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]),
    ),
  );
}

export function useSelectedModel() {
  return useSelectedModelSelection()?.model;
}
