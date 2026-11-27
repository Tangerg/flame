import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type Message } from "@/features/agent/public/sessionView";
import { t } from "@/lib/i18n/index";
import { type AgentInput } from "@/features/agent/domain/input";
import { type RestoreType } from "@/features/agent/application/runtimeGateway";

import {
  selectCurrentRootRun,
  selectRootNarrativeMessages,
  selectRootRollbackBoundary,
} from "@/features/agent/application/view/runTree";
import { synchronizeRewrittenSessionView } from "@/features/agent/application/session/synchronizeRewrittenSessionView";
import { projectAgentSessionSnapshot } from "@/features/agent/application/session/sessionSnapshot";
import { type AgentCommandOwner } from "@/features/agent/application/agentCommandOwner";

type RollbackRefusal = { status: "unavailable" } | { status: "inFlight" };

type SessionRollbackResult = { status: "committed"; userInput?: AgentInput } | RollbackRefusal;

type SessionRegenerationResult =
  | { status: "submitted" | "noInput" }
  | { status: "notReady"; userInput: AgentInput; refusal?: { error: unknown } }
  | RollbackRefusal;

interface ActiveAgentConversation {
  sessionId: string;
  messages: Message[];
}

export function activeAgentConversation(
  agentAccess: AgentDependencies,
): ActiveAgentConversation | null {
  const sessionId = agentAccess.state.getActiveSessionId();
  if (!sessionId) return null;
  return {
    sessionId,
    messages: selectRootNarrativeMessages(agentAccess.view.getCurrentView()),
  };
}

export function rollbackSessionToBeforeRun(
  agentAccess: AgentDependencies,
  sessionId: string,
  runId: string,
  restoreType: RestoreType = "history",
): Promise<SessionRollbackResult> {
  return withSessionRollback(agentAccess, sessionId, async (owner) => {
    const rollback = await rollbackBeforeRun(agentAccess, owner, sessionId, runId, restoreType);
    if (rollback.status === "committed") void refreshAfterRollback(agentAccess, owner, sessionId);
    return rollback;
  });
}

export function regenerateSessionRun(
  agentAccess: AgentDependencies,
  sessionId: string,
  runId: string,
  restoreType: RestoreType = "history",
): Promise<SessionRegenerationResult> {
  return withSessionRollback<SessionRegenerationResult>(agentAccess, sessionId, async (owner) => {
    const rollback = await rollbackBeforeRun(agentAccess, owner, sessionId, runId, restoreType);
    if (rollback.status !== "committed") return rollback;
    const refreshed = await refreshAfterRollback(agentAccess, owner, sessionId);
    owner.assertCurrent();
    const { userInput } = rollback;
    if (!userInput) return { status: "noInput" };
    const unsent = { status: "notReady", userInput } as const;
    const session = agentAccess.view.getSession(sessionId);
    if (!refreshed || !session?.send) return unsent;
    const root = selectCurrentRootRun(session.view);
    if (root && root.status !== "finished") return unsent;
    const opening = session.send(userInput);
    if (!opening) return unsent;
    try {
      await owner.settle(opening);
    } catch (error) {
      owner.assertCurrent();
      return { ...unsent, refusal: { error } };
    }
    return { status: "submitted" };
  });
}

async function withSessionRollback<T>(
  agentAccess: AgentDependencies,
  sessionId: string,
  execute: (owner: AgentCommandOwner) => Promise<T>,
): Promise<T | RollbackRefusal> {
  const owner = agentAccess.commands.current();
  const lease = owner.beginSessionRollback(sessionId);
  if (!lease) return { status: "inFlight" };
  try {
    const result = await execute(owner);
    owner.assertCurrent();
    return result;
  } catch (error) {
    if (!lease.isCurrent()) return { status: "unavailable" };
    throw error;
  } finally {
    lease.release();
  }
}

async function rollbackBeforeRun(
  agentAccess: AgentDependencies,
  owner: AgentCommandOwner,
  sessionId: string,
  runId: string,
  restoreType: RestoreType,
): Promise<SessionRollbackResult> {
  const runtime = agentAccess.runtime;
  const material = await owner.settle(runtime.loadSessionSnapshot(sessionId));
  owner.assertCurrent();
  if (!material) return { status: "unavailable" };
  const view = projectAgentSessionSnapshot(material.snapshot);
  const boundary = selectRootRollbackBoundary(view, runId);
  if (!boundary) return { status: "unavailable" };
  const keep = boundary.toRunId;
  const wantsFiles = restoreType !== "history";
  if (wantsFiles && !keep) {
    agentAccess.notifications.notify("info", t("session.restore.noCheckpoint"), {
      source: "session",
    });
    return { status: "unavailable" };
  }
  const result = await owner.settle(
    runtime.rollbackSession({
      sessionId,
      ...material.rollbackBasis,
      ...(keep ? { toRunId: keep } : {}),
      ...(wantsFiles && keep ? { restoreType } : {}),
    }),
  );
  owner.assertCurrent();
  const userInput = result.droppedRuns.find((dropped) => dropped.runId === runId)?.userInput;
  return { status: "committed", ...(userInput ? { userInput } : {}) };
}

async function refreshAfterRollback(
  agentAccess: AgentDependencies,
  owner: AgentCommandOwner,
  sessionId: string,
): Promise<boolean> {
  try {
    return await owner.settle(synchronizeRewrittenSessionView(agentAccess, sessionId));
  } catch (error) {
    if (owner.isCurrent()) {
      console.error("[session] rollback view could not be refreshed:", error);
      agentAccess.notifications.notify("error", t("agent.synchronizationIncomplete"), {
        source: "session",
      });
    }
    return false;
  }
}
