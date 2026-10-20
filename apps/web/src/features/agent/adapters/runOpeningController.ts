import { disposeAsyncIterable } from "@/lib/asyncOwnership";
import { type RunId, type SegmentId, type RunOpeningResult } from "@flame/runtime-client";
import { type AgentProblem } from "@/features/agent/public/sessionView";
import { endSpan, startRunSpan, withSpan } from "@/lib/observability/tracing";
import { agentProblemFromRpcFailure } from "@/features/agent/adapters/rpcProblem";
import { mutationSettlementIsUnknown } from "@flame/runtime-client";
type RunOpening = { runId: RunId; segmentId: SegmentId };

interface RunOpeningControllerOptions {
  sessionId: string;
  isCancelled: () => boolean;
  markInteracted: () => void;
  setAbortController: (controller: AbortController) => void;
  abortCurrent: () => void;
  setStartError: (error: AgentProblem) => void;
  onRejected: () => void;
}

export interface RunOpeningController {
  isStarting: () => boolean;
  begin: <Result extends RunOpening>(
    run: (signal: AbortSignal) => Promise<RunOpeningResult<Result>>,
    callbacks: {
      observe: (opening: RunOpeningResult<Result>, signal: AbortSignal) => Promise<void>;
      onResult?: (result: Result, observation: RunOpeningResult<Result>["observation"]) => void;
      onStartError?: () => void;
    },
  ) => void;
  retire: () => void;
}

export function createRunOpeningController({
  sessionId,
  isCancelled,
  markInteracted,
  setAbortController,
  abortCurrent,
  setStartError,
  onRejected,
}: RunOpeningControllerOptions): RunOpeningController {
  let starting = false;
  let openingLease: object = {};
  let endActiveSpan: (() => void) | null = null;
  let releaseActiveDraft: (() => void) | null = null;

  return {
    isStarting: () => starting,
    begin(run, { observe, onResult, onStartError }) {
      releaseActiveDraft?.();
      endActiveSpan?.();
      starting = true;
      const ownLease = (openingLease = {});
      markInteracted();
      abortCurrent();
      const ctrl = new AbortController();
      setAbortController(ctrl);
      const span = startRunSpan({ "flame.session_id": sessionId });
      let failure: unknown;
      let accepted = false;
      let draftReleased = false;
      const releaseDraft = () => {
        if (accepted || draftReleased) return;
        draftReleased = true;
        onStartError?.();
      };
      releaseActiveDraft = releaseDraft;
      let spanEnded = false;
      const finishSpan = () => {
        if (spanEnded) return;
        spanEnded = true;
        if (endActiveSpan === finishSpan) endActiveSpan = null;
        endSpan(span, failure);
      };
      endActiveSpan = finishSpan;
      let opening: ReturnType<typeof run>;
      try {
        opening = withSpan(span, () => run(ctrl.signal));
      } catch (err) {
        opening = Promise.reject(err);
      }
      void opening
        .then(
          async (opening) => {
            if (isCancelled() || ctrl.signal.aborted || ownLease !== openingLease) {
              if (opening.observation.type === "stream") {
                void disposeAsyncIterable(opening.observation.events);
              }
              return;
            }
            try {
              accepted = true;
              onResult?.(opening.result, opening.observation);
              span.setAttribute("flame.run_id", opening.result.runId);
              await observe(opening, ctrl.signal);
            } catch (err) {
              if (isCancelled() || ctrl.signal.aborted || ownLease !== openingLease) return;
              failure = err;
              console.error("[agent] accepted run stream failed:", sessionId, err);
            }
          },
          (err: unknown) => {
            if (isCancelled() || ctrl.signal.aborted || ownLease !== openingLease) return;
            releaseDraft();
            failure = err;
            console.error("[agent] run failed to start:", sessionId, err);
            onRejected();
            if (mutationSettlementIsUnknown(err)) return;
            const problem = agentProblemFromRpcFailure(err);
            if (problem) setStartError(problem);
          },
        )
        .finally(() => {
          if (releaseActiveDraft === releaseDraft) releaseActiveDraft = null;
          if (ownLease === openingLease) starting = false;
          finishSpan();
        });
    },
    retire() {
      releaseActiveDraft?.();
      releaseActiveDraft = null;
      openingLease = {};
      starting = false;
      abortCurrent();
      endActiveSpan?.();
    },
  };
}
