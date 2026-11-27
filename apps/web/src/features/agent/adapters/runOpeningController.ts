import { type AgentProblem } from "@/features/agent/public/sessionView";
import { endSpan, startRunSpan, withSpan } from "@/lib/observability/tracing";
import { agentProblemFromRpcFailure } from "@/features/agent/adapters/rpcProblem";
import { mutationSettlementIsUnknown } from "@flame/runtime-client";
type OpenedRun = { runId: string };

interface RunOpeningControllerOptions {
  sessionId: string;
  isCanceled: () => boolean;
  onBegin: () => void;
  setAbortController: (controller: AbortController) => void;
  abortCurrent: () => void;
  setStartError: (error: AgentProblem) => void;
  onRejected: () => void;
}

interface RunOpeningController {
  isStarting: () => boolean;
  begin: <Result extends OpenedRun>(
    run: (signal: AbortSignal) => Promise<Result>,
    callbacks: {
      observe: (opening: Result, signal: AbortSignal) => Promise<void>;
      release?: (opening: Result) => Promise<void> | undefined;
      onResult?: (opening: Result) => void;
      onStartError?: () => void;
    },
  ) => Promise<Result>;
  retire: () => void;
}

export function createRunOpeningController({
  sessionId,
  isCanceled,
  onBegin,
  setAbortController,
  abortCurrent,
  setStartError,
  onRejected,
}: RunOpeningControllerOptions): RunOpeningController {
  let active: { releaseDraft(): void; finishSpan(): void } | null = null;

  return {
    isStarting: () => active !== null,
    begin(run, { observe, release, onResult, onStartError }) {
      active?.releaseDraft();
      active?.finishSpan();
      onBegin();
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
              void release?.(opening);
              return;
            }
            try {
              releasePendingDraft = undefined;
              onResult?.(opening);
              span.setAttribute("flame.run_id", opening.runId);
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
