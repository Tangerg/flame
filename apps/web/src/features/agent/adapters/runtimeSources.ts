import { type AgentSourceSpec } from "@/features/agent/adapters/runtimeDriver";
import { defineExtensionPoint } from "@/extensions/contracts";
import { lookupExtensionPoint } from "@/extensions/selectors/extensions";
export const AGENT_SOURCE = defineExtensionPoint<AgentSourceSpec>({
  id: "flame.agent.source",
  keying: "single",
});

export function pickAgentSource(): AgentSourceSpec | undefined {
  const sources = lookupExtensionPoint(AGENT_SOURCE);
  if (sources.length === 0) return undefined;
  return sources.reduce((best, cur) => ((cur.priority ?? 0) > (best.priority ?? 0) ? cur : best));
}
