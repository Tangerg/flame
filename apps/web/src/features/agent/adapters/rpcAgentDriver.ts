import { type RuntimeAgentDriver } from "@/features/agent/adapters/runtimeDriver";
export const DEFAULT_RPC_SESSION_ID = "ses_default";

export type RpcAgentInput = Parameters<RuntimeAgentDriver["start"]>[0];

export type RpcAgentStartOptions = Parameters<RuntimeAgentDriver["start"]>[1];

type RpcAgentRunId = Parameters<RuntimeAgentDriver["resume"]>[0];

type RpcAgentResumeOptions = Parameters<RuntimeAgentDriver["resume"]>[1];

export interface RpcRunStartParams {
  sessionId: string;
  input: RpcAgentInput;
  provider?: string;
  model?: string;
  reasoningEffort?: string;
}

export interface RpcRunResumeParams {
  runId: RpcAgentRunId;
  responses: RpcAgentResumeOptions["responses"];
  input?: RpcAgentResumeOptions["input"];
}

export interface RpcRunsGateway {
  start: (
    params: RpcRunStartParams,
    signal?: AbortSignal,
    onMutation?: (idempotencyKey: string) => void,
  ) => ReturnType<RuntimeAgentDriver["start"]>;
  resume: (
    params: RpcRunResumeParams,
    signal?: AbortSignal,
  ) => ReturnType<RuntimeAgentDriver["resume"]>;
}

export function activeRpcSessionId(sessionId: string | null | undefined): string {
  return sessionId || DEFAULT_RPC_SESSION_ID;
}

export function rpcRunStartParams(
  sessionId: string,
  input: RpcAgentInput,
  options: RpcAgentStartOptions,
): RpcRunStartParams {
  const { provider, model, reasoningEffort } = options;
  return {
    sessionId,
    input,
    ...(provider && model
      ? { provider, model, ...(reasoningEffort ? { reasoningEffort } : {}) }
      : {}),
  };
}

export function createRpcAgentDriver(
  sessionId: string,
  gateway: () => RpcRunsGateway,
): RuntimeAgentDriver {
  return {
    start: (input, options, signal, onMutation) =>
      gateway().start(rpcRunStartParams(sessionId, input, options), signal, onMutation),
    resume: (runId, options, signal) => gateway().resume({ runId, ...options }, signal),
  };
}
