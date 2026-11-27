import { type FlameClient } from "@flame/runtime-client";
import {
  asItemId,
  asRunId,
  asSegmentId,
  asSessionId,
  createMutationSettler,
  type StartRunResponse,
} from "@flame/runtime-client";
import {
  type RpcRunResumeParams,
  type RpcRunsGateway,
  type RpcRunStartParams,
} from "@/features/agent/adapters/rpcAgentDriver";
export class RuntimeRunsGateway implements RpcRunsGateway {
  constructor(private readonly runtimeClient: () => FlameClient) {}

  #openings = createMutationSettler({ acceptedAttempt: "retained" });

  async start(
    { sessionId, ...params }: RpcRunStartParams,
    signal?: AbortSignal,
    onMutation?: (idempotencyKey: string) => void,
  ) {
    const client = this.runtimeClient();
    const { result, observation } = await this.#openings.settle(
      (attemptSignal) =>
        client.runs.start({ ...params, sessionId: asSessionId(sessionId) }, attemptSignal),
      { parent: signal, onMutation },
    );
    return { result: brandStartedRun(result), observation };
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

function brandStartedRun(result: StartRunResponse) {
  return {
    runId: asRunId(result.runId),
    segmentId: asSegmentId(result.segmentId),
    userItemId: asItemId(result.userItemId),
  };
}
