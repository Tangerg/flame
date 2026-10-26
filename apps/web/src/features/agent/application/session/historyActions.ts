import { type AgentAccess } from "@/features/agent/public/agent";
import { type Message } from "@/features/agent/public/sessionView";
import { t } from "@/lib/i18n/index";
import { type AgentInput } from "@/features/agent/domain/input";
import { notifyInfo } from "@/features/notifications/public/notifications";
import { type RestoreType } from "@/features/agent/application/runtimeGateway";

import {
  selectRootNarrativeMessages,
  selectRootRuns,
} from "@/features/agent/application/view/runTree";
import { forkSessionAt } from "@/features/agent/application/session/forkSession";
import { rehydrateSessionView } from "@/features/agent/application/session/rehydrateSession";
import { projectAgentSessionSnapshot } from "@/features/agent/application/session/sessionSnapshot";

export type SessionRollbackResult =
  | { status: "committed"; userInput?: AgentInput }
  | { status: "unavailable" }
  | { status: "inFlight" };

export interface ActiveAgentConversation {
  sessionId: string;
  messages: Message[];
}

export function activeAgentConversation(agentAccess: AgentAccess): ActiveAgentConversation | null {
  const sessionId = agentAccess.state.getActiveSessionId();
  if (!sessionId) return null;
  return {
    sessionId,
    messages: selectRootNarrativeMessages(agentAccess.view.getCurrentView()),
  };
}

export function sendToAgentSession(
  agentAccess: AgentAccess,
  sessionId: string,
  input: AgentInput,
): boolean {
  return agentAccess.view.sendToSession(sessionId, input);
}

export async function rollbackSessionToBeforeRun(
  agentAccess: AgentAccess,
  sessionId: string,
  runId: string,
  restoreType: RestoreType = "history",
): Promise<SessionRollbackResult> {
  const owner = agentAccess.commands.current();
  const lease = owner.beginSessionRollback(sessionId);
  if (!lease) return { status: "inFlight" };
  const runtime = agentAccess.runtime;
  try {
    const material = await owner.settle(runtime.loadSessionSnapshot(sessionId));
    owner.assertCurrent();
    if (!material) return { status: "unavailable" };
    const view = projectAgentSessionSnapshot(material.snapshot);
    const roots = selectRootRuns(view);
    const index = roots.findIndex((run) => run.id === runId);
    if (index < 0) return { status: "unavailable" };
    const keep = index > 0 ? roots[index - 1]!.id : undefined;
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
    return {
      status: "committed",
      ...(userInput ? { userInput } : {}),
    };
  } catch (error) {
    if (!lease.isCurrent()) return { status: "unavailable" };
    throw error;
  } finally {
    lease.release();
  }
}

export function forkAgentSessionAtRun(
  agentAccess: AgentAccess,
  sessionId: string,
  runId: string,
): Promise<void> {
  return forkSessionAt(agentAccess, sessionId, runId);
}
