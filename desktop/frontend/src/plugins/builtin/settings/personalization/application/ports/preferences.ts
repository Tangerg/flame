import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { StreamReveal } from "@/plugins/builtin/chat/message/public/streamReveal";

export interface PersonalizationPreferencesPort {
  useCompletionSound(): boolean;
  useSetCompletionSound(): (on: boolean) => void;
  useStreamReveal(): StreamReveal;
  useSetStreamReveal(): (mode: StreamReveal) => void;
}

const port = createSingletonPort<PersonalizationPreferencesPort>(
  "Personalization preferences port is not configured",
);

export const configurePersonalizationPreferencesPort = port.configure;
export const personalizationPreferences = port.get;
