import { useModels } from "@/plugins/builtin/settings/providers/public/queries";
import { useActiveSessionId, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { resolveComposerModelSelection } from "../application/modelSelection";
import { useComposerModelPreference } from "./modelPreference";

/** The model the next run will use: the composer's explicit pick, else the active
 *  Session's own selection, else the catalog default. While an active Session summary is
 *  loading there is deliberately NO fallback — choosing early turns a query race into a
 *  model override. `undefined` when no provider is enabled yet. */
export function useSelectedModelSelection() {
  const { data: models = [] } = useModels();
  const preference = useComposerModelPreference();
  const activeSessionId = useActiveSessionId();
  const { data: sessions } = useAgentSessions();
  const activeSessionSelection = activeSessionId
    ? sessions === undefined
      ? undefined
      : (() => {
          const session = sessions.find((candidate) => candidate.id === activeSessionId);
          return session
            ? {
                provider: session.provider,
                model: session.model,
                reasoningEffort: session.reasoningEffort,
              }
            : null;
        })()
    : null;
  return resolveComposerModelSelection(models, preference, activeSessionSelection);
}

export function useSelectedModel() {
  return useSelectedModelSelection()?.model;
}
