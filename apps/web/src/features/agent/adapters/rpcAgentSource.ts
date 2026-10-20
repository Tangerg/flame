import { type AgentSourceSpec } from "@/features/agent/adapters/runtimeDriver";
import {
  activeRpcSessionId,
  createRpcAgentDriver,
  type RpcRunsGateway,
} from "@/features/agent/adapters/rpcAgentDriver";
import { type Translate } from "@/lib/i18n/index";
export type ActiveSessionId = () => string | null | undefined;

export function rpcAgentSource(
  t: Translate,
  activeSessionId: ActiveSessionId,
  gateway: () => RpcRunsGateway,
): AgentSourceSpec {
  return {
    id: "rpc",
    label: t("agentSource.rpc"),
    priority: 1,
    factory: () => createRpcAgentDriver(activeRpcSessionId(activeSessionId()), gateway),
  };
}
