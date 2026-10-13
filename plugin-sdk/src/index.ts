import type { ClientTelemetry } from "./telemetry";
import type { ComponentType } from "react";
import type {
  FlameClient,
  MutationPromise,
} from "@flame/runtime-contract/client";
import type {
  CallPluginResponse,
  Descriptor,
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

export interface WorkspaceContext {
  readonly ref: Readonly<WorkspaceRef>;
  readonly focus?: Readonly<{ path: string; revision: bigint }>;
}

export interface ComponentContext {
  readonly runtime: FlameClient;
  readonly telemetry: ClientTelemetry;
  readonly workspace?: Readonly<WorkspaceContext>;
  readonly sessionId?: string;
  readonly binding?: Readonly<PluginOperationBinding>;
  readonly signal: AbortSignal;
  track<T>(operation: Promise<T>): Promise<T>;
}

export type PluginComponent = ComponentType<{ context: ComponentContext }>;

export class OperationFailure extends Error {
  constructor(
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "OperationFailure";
  }
}

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
  ) {
    context.signal.throwIfAborted();
    const descriptor = required.methods.find((value) => value.name === method);
    if (
      !descriptor ||
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
    query: async <K extends keyof Inputs & string>(
      method: K,
      input: Inputs[K],
      signal: AbortSignal = context.signal,
    ): Promise<Outputs[K]> =>
      context.track(
        context.runtime.plugins
          .call(request(method, input, "query"), {
            signal: AbortSignal.any([context.signal, signal]),
          })
          .then(output<Outputs[K]>),
      ),
    command: <K extends keyof Inputs & string>(
      method: K,
      input: Inputs[K],
    ): MutationPromise<Outputs[K]> => {
      const mutation = context.runtime.plugins.call(
        request(method, input, "command"),
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
