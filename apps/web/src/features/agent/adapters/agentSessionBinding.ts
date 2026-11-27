import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type AgentStoreOwner } from "@/features/agent/adapters/agentStore";
import { type AgentSessionStore } from "@/features/agent/adapters/agentSessionStore";
import { type FlameClient } from "@flame/runtime-client";
import { t } from "@/lib/i18n/index";
import {
  releaseRuntimeOpening,
  type RuntimeRunOpening,
  type RuntimeRunOpenings,
  type RunStartRecovery,
} from "./runtimeRunOpenings";
import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import {
  asItemId,
  asRunId,
  RpcConnectionError,
  type InterruptResponse,
} from "@flame/runtime-client";
import { queryClient } from "@/lib/queryClient";
import { type AgentInput } from "@/features/agent/domain/input";
import {
  settleAgentInput,
  type AgentInputSettlement,
  type AgentOpeningAcceptance,
} from "@/features/agent/application/input/inputSubmission";
import { type InterruptResumeInput } from "@/features/agent/domain/hitl";
import { selectCurrentRootRun } from "@/features/agent/application/view/runTree";
import { agentInputToContentBlocks } from "@/features/agent/adapters/wireInput";
import { createAgentRunPump } from "@/features/agent/adapters/agentRunPump";
import { createRunStreamReattach } from "@/features/agent/adapters/runStreamReattach";
import { refreshAgentSessionProjection } from "@/features/agent/application/session/refreshSessionProjection";
import { startAgentSessionRecovery } from "@/features/agent/adapters/agentSessionRecovery";
import { createOptimisticUserMessage } from "@/features/agent/adapters/optimisticUserMessage";
import { createRunOpeningController } from "@/features/agent/adapters/runOpeningController";
import { agentProblemFromRpcFailure } from "@/features/agent/adapters/rpcProblem";
import { createSessionProjectionSynchronization } from "@/features/agent/application/session/sessionProjectionSynchronization";
import { createRunCancellationController } from "@/features/agent/adapters/runCancellationController";
import { revalidateRunTermination } from "@/features/agent/application/run/revalidateRunTermination";

export interface AgentSessionDependencies {
  agentAccess: AgentDependencies;
  projection: AgentStoreOwner;
  sessions: AgentSessionStore;
  runOpenings: RuntimeRunOpenings;
}

