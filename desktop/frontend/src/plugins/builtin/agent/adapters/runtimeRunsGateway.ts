import type { FlameClient } from "@flame/runtime-contract/client";
import {
  asItemId,
  asRunId,
  asSegmentId,
  asSessionId,
  createMutationSettler,
  type StartRunResponse,
} from "@flame/runtime-contract/client";
import type {
  RpcRunResumeParams,
  RpcRunsGateway,
  RpcRunStartParams,
} from "../application/rpcAgentDriver";

function runOpeningIdentity(method: "start" | "resume", params: unknown): string {
  return JSON.stringify([`runs.${method}`, params]);
}

export interface RuntimeRunsGateway extends RpcRunsGateway {
  replaceRuntimeGeneration(): void;
  dispose(): void;
}

class DefaultRuntimeRunsGateway implements RuntimeRunsGateway {
  constructor(private readonly runtimeClient: () => FlameClient) {}

  #openings = createMutationSettler({ acceptedAttempt: "retained" });

  async start({ sessionId, ...params }: RpcRunStartParams, signal?: AbortSignal) {
    const client = this.runtimeClient();
    const { result, events } = await this.#openings.settle(
      runOpeningIdentity("start", { sessionId, ...params }),
      (attemptSignal) =>
        client.runs.start({ ...params, sessionId: asSessionId(sessionId) }, attemptSignal),
      { parent: signal },
    );
    return { result: brandStartedRun(result), events };
  }

  async resume(params: RpcRunResumeParams, signal?: AbortSignal) {
    const client = this.runtimeClient();
    const { result, events } = await this.#openings.settle(
      runOpeningIdentity("resume", params),
      (attemptSignal) => client.runs.resume(params, attemptSignal),
      { parent: signal },
    );
    return {
      result: {
        runId: asRunId(result.runId),
        segmentId: asSegmentId(result.segmentId),
        ...(result.userItemId ? { userItemId: asItemId(result.userItemId) } : {}),
      },
      events,
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

export function runtimeRunsGateway(runtimeClient: () => FlameClient): RuntimeRunsGateway {
  return new DefaultRuntimeRunsGateway(runtimeClient);
}

function brandStartedRun(result: StartRunResponse) {
  return {
    runId: asRunId(result.runId),
    segmentId: asSegmentId(result.segmentId),
    userItemId: asItemId(result.userItemId),
  };
}
