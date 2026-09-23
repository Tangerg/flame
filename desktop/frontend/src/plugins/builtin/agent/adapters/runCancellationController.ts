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
