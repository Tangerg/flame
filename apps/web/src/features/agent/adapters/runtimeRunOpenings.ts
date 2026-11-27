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
  retry(signal: AbortSignal): Promise<StartedRunOpening>;
}

type StartMutation = ReturnType<FlameClient["runs"]["start"]>;
export class RuntimeRunOpenings {
  constructor(private readonly runtimeClient: () => FlameClient) {}

  #settler = createMutationSettler({ acceptedAttempt: "retained" });

  start(
    sessionId: string,
    input: ContentBlock[],
    options: AgentRunStartOptions,
    signal?: AbortSignal,
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
      signal,
      onCommand,
    );
  }

  async resume(
    runId: RunId,
    request: Pick<ResumeRunRequest, "responses" | "input">,
    signal?: AbortSignal,
  ) {
    const client = this.runtimeClient();
    const { result, observation } = await this.#settler.settle(
      (attemptSignal) => client.runs.resume({ runId, ...request }, attemptSignal),
      { parent: signal },
    );
    return {
      runId: asRunId(result.runId),
      segmentId: asSegmentId(result.segmentId),
      ...(result.userItemId ? { userItemId: asItemId(result.userItemId) } : {}),
      observation,
    };
  }

  replaceRuntimeGeneration(): void {
    const predecessor = this.#settler;
    this.#settler = createMutationSettler({ acceptedAttempt: "retained" });
    predecessor.dispose();
  }

  dispose(): void {
    this.#settler.dispose();
  }
}

async function observeRunStart(
  owner: MutationSettler,
  open: (signal: AbortSignal) => StartMutation,
  parent?: AbortSignal,
  onCommand?: (original: RunStartRecovery) => void,
): Promise<StartedRunOpening> {
  const { result, observation } = await owner.settle(open, {
    parent,
    onCommand:
      onCommand &&
      ((command) =>
        onCommand({
          idempotencyKey: command.idempotencyKey,
          retry: (signal) =>
            observeRunStart(owner, (attempt) => command.retry({ signal: attempt }), signal),
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
