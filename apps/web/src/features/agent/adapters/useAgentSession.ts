import { type AgentAccess } from "@/features/agent/application/agentAccess";
import { type AgentStoreOwner } from "@/features/agent/adapters/agentStore";
import { type AgentSessionStore } from "@/features/agent/adapters/agentSessionStore";
import { type createAgentRuntimeGateway } from "@/features/agent/adapters/agentRuntimeGateway";
export interface AgentSessionDependencies {
  agentAccess: AgentAccess;
  projection: AgentStoreOwner;
  sessions: AgentSessionStore;
  retries: ReturnType<typeof createAgentRuntimeGateway>["retries"];
}
import { type FlameClient } from "@flame/runtime-client";
import { t } from "@/lib/i18n/index";
import { notifyError, notifyInfo } from "@/features/notifications/public/notifications";
import { type RuntimeAgentDriver } from "@/features/agent/adapters/runtimeDriver";
import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import {
  asItemId,
  asRunId,
  RpcConnectionError,
  type InterruptResponse,
} from "@flame/runtime-client";
import { useEffect, useEffectEvent } from "react";
import { queryClient } from "@/lib/queryClient";
import { type AgentInput } from "@/features/agent/domain/input";
import { type AgentSession } from "@/features/agent/application/session/sessionAccess";
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
import { disposeAsyncIterable } from "@/lib/asyncOwnership";
import { type RunOpeningResult, type RunId, type SegmentId } from "@flame/runtime-client";
export function useAgentSession(
  dependencies: AgentSessionDependencies,
  client: () => FlameClient,
  makeDriver: () => RuntimeAgentDriver,
  sessionId: string,
  onConnectionLost: () => void,
): AgentSession {
  const { agentAccess, retries } = dependencies;
  const useAgentStore = dependencies.projection.store;
  const useAgentSessionStore = dependencies.sessions.store;
  const createDriver = useEffectEvent(makeDriver);
  const reportConnectionLoss = useEffectEvent(onConnectionLost);

  useEffect(() => {
    if (!sessionId) return;
    const driver = createDriver();
    const store = () => useAgentStore.getState();

    const sessionMemory = useAgentSessionStore.getState();
    sessionMemory.holdOpen(sessionId);
    sessionMemory.rememberSession(sessionId);
    store().ensureSession(sessionId);

    let abort: AbortController | null = null;
    let cancelled = false;
    let interacted = false;
    let commandsReady = false;
    let projectionSynchronization: ReturnType<
      typeof createSessionProjectionSynchronization
    > | null = null;

    const reportSynchronizationFailure = (error: unknown) => {
      if (error instanceof RpcConnectionError) reportConnectionLoss();
      else notifyError(t("agent.synchronizationIncomplete"), { source: "session" });
    };

    const runPump = createAgentRunPump({
      sessionId,
      onRunFinished: () => {
        void queryClient.invalidateQueries({ queryKey: agentAccess.usage.queryKey(sessionId) });
      },
      isCancelled: () => cancelled,
      readEpoch: () => store().sessions[sessionId]?.viewEpoch ?? 0n,
      applyEvents: (events) => store().applyRunEvents(sessionId, events),
      readRunSnapshot: (runId, signal) => client().runs.get(runId, signal),
      applyRunSnapshot: (run) => store().applyRunSnapshot(sessionId, run),
      reattach: createRunStreamReattach(agentAccess, {
        sessionId,
        client,
        isCancelled: () => cancelled,
        onConnectionLost: reportConnectionLoss,
        recoverProjection: (signal) =>
          refreshAgentSessionProjection(agentAccess, sessionId, {
            canCommit: () => !cancelled && !signal.aborted,
            signal,
          }).then(() => undefined),
      }),
      onSynchronizationFailed: reportSynchronizationFailure,
      onIdle: () => projectionSynchronization?.liveStreamSettled(),
    });

    let guardInitialInteraction = true;
    projectionSynchronization = createSessionProjectionSynchronization({
      isLiveStreamActive: runPump.isActive,
      synchronize: (signal) => {
        const guardInteraction = guardInitialInteraction;
        guardInitialInteraction = false;
        return startAgentSessionRecovery(agentAccess, {
          client: client(),
          sessionId,
          signal,
          isCancelled: () => cancelled,
          hasInteracted: () => guardInteraction && interacted,
          isFollowing: runPump.isFollowing,
          setAbortController: (controller) => {
            abort?.abort();
            abort = controller;
          },
          pump: runPump.pump,
          onSynchronizationFailed: reportSynchronizationFailure,
        }).then((authoritativeView) => {
          if (!cancelled && !signal.aborted) commandsReady = authoritativeView !== null;
          return authoritativeView !== null;
        });
      },
    });

    void projectionSynchronization.request();

    const runOpening = createRunOpeningController({
      sessionId,
      isCancelled: () => cancelled,
      markInteracted: () => {
        interacted = true;
      },
      abortCurrent: () => abort?.abort(),
      setAbortController: (ctrl) => {
        abort = ctrl;
      },
      setStartError: (error) => store().setCommandError(sessionId, error),
      onRejected: () => {
        commandsReady = false;
        void projectionSynchronization?.request();
      },
    });

    const recoverOpeningObservation = async (
      opening: RunOpeningResult<{ runId: RunId; segmentId: SegmentId }>,
      signal: AbortSignal,
    ): Promise<void> => {
      commandsReady = false;
      try {
        const observation = opening.observation;
        if (observation.type !== "stream" && observation.sessionId !== sessionId) {
          throw new Error("accepted opening belongs to another Session");
        }
        if (observation.type === "retired") {
          notifyInfo(t("agent.acceptedRunRetired"), { source: "session" });
          const view = await refreshAgentSessionProjection(agentAccess, sessionId, {
            signal,
            canCommit: () => !cancelled && !signal.aborted,
          });
          if (!cancelled && !signal.aborted) commandsReady = view !== null;
          return;
        }
        if (observation.type === "stream") await disposeAsyncIterable(observation.events);
        const view = await startAgentSessionRecovery(agentAccess, {
          client: client(),
          sessionId,
          signal,
          isCancelled: () => cancelled,
          hasInteracted: () => false,
          isFollowing: runPump.isFollowing,
          setAbortController: (controller) => {
            abort = controller;
          },
          pump: runPump.pump,
          onSynchronizationFailed: reportSynchronizationFailure,
        });
        if (!cancelled && !signal.aborted) commandsReady = view !== null;
      } catch (error) {
        if (!cancelled && !signal.aborted) reportSynchronizationFailure(error);
        throw error;
      }
    };

    const send = (input: AgentInput, options: AgentRunStartOptions = {}): boolean => {
      if (
        !commandsReady ||
        runOpening.isStarting() ||
        store().sessions[sessionId]?.openingCommands.length
      )
        return false;
      const currentRoot = store().sessions[sessionId]?.view;
      if (currentRoot && selectCurrentRootRun(currentRoot)?.status === "waiting") return false;
      const wireInput = agentInputToContentBlocks(input);
      const optimistic = createOptimisticUserMessage(wireInput);
      store().appendLocalMessage(sessionId, optimistic.message);
      runOpening.begin((signal) => driver.start(wireInput, options, signal), {
        onResult: (result, observation) => {
          if (observation.type === "retired") store().dropMessage(sessionId, optimistic.localId);
          else store().reconcileMessageIdentity(sessionId, optimistic.localId, result.userItemId);
        },
        onStartError: () => store().dropMessage(sessionId, optimistic.localId),
        observe: (opening, signal) =>
          opening.observation.type === "stream"
            ? runPump.pump({ result: opening.result, events: opening.observation.events }, signal)
            : recoverOpeningObservation(opening, signal),
      });
      return true;
    };

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
        (signal) => driver.resume(asRunId(runId), { responses: wireResponses }, signal),
        {
          onResult: onSettled ? () => onSettled() : undefined,
          onStartError,
          observe: recoverOpeningObservation,
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
          abort?.abort();
          projectionSynchronization?.request();
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
    store().setRetryOpening(sessionId, (key) => {
      if (!commandsReady || runOpening.isStarting()) return false;
      const command = store().sessions[sessionId]?.openingCommands.find(
        (pending) => pending.idempotencyKey === key,
      );
      if (!command) return false;
      runOpening.begin((signal) => retries.retry(key, signal), {
        onResult: () => store().projectOpeningAccepted(sessionId, key),
        observe: recoverOpeningObservation,
      });
      return true;
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
          projectionSynchronization?.retire();
          return Promise.resolve(false);
        }
        runCancellation = createCancellationGeneration();
        return projectionSynchronization?.replace() ?? Promise.resolve(false);
      }
      return projectionSynchronization?.request() ?? Promise.resolve(false);
    });
    store().setCancelRun(sessionId, cancelRun);

    return () => {
      cancelled = true;
      runOpening.retire();
      runCancellation.retire();
      projectionSynchronization?.dispose();
      runPump.dispose();
      abort?.abort();
      store().setSend(sessionId, null);
      store().setStop(sessionId, null);
      store().setResume(sessionId, null);
      store().setRetryOpening(sessionId, null);
      store().setSynchronize(sessionId, null);
      store().setCancelRun(sessionId, null);
    };
  }, [client, sessionId, agentAccess, useAgentStore, useAgentSessionStore, retries]);

  return {
    send: (input: AgentInput, options?: AgentRunStartOptions) =>
      useAgentStore.getState().sessions[sessionId]?.send?.(input, options) ?? false,
    stop: () => {
      useAgentStore.getState().sessions[sessionId]?.stop?.();
    },
  };
}
