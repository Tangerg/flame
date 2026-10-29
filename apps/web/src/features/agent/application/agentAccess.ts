import { type AgentDependencies } from "./agentDependencies";
import {
  closeActiveAgentSession,
  getAgentSessionLifecycleSnapshot,
  getActiveSessionId,
  selectAgentSession,
  subscribeAgentSessionLifecycle,
  subscribeDeletedAgentSession,
  subscribeActiveSessionId,
} from "./session/activeSession";
import { createSession } from "./session/createSession";
import {
  activeAgentConversation,
  forkAgentSessionAtRun,
  rollbackSessionToBeforeRun,
  regenerateSessionRun,
} from "./session/historyActions";
import { rehydrateSessionView } from "./session/rehydrateSession";
import {
  synchronizeMountedAgentSession,
  synchronizeMountedAgentSessions,
} from "./session/refreshSessionProjection";
import { stepActiveAgentSession } from "./session/stepSession";
import {
  cancelSessionRun,
  dismissActiveSessionProblem,
  stopCurrentRootRun,
} from "./run/runCommands";
import { subscribeAnySessionRunning } from "./run/rootAttention";
import { retrySteerInput } from "./input/steering";
import { allowMCPTool, forgetRule, forgetRules, setApprovalMode } from "./approvalPolicy";
import { getActiveConversationMessages } from "./conversation/readModel";
import { submitPendingApproval } from "./hitl/submitPendingApproval";

export function createAgentAccess(dependencies: AgentDependencies) {
  return {
    closeActiveAgentSession: closeActiveAgentSession.bind(undefined, dependencies),
    getAgentSessionLifecycleSnapshot: getAgentSessionLifecycleSnapshot.bind(
      undefined,
      dependencies,
    ),
    getActiveSessionId: getActiveSessionId.bind(undefined, dependencies),
    selectAgentSession: selectAgentSession.bind(undefined, dependencies),
    subscribeAgentSessionLifecycle: subscribeAgentSessionLifecycle.bind(undefined, dependencies),
    subscribeDeletedAgentSession: subscribeDeletedAgentSession.bind(undefined, dependencies),
    subscribeActiveSessionId: subscribeActiveSessionId.bind(undefined, dependencies),
    createSession: createSession.bind(undefined, dependencies),
    activeAgentConversation: activeAgentConversation.bind(undefined, dependencies),
    forkAgentSessionAtRun: forkAgentSessionAtRun.bind(undefined, dependencies),
    rollbackSessionToBeforeRun: rollbackSessionToBeforeRun.bind(undefined, dependencies),
    regenerateSessionRun: regenerateSessionRun.bind(undefined, dependencies),
    rehydrateSessionView: rehydrateSessionView.bind(undefined, dependencies),
    synchronizeMountedAgentSession: synchronizeMountedAgentSession.bind(undefined, dependencies),
    synchronizeMountedAgentSessions: synchronizeMountedAgentSessions.bind(undefined, dependencies),
    stepActiveAgentSession: stepActiveAgentSession.bind(undefined, dependencies),
    cancelSessionRun: cancelSessionRun.bind(undefined, dependencies),
    dismissActiveSessionProblem: dismissActiveSessionProblem.bind(undefined, dependencies),
    stopCurrentRootRun: stopCurrentRootRun.bind(undefined, dependencies),
    subscribeAnySessionRunning: subscribeAnySessionRunning.bind(undefined, dependencies),
    subscribeRootRunSettlements: dependencies.view.subscribeRootRunSettlements,
    retrySteerInput: retrySteerInput.bind(undefined, dependencies),
    allowMCPTool: allowMCPTool.bind(undefined, dependencies),
    forgetRule: forgetRule.bind(undefined, dependencies),
    forgetRules: forgetRules.bind(undefined, dependencies),
    setApprovalMode: setApprovalMode.bind(undefined, dependencies),
    getActiveConversationMessages: getActiveConversationMessages.bind(undefined, dependencies),
    submitPendingApproval: submitPendingApproval.bind(undefined, dependencies),
    sessionUsageQueryKey: dependencies.usage.queryKey,
    ownsSessionUsageQuery: dependencies.usage.ownsQuery,
    registerAgentSessionSharedMaterial: dependencies.sharedMaterial.register,
    getAgentSessionSharedMaterial<T>(sessionId: string, path: string): T | undefined {
      return dependencies.view.getSession(sessionId)?.view.shared[path] as T | undefined;
    },
  } as const;
}
export type AgentAccess = ReturnType<typeof createAgentAccess>;
