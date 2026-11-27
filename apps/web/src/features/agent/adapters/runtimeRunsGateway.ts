import { type FlameClient } from "@flame/runtime-client";
import {
  asItemId,
  asRunId,
  asSegmentId,
  asSessionId,
  createMutationSettler,
  type StartRunResponse,
  type MutationSettler,
} from "@flame/runtime-client";
import {
  type RpcRunResumeParams,
  type RpcRunsGateway,
  type RpcRunStartParams,
} from "@/features/agent/adapters/rpcAgentDriver";
import { type RunStartRecovery, type StartedRunOpening } from "./runtimeDriver";

type StartMutation = ReturnType<FlameClient["runs"]["start"]>;
export class RuntimeRunsGateway implements RpcRunsGateway {
  constructor(private readonly runtimeClient: () => FlameClient) {}

  #openings = createMutationSettler({ acceptedAttempt: "retained" });

  start(
    { sessionId, ...params }: RpcRunStartParams,
    signal?: AbortSignal,
    onCommand?: (original: RunStartRecovery) => void,
  ): Promise<StartedRunOpening> {
    const client = this.runtimeClient();
    return observeRunStart(
      this.#openings,
      (attempt) => client.runs.start({ ...params, sessionId: asSessionId(sessionId) }, attempt),
      signal,
      onCommand,
    );
  }

  async resume(params: RpcRunResumeParams, signal?: AbortSignal) {
    const client = this.runtimeClient();
    const { result, observation } = await this.#openings.settle(
      (attemptSignal) => client.runs.resume(params, attemptSignal),
      { parent: signal },
    );
    return {
      result: {
        runId: asRunId(result.runId),
        segmentId: asSegmentId(result.segmentId),
        ...(result.userItemId ? { userItemId: asItemId(result.userItemId) } : {}),
      },
      observation,
    };
  }

  replaceRuntimeGeneration(): void {
    const predecessor = this.#openings;
    this.#openings = createMutationSettler({ acceptedAttempt: "retained" });
    predecessor.dispose();
  }

  dispose(): void {
    this.#openings.dispose();
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
  return { result: brandStartedRun(result), observation };
}

function brandStartedRun(result: StartRunResponse) {
  return {
    runId: asRunId(result.runId),
    segmentId: asSegmentId(result.segmentId),
    userItemId: asItemId(result.userItemId),
  };
}
