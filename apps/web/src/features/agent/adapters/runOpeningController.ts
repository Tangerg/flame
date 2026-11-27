import { disposeAsyncIterable } from "@/lib/asyncOwnership";
import { type RunId, type SegmentId, type RunOpeningResult } from "@flame/runtime-client";
import { type AgentProblem } from "@/features/agent/public/sessionView";
import { endSpan, startRunSpan, withSpan } from "@/lib/observability/tracing";
import { agentProblemFromRpcFailure } from "@/features/agent/adapters/rpcProblem";
import { mutationSettlementIsUnknown } from "@flame/runtime-client";
type RunOpening = { runId: RunId; segmentId: SegmentId };

interface RunOpeningControllerOptions {
  sessionId: string;
  isCanceled: () => boolean;
  markInteracted: () => void;
  setAbortController: (controller: AbortController) => void;
  abortCurrent: () => void;
  setStartError: (error: AgentProblem) => void;
  onRejected: () => void;
}

interface RunOpeningController {
  isStarting: () => boolean;
  begin: <Result extends RunOpening>(
    run: (signal: AbortSignal) => Promise<RunOpeningResult<Result>>,
    callbacks: {
      observe: (opening: RunOpeningResult<Result>, signal: AbortSignal) => Promise<void>;
      onResult?: (result: Result, observation: RunOpeningResult<Result>["observation"]) => void;
      onStartError?: () => void;
    },
  ) => Promise<RunOpeningResult<Result>>;
  retire: () => void;
}

export function createRunOpeningController({
  sessionId,
  isCanceled,
  markInteracted,
  setAbortController,
  abortCurrent,
  setStartError,
  onRejected,
}: RunOpeningControllerOptions): RunOpeningController {
  let active: { releaseDraft(): void; finishSpan(): void } | null = null;

  return {
    isStarting: () => active !== null,
    begin(run, { observe, onResult, onStartError }) {
      active?.releaseDraft();
      active?.finishSpan();
      markInteracted();
      abortCurrent();
      const ctrl = new AbortController();
      setAbortController(ctrl);
      const span = startRunSpan({ "flame.session_id": sessionId });
      let failure: unknown;
      let releasePendingDraft = onStartError;
      const releaseDraft = () => {
        const release = releasePendingDraft;
        releasePendingDraft = undefined;
        release?.();
      };
      let spanEnded = false;
      const finishSpan = () => {
        if (spanEnded) return;
        spanEnded = true;
        endSpan(span, failure);
      };
      const openingLease = { releaseDraft, finishSpan };
      active = openingLease;
      let opening: ReturnType<typeof run>;
      try {
        opening = withSpan(span, () => run(ctrl.signal));
      } catch (err) {
        opening = Promise.reject(err);
      }
      void opening
        .then(
          async (opening) => {
            if (isCanceled() || ctrl.signal.aborted || active !== openingLease) {
              if (opening.observation.type === "stream") {
                void disposeAsyncIterable(opening.observation.events);
              }
              return;
            }
            try {
              releasePendingDraft = undefined;
              onResult?.(opening.result, opening.observation);
              span.setAttribute("flame.run_id", opening.result.runId);
              await observe(opening, ctrl.signal);
            } catch (err) {
              if (isCanceled() || ctrl.signal.aborted || active !== openingLease) return;
              failure = err;
              console.error("[agent] accepted run stream failed:", sessionId, err);
            }
          },
          (err: unknown) => {
            if (isCanceled() || ctrl.signal.aborted || active !== openingLease) return;
            active = null;
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
          if (active === openingLease) active = null;
          finishSpan();
        });
      // Acceptance and the observed Run have different lifetimes. The existing
      // controller keeps the stream; input consumers only receive its receipt.
      return opening;
    },
    retire() {
      const opening = active;
      active = null;
      opening?.releaseDraft();
      abortCurrent();
      opening?.finishSpan();
    },
  };
}
