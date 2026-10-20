import { defineExtensionPoint } from "@/extensions/contracts";
import { lookupExtensionPoint } from "@/extensions/selectors/extensions";
export interface AgentRunStartOptions {
  provider?: string;
  model?: string;
  reasoningEffort?: string;
}

export interface AgentRunOptionsProviderSpec {
  id: string;
  priority?: number;
  resolve: () => AgentRunStartOptions;
}

export const AGENT_RUN_OPTIONS = defineExtensionPoint<AgentRunOptionsProviderSpec>({
  id: "flame.agent.runOptions",
  keying: "single",
  keyOf: (s) => s.id,
});

function pickAgentRunOptionsProvider(): AgentRunOptionsProviderSpec | undefined {
  const providers = lookupExtensionPoint(AGENT_RUN_OPTIONS);
  if (providers.length === 0) return undefined;
  return providers.reduce((best, cur) =>
    (cur.priority ?? 0) >= (best.priority ?? 0) ? cur : best,
  );
}

export function resolveAgentRunStartOptions(): AgentRunStartOptions {
  return pickAgentRunOptionsProvider()?.resolve() ?? {};
}
