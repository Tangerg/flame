import type { ClientTelemetry } from "./telemetry";
import { OperationFailure } from "@flame/runtime-contract/client";
export { OperationFailure } from "@flame/runtime-contract/client";
import type { ComponentType } from "react";
import type {
  FlameClient,
  MutationPromise,
} from "@flame/runtime-contract/client";
import type {
  CallPluginResponse,
  Descriptor,
  Session,
  PluginOperationBinding,
  Target,
  WorkspaceRef,
} from "@flame/runtime-contract/wire";
export type {
  ClientTelemetry,
  TelemetrySnapshot,
  TelemetryAttributes,
  SpanRow,
  MetricRow,
  LogRow,
} from "./telemetry";
export { mutationSettlementIsUnknown } from "@flame/runtime-contract/client/mutation";
export { isErrorType } from "@flame/runtime-contract/client/errors";

export interface WorkspaceContext {
  readonly ref: Readonly<WorkspaceRef>;
  readonly focus?: Readonly<{ path: string; revision: bigint }>;
}

export interface CapturedFile {
  readonly workspace?: Readonly<WorkspaceRef>;
  readonly path: string;
  readonly content: string;
  readonly startLine: number;
  readonly totalLines: number;
  readonly truncated: boolean;
}

export interface RecordedFileChange {
  readonly path: string;
  readonly status: "added" | "deleted" | "modified" | "moved";
  readonly from?: string;
  readonly added: number;
  readonly removed: number;
}

export interface CapturedToolResult {
  readonly id: string;
  readonly runId: string;
  readonly name: string;
  readonly fn: string;
  readonly fnKind?: "path";
  readonly args: string;
  readonly status: "running" | "ok" | "err" | "denied";
  readonly added?: number;
  readonly removed?: number;
  readonly changes?: readonly RecordedFileChange[];
  readonly hits?: number;
  readonly files?: number;
  readonly lines?: number;
  readonly exitCode?: number;
  readonly result?: string;
  readonly command?: string;
  readonly error?: string;
  readonly operation?: string;
  readonly safetyClass?: "safe" | "write" | "exec" | "network";
  readonly range?: Readonly<{ start: number; end: number }>;
  readonly durationMillis?: number;
  readonly approvalDecision?: "approve" | "deny";
}

export type CapturedSession = Readonly<
  Pick<Session, "id" | "revision" | "title">
>;

export interface ComponentContext {
  readonly file?: Readonly<CapturedFile>;
  readonly toolResult?: Readonly<CapturedToolResult>;
  readonly runtime: FlameClient;
  readonly telemetry: ClientTelemetry;
  readonly workspace?: Readonly<WorkspaceContext>;
  readonly sessionId?: string;
  readonly sessionTarget?: CapturedSession;
  readonly binding?: Readonly<PluginOperationBinding>;
  readonly signal: AbortSignal;
  track<T>(operation: Promise<T>): Promise<T>;
}

export type PluginComponent = ComponentType<{ context: ComponentContext }>;

function output<R>(reply: CallPluginResponse): R {
  if (reply.failure)
    throw new OperationFailure(reply.failure.code, reply.failure.message);
  if (!("output" in reply))
    throw new Error("plugin operation omitted its output");
  return reply.output as R;
}

// Mapping preserves the Runtime client's original mutation identity and retry
// closure. A component never creates a replacement command after response loss.
function mapMutation<R>(
  source: MutationPromise<CallPluginResponse>,
  context: ComponentContext,
): MutationPromise<R> {
  const mapped = Object.defineProperties(source.then(output<R>), {
    idempotencyKey: { enumerable: true, value: source.idempotencyKey },
    retry: {
      enumerable: true,
      value: (options?: { signal?: AbortSignal }) => {
        context.signal.throwIfAborted();
        return mapMutation<R>(
          source.retry({
            signal: options?.signal
              ? AbortSignal.any([context.signal, options.signal])
              : context.signal,
          }),
          context,
        );
      },
    },
  }) as MutationPromise<R>;
  context.track(mapped);
  return mapped;
}

export function bindOperations<
  Inputs,
  Outputs extends { [K in keyof Inputs]: unknown },
  Chunks extends { [K in keyof Inputs]?: unknown } = {},
>(context: ComponentContext, contract: string, target: Target) {
  if (!context.binding)
    throw new Error("component has no declared callable contribution");
  const binding = structuredClone(context.binding);
  const required = JSON.parse(contract) as Descriptor;
  const captured = structuredClone(target);
  function request<K extends keyof Inputs & string>(
    method: K,
    input: Inputs[K],
    kind: "query" | "command",
    delivery: "unary" | "stream",
  ) {
    context.signal.throwIfAborted();
    const descriptor = required.methods.find((value) => value.name === method);
    if (
      !descriptor ||
      (descriptor.stream !== undefined) !== (delivery === "stream") ||
      descriptor.kind !== kind ||
      descriptor.target !== captured.scope
    ) {
      throw new Error(
        "plugin operation differs from its required contract or target",
      );
    }
    return {
      installationId: binding.installationId,
      digest: binding.digest,
      contribution: binding.contribution,
      contract,
      method,
      input: structuredClone(input),
      target: captured,
    };
  }
  return {
    stream: <K extends keyof Inputs & keyof Chunks & string>(
      intent: string,
      method: K,
      input: Inputs[K],
      consume: (chunk: Chunks[K], signal: AbortSignal) => void | Promise<void>,
      signal: AbortSignal = context.signal,
    ): Promise<Outputs[K]> => {
      const invocation = request(method, input, "command", "stream");
      const { input: _, contract, method: name, ...binding } = invocation;
      return context.track(
        context.runtime.plugins.stream(
          intent,
          { contract, method: name, input: invocation.input },
          async () => binding,
          (chunk, signal) => consume(chunk as Chunks[K], signal),
          (value) => value as Outputs[K],
          AbortSignal.any([context.signal, signal]),
        ),
      );
    },
    query: async <K extends keyof Inputs & string>(
      method: K,
      input: Inputs[K],
      signal: AbortSignal = context.signal,
    ): Promise<Outputs[K]> =>
      context.track(
        context.runtime.plugins
          .call(request(method, input, "query", "unary"), {
            signal: AbortSignal.any([context.signal, signal]),
          })
          .then(output<Outputs[K]>),
      ),
    command: <K extends keyof Inputs & string>(
      method: K,
      input: Inputs[K],
    ): MutationPromise<Outputs[K]> => {
      const mutation = context.runtime.plugins.call(
        request(method, input, "command", "unary"),
        { signal: context.signal },
      );
      if (!("retry" in mutation) || !("idempotencyKey" in mutation))
        throw new Error("Runtime omitted the command identity");
      return mapMutation<Outputs[K]>(
        mutation as MutationPromise<CallPluginResponse>,
        context,
      );
    },
  };
}
