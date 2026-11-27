import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { useCallback } from "react";
import { t } from "@/lib/i18n/index";
import { tupleKey } from "@/lib/tupleKey";
import { invalidateAgentSessions } from "@/features/agent/application/session/sessionQueries";
import { type AgentSessionCreation } from "@/features/agent/application/runtimeGateway";
import { reportSessionError } from "@/features/agent/application/session/reportSessionError";
import { type AgentCommandOwner } from "@/features/agent/application/agentCommandOwner";
interface CreateSessionOptions {
  cwd: string;
  projectId?: string;
}

function observeSessionCreation(
  agentAccess: AgentDependencies,
  owner: AgentCommandOwner,
  key: string,
  observe: () => Promise<AgentSessionCreation>,
): Promise<string | null> {
  return owner
    .runSessionCreate(key, async () => {
      const outcome = await owner.settle(observe());
      owner.assertCurrent();
      if (outcome.type === "confirmed") {
        agentAccess.state.selectSession(outcome.sessionId);
        invalidateAgentSessions([outcome.sessionId]);
        return outcome.sessionId;
      }
      invalidateAgentSessions();
      agentAccess.notifications.show({
        level: "error",
        message: t("agent.command.unknown"),
        description: outcome.idempotencyKey,
        lifetime: "persistent",
        action: {
          label: t("agent.command.retry"),
          run: () => {
            if (!owner.isCurrent()) return;
            const recovery = tupleKey("recovery", outcome.idempotencyKey);
            void observeSessionCreation(agentAccess, owner, recovery, outcome.retry);
          },
        },
      });
      return null;
    })
    .catch((error: unknown) => {
      if (owner.isCurrent()) reportSessionError(agentAccess.notifications, "create", error);
      return null;
    });
}

function doCreate(
  agentAccess: AgentDependencies,
  opts: CreateSessionOptions,
): Promise<string | null> {
  if (opts.cwd.trim() === "") return Promise.resolve(null);
  const owner = agentAccess.commands.current();
  const key = tupleKey("create", opts.cwd, opts.projectId ?? "");
  return observeSessionCreation(agentAccess, owner, key, () =>
    agentAccess.runtime.createSession(opts),
  );
}

export async function createSession(agentAccess: AgentDependencies): Promise<string | null> {
  const sessionId = agentAccess.state.getActiveSessionId();
  if (!sessionId) return null;
  const current = agentAccess.view.getSession(sessionId);
  if (
    current?.hasAuthoritativeSnapshot &&
    current.openingCommands.length === 0 &&
    current.view.messages.length === 0 &&
    Object.keys(current.view.runsById).length === 0
  )
    return sessionId;
  const owner = agentAccess.commands.current();
  try {
    const active = await owner.settle(agentAccess.runtime.loadSessionSummary(sessionId));
    owner.assertCurrent();
    if (!active) return null;
    return doCreate(agentAccess, { cwd: active.workspace.path, projectId: active.projectId });
  } catch (error) {
    if (owner.isCurrent()) reportSessionError(agentAccess.notifications, "create", error);
    return null;
  }
}

export function useCreateSession(): (opts: CreateSessionOptions) => Promise<string | null> {
  const agentAccess = useAgentDependencies();

  return useCallback((opts) => doCreate(agentAccess, opts), [agentAccess]);
}
