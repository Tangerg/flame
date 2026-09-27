import type { Contributor } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { asSessionId, type FlameClient, type Session } from "@flame/runtime-contract/client";
import {
  MODEL_INVOCATIONS_KEY,
  type ModelInvocationQuery,
} from "../application/run/modelInvocations";
import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
  type ApprovalRulesQuery,
} from "../application/approvalPolicyQueries";
import {
  AGENT_SESSIONS_KEY,
  type AgentSessionSummary,
} from "../application/session/sessionQueries";

function requiredParams<P>(key: string, params: unknown): P {
  if (params === undefined) throw new Error(`Data provider "${key}" requires parameters`);
  return params as P;
}

export function registerAgentDataProviders(
  ctx: Contributor,
  runtimeClient: () => FlameClient,
): void {
  ctx.contribute(DATA_PROVIDER, {
    key: MODEL_INVOCATIONS_KEY,
    fetcher: async (params, signal) => {
      const query = requiredParams<ModelInvocationQuery>(MODEL_INVOCATIONS_KEY, params);
      return runtimeClient().modelInvocations.list(query, signal);
    },
  });
  ctx.contribute(DATA_PROVIDER, {
    key: AGENT_SESSIONS_KEY,
    fetcher: async (_params, signal) =>
      (await runtimeClient().sessions.list(undefined, signal).autoPagingToArray()).map(
        toAgentSessionSummary,
      ),
  });
  ctx.contribute(DATA_PROVIDER, {
    key: APPROVAL_MODE_KEY,
    fetcher: async (_params, signal) => (await runtimeClient().approval.getMode(signal)).mode,
  });
  ctx.contribute(DATA_PROVIDER, {
    key: APPROVAL_RULES_KEY,
    fetcher: async (params, signal) => {
      const query = requiredParams<ApprovalRulesQuery>(APPROVAL_RULES_KEY, params);
      return (await runtimeClient().approval.listRules(asSessionId(query.sessionId), signal)).rules;
    },
  });
}

function toAgentSessionSummary(session: Session): AgentSessionSummary {
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
