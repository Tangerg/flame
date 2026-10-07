import { queryClient } from "@/lib/queryClient";
import {
  MODELS_KEY,
  useModels,
  type SelectableModel,
} from "@/plugins/builtin/providers/public/queries";
import {
  AGENT_SESSIONS_KEY,
  getActiveSessionId,
  useActiveSessionId,
  useAgentSessions,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import {
  resolveComposerModelSelection,
  type ComposerSessionModelSelection,
} from "../application/modelSelection";
import { selectedComposerModelPreference, useComposerModelPreference } from "./modelPreference";

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
