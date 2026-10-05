import type { Contributor } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { asRunId, asSessionId, type FlameClient } from "@flame/runtime-contract/client";
import {
  TRAJECTORY_KEY,
  TRAJECTORY_RUN_KEY,
  type TrajectoryQuery,
  type TrajectoryRunQuery,
  type TrajectoryEntry,
} from "../application/run/trajectory";
import { runtimeItem, runtimeRunFact } from "./runtimeAgentFacts";
import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
  type ApprovalRulesQuery,
  type ApprovalRuleSummary,
} from "../application/approvalPolicyQueries";
import { AGENT_SESSIONS_KEY } from "../application/session/sessionQueries";
import { toAgentSessionSummary } from "./runtimeSessionSummary";

function requiredParams<P>(key: string, params: unknown): P {
  if (params === undefined) throw new Error(`Data provider "${key}" requires parameters`);
  return params as P;
}

export function registerAgentDataProviders(
  ctx: Contributor,
  runtimeClient: () => FlameClient,
): void {
  ctx.contribute(DATA_PROVIDER, {
    key: TRAJECTORY_KEY,
    fetcher: async (params, signal) => {
      const query = requiredParams<TrajectoryQuery>(TRAJECTORY_KEY, params);
      const page = await runtimeClient().sessions.trajectory(query, signal);
      const data: TrajectoryEntry[] = page.data.map((entry) => {
        switch (entry.type) {
          case "run":
            return {
              type: entry.type,
              occurredAt: entry.occurredAt,
              run: runtimeRunFact(entry.run),
            };
          case "item":
            return {
              type: entry.type,
              occurredAt: entry.occurredAt,
              item: runtimeItem(entry.item),
            };
          case "model":
            return { type: entry.type, occurredAt: entry.occurredAt, model: entry.model };
        }
      });
      return { data, ...(page.nextCursor ? { nextCursor: page.nextCursor } : {}) };
    },
  });
  ctx.contribute(DATA_PROVIDER, {
    key: TRAJECTORY_RUN_KEY,
    fetcher: async (params, signal) => {
      const query = requiredParams<TrajectoryRunQuery>(TRAJECTORY_RUN_KEY, params);
      return runtimeRunFact(await runtimeClient().runs.get(asRunId(query.runId), signal));
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
      return (
        await runtimeClient().approval.listRules(
          query.sessionId ? asSessionId(query.sessionId) : undefined,
          signal,
        )
      ).rules.map((rule): ApprovalRuleSummary => ({
        id: rule.id,
        scope: rule.scope,
        tool: { ...rule.tool },
        modelName: rule.modelName,
        stale: rule.stale,
        subject: rule.subject,
        dir: rule.dir,
        decision: rule.decision,
      }));
    },
  });
}
