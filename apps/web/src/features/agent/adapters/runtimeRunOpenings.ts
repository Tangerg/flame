import { type FlameClient } from "@flame/runtime-client";
import {
  asItemId,
  asRunId,
  asSegmentId,
  asSessionId,
  createMutationSettler,
  type StartRunResponse,
  type MutationSettler,
  type ContentBlock,
  type ItemId,
  type RunId,
  type RunOpeningResult,
  type SegmentId,
} from "@flame/runtime-client";
import type { ResumeRunRequest } from "@flame/runtime-client/wire";
import type { AgentRunStartOptions } from "@/features/agent/public/runOptions";
import { disposeAsyncIterable } from "@/lib/asyncOwnership";

export type RuntimeRunOpening<Result> = Result & Pick<RunOpeningResult<Result>, "observation">;

export function releaseRuntimeOpening({
  observation,
}: RuntimeRunOpening<unknown>): Promise<void> | undefined {
  if (observation.type === "stream") return disposeAsyncIterable(observation.events);
}

type StartedRunOpening = RuntimeRunOpening<{
  runId: RunId;
  segmentId: SegmentId;
  userItemId: ItemId;
}>;

export interface RunStartRecovery {
  readonly idempotencyKey: string;
  retry(): Promise<StartedRunOpening>;
}

type StartMutation = ReturnType<FlameClient["runs"]["start"]>;
export class RuntimeRunOpenings {
  constructor(private readonly runtimeClient: () => FlameClient) {}

  readonly #settler = createMutationSettler();

  start(
    sessionId: string,
    input: ContentBlock[],
    options: AgentRunStartOptions,
    onCommand?: (original: RunStartRecovery) => void,
  ): Promise<StartedRunOpening> {
    const client = this.runtimeClient();
    const { provider, model, reasoningEffort } = options;
    return observeRunStart(
      this.#settler,
      (attempt) =>
        client.runs.start(
          {
            sessionId: asSessionId(sessionId),
            input,
            ...(provider && model
              ? { provider, model, ...(reasoningEffort ? { reasoningEffort } : {}) }
              : {}),
          },
          attempt,
        ),
      onCommand,
    );
  }

  async resume(runId: RunId, request: Pick<ResumeRunRequest, "responses" | "input">) {
    const client = this.runtimeClient();
    const { result, observation } = await this.#settler.settle((attemptSignal) =>
      client.runs.resume({ runId, ...request }, attemptSignal),
    );
    return {
      runId: asRunId(result.runId),
      segmentId: asSegmentId(result.segmentId),
      ...(result.userItemId ? { userItemId: asItemId(result.userItemId) } : {}),
      observation,
    };
  }

  dispose(): void {
    this.#settler.dispose();
  }
}

async function observeRunStart(
  owner: MutationSettler,
  open: (signal: AbortSignal) => StartMutation,
  onCommand?: (original: RunStartRecovery) => void,
): Promise<StartedRunOpening> {
  const { result, observation } = await owner.settle(open, {
    onCommand:
      onCommand &&
      ((command) =>
        onCommand({
          idempotencyKey: command.idempotencyKey,
          retry: () => observeRunStart(owner, (attempt) => command.retry({ signal: attempt })),
        })),
  });
  return { ...brandStartedRun(result), observation };
}

function brandStartedRun(result: StartRunResponse) {
  return {
    runId: asRunId(result.runId),
    segmentId: asSegmentId(result.segmentId),
    userItemId: asItemId(result.userItemId),
  };
}
