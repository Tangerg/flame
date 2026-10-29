import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type Message } from "@/features/agent/public/sessionView";
import { t } from "@/lib/i18n/index";
import { type AgentInput } from "@/features/agent/domain/input";
import { notifyInfo } from "@/features/notifications/public/notifications";
import { type RestoreType } from "@/features/agent/application/runtimeGateway";
import { submitAgentInput } from "@/features/agent/application/input/submitAgentInput";

import {
  selectRootNarrativeMessages,
  selectRootRollbackBoundary,
} from "@/features/agent/application/view/runTree";
import { forkSessionAt } from "@/features/agent/application/session/forkSession";
import { rehydrateSessionView } from "@/features/agent/application/session/rehydrateSession";
import { projectAgentSessionSnapshot } from "@/features/agent/application/session/sessionSnapshot";
import { type AgentCommandOwner } from "@/features/agent/application/agentCommandOwner";

type RollbackRefusal = { status: "unavailable" } | { status: "inFlight" };

export type SessionRollbackResult =
  { status: "committed"; userInput?: AgentInput } | RollbackRefusal;

export type SessionRegenerationResult =
  { status: "submitted" | "noInput" | "notReady" } | RollbackRefusal;

export interface ActiveAgentConversation {
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
  return withSessionRollback(agentAccess, sessionId, (owner) =>
    rollbackBeforeRun(agentAccess, owner, sessionId, runId, restoreType),
  );
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
    owner.assertCurrent();
    if (!rollback.userInput) return { status: "noInput" };
    if (!submitAgentInput(agentAccess, sessionId, rollback.userInput))
      return { status: "notReady" };
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
    notifyInfo(t("session.restore.noCheckpoint"), {
      source: "session",
    });
    return { status: "unavailable" };
  }
  const result = await owner.settle(
    runtime.rollbackSession({
      sessionId,
      ...(keep ? { toRunId: keep } : {}),
      ...(wantsFiles && keep ? { restoreType } : {}),
    }),
  );
  owner.assertCurrent();
  await owner.settle(rehydrateSessionView(agentAccess, sessionId));
  owner.assertCurrent();
  const userInput = result.droppedRuns.find((dropped) => dropped.runId === runId)?.userInput;
  return { status: "committed", ...(userInput ? { userInput } : {}) };
}

export function forkAgentSessionAtRun(
  agentAccess: AgentDependencies,
  sessionId: string,
  runId: string,
): Promise<void> {
  return forkSessionAt(agentAccess, sessionId, runId);
}