export function bindAgentSession(
  { agentAccess, projection, sessions, runOpenings }: AgentSessionDependencies,
  client: () => FlameClient,
  sessionId: string,
  reportConnectionLoss: () => void,
): () => void {
  const store = projection.store.getState;
  const sessionMemory = sessions.store.getState();
  sessionMemory.holdOpen(sessionId);
  sessionMemory.rememberSession(sessionId);
  store().ensureSession(sessionId);

  let observation: AbortController | null = null;
  let disposed = false;
  let interacted = false;
  let commandsReady = false;

  const reportSynchronizationFailure = (error: unknown) => {
    if (error instanceof RpcConnectionError) reportConnectionLoss();
    else
      agentAccess.notifications.notify("error", t("agent.synchronizationIncomplete"), {
        source: "session",
      });
  };

  const runPump = createAgentRunPump({
    sessionId,
    onRunFinished: () => {
      void queryClient.invalidateQueries({ queryKey: agentAccess.usage.queryKey(sessionId) });
    },
    isCanceled: () => disposed,
    readEpoch: () => store().sessions[sessionId]?.viewEpoch ?? 0n,
    applyEvents: (events) => store().applyRunEvents(sessionId, events),
    readRunSnapshot: (runId, signal) => client().runs.get(runId, signal),
    applyRunSnapshot: (run) => store().applyRunSnapshot(sessionId, run),
    reattach: createRunStreamReattach(agentAccess, {
      sessionId,
      client,
      isCanceled: () => disposed,
      onConnectionLost: reportConnectionLoss,
      recoverProjection: (signal) =>
        refreshAgentSessionProjection(agentAccess, sessionId, {
          canCommit: () => !disposed && !signal.aborted,
          signal,
        }).then(() => undefined),
    }),
    onSynchronizationFailed: reportSynchronizationFailure,
    onIdle: () => projectionSynchronization.liveStreamSettled(),
  });

  let openingSynchronization = true;
  const projectionSynchronization = createSessionProjectionSynchronization({
    isLiveStreamActive: runPump.isActive,
    synchronize: (signal) => {
      const guardInteraction = openingSynchronization;
      openingSynchronization = false;
      return startAgentSessionRecovery(agentAccess, {
        client: client(),
        sessionId,
        signal,
        isCanceled: () => disposed,
        hasInteracted: () => guardInteraction && interacted,
        isFollowing: runPump.isFollowing,
        setAbortController: (controller) => {
          observation?.abort();
          observation = controller;
        },
        pump: runPump.pump,
        onSynchronizationFailed: reportSynchronizationFailure,
      }).then((authoritativeView) => {
        if (!disposed && !signal.aborted) commandsReady = authoritativeView !== null;
        return authoritativeView !== null;
      });
    },
  });

  const runOpening = createRunOpeningController({
    sessionId,
    isCanceled: () => disposed,
    markInteracted: () => {
      interacted = true;
    },
    abortCurrent: () => observation?.abort(),
    setAbortController: (ctrl) => {
      observation = ctrl;
    },
    setStartError: (error) => store().setCommandError(sessionId, error),
    onRejected: () => {
      commandsReady = false;
      void projectionSynchronization.request();
    },
  });

  const observeRecoveredOpening = async (
    receipt: AgentOpeningAcceptance,
    signal: AbortSignal,
    release?: () => Promise<void> | undefined,
  ): Promise<void> => {
    commandsReady = false;
    try {
      if (receipt.sessionId !== sessionId) {
        throw new Error("accepted opening belongs to another Session");
      }
      if (receipt.status === "retired") {
        agentAccess.notifications.notify("info", t("agent.acceptedRunRetired"), {
          source: "session",
        });
      }
      await release?.();
      if (!disposed && !signal.aborted) await projectionSynchronization.request();
    } catch (error) {
      if (!disposed && !signal.aborted) reportSynchronizationFailure(error);
      throw error;
    }
  };

  type RuntimeOpening = RuntimeRunOpening<{ runId: string; userItemId?: string }>;
  const acceptance = ({
    runId,
    userItemId,
    observation,
  }: RuntimeOpening): AgentOpeningAcceptance => ({
    status: observation.type === "retired" ? "retired" : "accepted",
    sessionId: observation.type === "stream" ? sessionId : observation.sessionId,
    runId,
    ...(userItemId ? { userItemId } : {}),
  });
  const recoverRuntimeOpening = (opening: RuntimeOpening, signal: AbortSignal) =>
    observeRecoveredOpening(acceptance(opening), signal, () => releaseRuntimeOpening(opening));

  const send = (
    input: AgentInput,
    options: AgentRunStartOptions = {},
  ): Promise<AgentInputSettlement> | null => {
    if (
      !commandsReady ||
      runOpening.isStarting() ||
      store().sessions[sessionId]?.openingCommands.length
    )
      return null;
    const currentRoot = store().sessions[sessionId]?.view;
    if (currentRoot && selectCurrentRootRun(currentRoot)?.status === "waiting") return null;
    const wireInput = agentInputToContentBlocks(input);
    const optimistic = createOptimisticUserMessage(wireInput);
    store().appendLocalMessage(sessionId, optimistic.message);
    let original: RunStartRecovery | undefined;
    const opening = runOpening.begin(
      (signal: AbortSignal) =>
        runOpenings.start(sessionId, wireInput, options, signal, (command) => {
          original = command;
        }),
      {
        onResult: (opening) => {
          if (opening.observation.type === "retired")
            store().dropMessage(sessionId, optimistic.localId);
          else store().reconcileMessageIdentity(sessionId, optimistic.localId, opening.userItemId);
        },
        onStartError: () => store().dropMessage(sessionId, optimistic.localId),
        release: releaseRuntimeOpening,
        observe: (opening, signal) =>
          opening.observation.type === "stream"
            ? runPump.pump({ result: opening, events: opening.observation.events }, signal)
            : recoverRuntimeOpening(opening, signal),
      },
    );
    return settleAgentInput(opening.then(acceptance), {
      key: () => original?.idempotencyKey,
      isUnknown: (error) => agentAccess.runtime.isInputReceiptUnknown(error),
      recover: () => {
        if (!original) throw new Error("input command omitted its original handle");
        return recoverStart(original);
      },
    });
  };

  function recoverStart(original: RunStartRecovery): Promise<AgentInputSettlement> {
    if (disposed || runOpening.isStarting())
      throw new Error("session is not ready to recover the original input");
    const opening = runOpening.begin(original.retry, {
      onResult: () => store().projectOpeningAccepted(sessionId, original.idempotencyKey),
      release: releaseRuntimeOpening,
      observe: recoverRuntimeOpening,
    });
    return settleAgentInput(opening.then(acceptance), {
      key: () => original.idempotencyKey,
      isUnknown: (error) => agentAccess.runtime.isInputReceiptUnknown(error),
      recover: () => recoverStart(original),
    });
  }

  const resume = (
    runId: string,
    responses: InterruptResumeInput[],
    onSettled?: () => void,
    onStartError?: () => void,
  ): boolean => {
    if (
      !commandsReady ||
      runOpening.isStarting() ||
      store().sessions[sessionId]?.openingCommands.length
    )
      return false;
    const wireResponses: InterruptResponse[] = responses.map((response) => ({
      itemId: asItemId(response.itemId),
      response: response.response,
    }));
    runOpening.begin(
      (signal: AbortSignal) =>
        runOpenings.resume(asRunId(runId), { responses: wireResponses }, signal),
      {
        onResult: onSettled ? () => onSettled() : undefined,
        onStartError,
        release: releaseRuntimeOpening,
        observe: recoverRuntimeOpening,
      },
    );
    return true;
  };

  const createCancellationGeneration = () => {
    let generationClient: ReturnType<typeof client> | null = null;
    const admittedClient = () => (generationClient ??= client());
    return createRunCancellationController({
      markInteracted: () => {
        interacted = true;
      },
      readTarget: (runId) => {
        const entry = store().sessions[sessionId];
        const run = entry?.view.runsById[runId];
        return entry && run
          ? {
              terminal: run.status === "finished",
              viewEpoch: entry.viewEpoch,
              viewRevision: entry.viewRevision,
            }
          : null;
      },
      execute: (runId) => admittedClient().runs.cancel(asRunId(runId)),
      commitIfCurrent: (response, target) =>
        store().commitCancelResponse(
          sessionId,
          { viewEpoch: target.viewEpoch, viewRevision: target.viewRevision },
          response,
        ),
      revalidateTerminal: (runId) => revalidateRunTermination(agentAccess, sessionId, runId),
      onSettled: () => {
        observation?.abort();
        projectionSynchronization.request();
        void queryClient.invalidateQueries({
          queryKey: agentAccess.usage.queryKey(sessionId),
        });
      },
      onSynchronizationFailed: reportSynchronizationFailure,
      onFailure: (runId, error) => {
        console.error("[agent] run cancellation failed:", sessionId, runId, error);
        const problem = agentProblemFromRpcFailure(error);
        if (problem) store().setCommandError(sessionId, problem);
      },
    });
  };
  let runCancellation = createCancellationGeneration();

  const cancelRun = (runId: string): void => runCancellation.cancel(runId);

  const stop = (): boolean => {
    const view = store().sessions[sessionId]?.view;
    const root = view ? selectCurrentRootRun(view) : null;
    if (root?.status !== "running") return false;
    cancelRun(root.id);
    return true;
  };

  store().setSend(sessionId, send);
  store().setStop(sessionId, stop);
  store().setResume(sessionId, resume);
  store().setRetryOpening(sessionId, (command) => {
    if (!commandsReady || runOpening.isStarting()) return null;
    if (!store().sessions[sessionId]?.openingCommands.includes(command)) return null;
    const opening = runOpening.begin(command.retry, {
      onResult: () => store().projectOpeningAccepted(sessionId, command.idempotencyKey),
      release: (receipt) => receipt.releaseObservation?.(),
      observe: (receipt, signal) =>
        observeRecoveredOpening(receipt, signal, receipt.releaseObservation),
    });
    const settlement = opening.then(({ releaseObservation, ...receipt }) => receipt);
    void settlement.catch(() => {});
    return settlement;
  });
  store().setSynchronize(sessionId, (ownership) => {
    if (
      ownership === "replace-live" ||
      ownership === "retire-live" ||
      ownership === "replace-server"
    ) {
      commandsReady = false;
      runOpening.retire();
      runCancellation.retire();
      if (ownership === "retire-live") {
        projectionSynchronization.retire();
        return Promise.resolve(false);
      }
      runCancellation = createCancellationGeneration();
      return projectionSynchronization.replace();
    }
    return projectionSynchronization.request();
  });
  store().setCancelRun(sessionId, cancelRun);
  void projectionSynchronization.request();

  return () => {
    disposed = true;
    runOpening.retire();
    runCancellation.retire();
    projectionSynchronization.dispose();
    runPump.dispose();
    observation?.abort();
    store().setSend(sessionId, null);
    store().setStop(sessionId, null);
    store().setResume(sessionId, null);
    store().setRetryOpening(sessionId, null);
    store().setSynchronize(sessionId, null);
    store().setCancelRun(sessionId, null);
  };
}
