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
    open: () => Promise<Result>,
    callbacks: {
      observe: (opening: Result, signal: AbortSignal) => Promise<void>;
      release?: (opening: Result) => Promise<void> | undefined;
      onResult?: (opening: Result) => void;
      releaseProvisional?: () => void;
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
  let active: { releasePending(): void } | null = null;

  return {
    isStarting: () => active !== null,
    begin(open, { observe, release, onResult, releaseProvisional }) {
      active?.releasePending();
      onBegin();
      abortCurrent();
      const ctrl = new AbortController();
      setAbortController(ctrl);
      const span = startRunSpan({ "flame.session_id": sessionId });
      let failure: unknown;
      let releasePendingView = releaseProvisional;
      const releasePending = () => {
        const release = releasePendingView;
        releasePendingView = undefined;
        release?.();
      };
      const openingLease = { releasePending };
      active = openingLease;
      let opening: ReturnType<typeof open>;
      try {
        opening = withSpan(span, open);
      } catch (err) {
        opening = Promise.reject(err);
      }
      void opening
        .then(
          async (opening) => {
            span.setAttribute("flame.run_id", opening.runId);
            if (isCanceled() || ctrl.signal.aborted || active !== openingLease) {
              void release?.(opening);
              return;
            }
            try {
              releasePendingView = undefined;
              onResult?.(opening);
              await observe(opening, ctrl.signal);
            } catch (err) {
              if (isCanceled() || ctrl.signal.aborted || active !== openingLease) return;
              failure = err;
              console.error("[agent] accepted run stream failed:", sessionId, err);
            }
          },
          (err: unknown) => {
            failure = err;
            if (isCanceled() || ctrl.signal.aborted || active !== openingLease) return;
            active = null;
            releasePending();
            console.error("[agent] run failed to start:", sessionId, err);
            onRejected();
            if (mutationSettlementIsUnknown(err)) return;
            const problem = agentProblemFromRpcFailure(err);
            if (problem) setStartError(problem);
          },
        )
        .finally(() => {
          if (active === openingLease) active = null;
          endSpan(span, failure);
        });
      // A view may retire while the connection still owns the opening.
      return opening;
    },
    retire() {
      const opening = active;
      active = null;
      opening?.releasePending();
      abortCurrent();
    },
  };
}
