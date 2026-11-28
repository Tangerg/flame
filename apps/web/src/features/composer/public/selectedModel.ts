import { useModels } from "@/features/providers/public/queries";
import { useActiveSessionId, useAgentSessionSummary } from "@/features/agent/public/session";
import { resolveComposerModelSelection } from "@/features/composer/application/modelSelection";
import { useComposerModelPreference } from "@/features/composer/public/modelPreference";

export function useSelectedModelSelection() {
  const { data: models = [] } = useModels();
  const preference = useComposerModelPreference();
  const activeSessionId = useActiveSessionId();
  const { data: session } = useAgentSessionSummary(activeSessionId || undefined);
  return resolveComposerModelSelection(
    models,
    preference,
    activeSessionId ? (session ?? undefined) : null,
  );
}

export function useSelectedModel() {
  return useSelectedModelSelection()?.model;
}
