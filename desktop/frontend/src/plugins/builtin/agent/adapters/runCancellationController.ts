import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { RetirableTaskCohort } from "@/lib/taskQueue";

interface RunCancellationTarget {
  terminal: boolean;
  viewEpoch: bigint;
  viewRevision: bigint;
}

interface RunCancellationControllerOptions<Response> {
  markInteracted: () => void;
  readTarget: (runId: string) => RunCancellationTarget | null;
  execute: (runId: string) => Promise<Response>;
  commitIfCurrent: (response: Response, target: RunCancellationTarget) => boolean;
  revalidateTerminal: (runId: string) => Promise<boolean>;
  onSettled: () => void;
  onFailure: (runId: string, error: unknown) => void;
}

export interface RunCancellationController {
  cancel(runId: string): void;
  retire(): void;
}

/** One cancellation command per Run inside one replaceable Runtime generation.
 *
 * A successful response is a snapshot taken at commit time, so it may only fold while the
 * material view still holds the epoch and revision the command started from. A failed
 * current-generation command is revalidated through the neutral Agent projection, where
 * another client reaching terminal counts as objective success and an active authoritative
 * Run preserves the original failure.
 */
export function createRunCancellationController<Response>({
  markInteracted,
  readTarget,
  execute,
  commitIfCurrent,
  revalidateTerminal,
  onSettled,
  onFailure,
}: RunCancellationControllerOptions<Response>): RunCancellationController {
  const pending = new Set<string>();
  const retiredError = new GenerationRetiredError("run_cancellation_generation");
  const cohort = new RetirableTaskCohort(retiredError);

  return {
    cancel(runId) {
      if (cohort.retired) return;
      const target = readTarget(runId);
      if (!target || target.terminal || pending.has(runId)) return;
      pending.add(runId);
      markInteracted();

      let command: Promise<Response>;
      try {
        command = execute(runId);
      } catch (error) {
        command = Promise.reject(error);
      }
      void cohort
        .settle(command)
        .then((response) => {
          commitIfCurrent(response, target);
          onSettled();
        })
        .catch(async (error: unknown) => {
          if (error === retiredError) return;
          let superseded = false;
          try {
            superseded = await cohort.settle(revalidateTerminal(runId));
          } catch (revalidationError) {
            if (revalidationError === retiredError) return;
            // Revalidation is evidence only. Its failure must neither replace
            // nor hide the command failure the caller can still act on.
          }
          if (superseded) {
            onSettled();
            return;
          }
          onFailure(runId, error);
        })
        .finally(() => pending.delete(runId));
    },
    retire() {
      cohort.retire();
    },
  };
}
