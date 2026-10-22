import type { ClientTelemetry } from "./telemetry";
import {
  OperationFailure,
  parseJSON,
  stringifyJSON,
} from "@flame/runtime-client";
import { pluginMethod } from "@flame/runtime-client";
export { OperationFailure } from "@flame/runtime-client";
import type { ComponentType, ReactNode } from "react";
import type { FlameClient, MutationPromise } from "@flame/runtime-client";
import type {
  CallPluginResponse,
  ChangeStatus,
  SafetyClass,
  Session,
  PluginOperationBinding,
  Target,
  WorkspaceRef,
  PluginPointReference,
} from "@flame/runtime-client/wire";
export type {
  ClientTelemetry,
  TelemetrySnapshot,
  TelemetryAttributes,
  SpanRow,
  MetricRow,
  LogRow,
} from "./telemetry";
export { mutationSettlementIsUnknown } from "@flame/runtime-client";
export { isErrorType } from "@flame/runtime-client";
export { scrollPositions, type ScrollPosition } from "./scroll";
export { failureText } from "./errorText";

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
  readonly status: ChangeStatus;
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
  readonly safetyClass?: SafetyClass;
  readonly range?: Readonly<{ start: number; end: number }>;
  readonly durationMillis?: number;
  readonly approvalDecision?: "approve" | "deny";
}

export type CapturedSession = Readonly<
  Pick<Session, "id" | "revision" | "title">
>;

export interface ComponentContext {
  readonly component: Readonly<ComponentReference>;
  readonly file?: Readonly<CapturedFile>;
  readonly toolResult?: Readonly<CapturedToolResult>;
  readonly runtime: FlameClient;
  readonly telemetry: ClientTelemetry;
  readonly workspace?: Readonly<WorkspaceContext>;
  readonly sessionId?: string;
  readonly sessionTarget?: CapturedSession;
  readonly binding?: Readonly<PluginOperationBinding>;
  // The host owns the active locale; a component projects it, never selects it.
  readonly locale: string;
  readonly signal: AbortSignal;
  track<T>(operation: Promise<T>): Promise<T>;
  compose(composition: ComponentComposition): ReactNode;
}

export interface ComponentReference {
  readonly installationId: string;
  readonly digest: string;
  readonly componentId: string;
}

export interface ComponentComposition {
  readonly contract: string;
  readonly target: Target;
  readonly attributes: unknown;
  readonly order?: readonly PluginPointReference[];
  readonly input?: unknown;
  readonly children?: ReactNode;
}

// The point's consumer owns the value schema, including props. The component
// reference is local to the contributor's exact admitted release.
export interface ComponentValue<Props = unknown> {
  readonly component: string;
  readonly props?: Props;
}

export type PluginComponent<Input = unknown, Props = unknown> = ComponentType<{
  context: ComponentContext;
  input?: Input;
  contribution?: Props;
  children?: ReactNode;
}>;

export type Catalog = Readonly<Record<string, string>>;

export type Translate = (
  key: string,
  params?: Readonly<Record<string, string | number>>,
) => string;

// Mirrors the host i18next conventions: flat dotted keys, {{name}}
// interpolation, _one/_other plural suffixes via Intl.PluralRules on `count`,
// with en as reference and fallback.
export function createTranslator(
  catalogs: Readonly<Record<string, Catalog>>,
  locale: string,
): Translate {
  const base = catalogs.en ?? {};
  const active = catalogs[locale] ?? catalogs[locale.split("-")[0]!] ?? base;
  const plural =
    typeof Intl !== "undefined" && "PluralRules" in Intl
      ? new Intl.PluralRules(locale)
      : undefined;
  return (key, params) => {
    let lookup = key;
    if (params && typeof params.count === "number") {
      const category = plural ? plural.select(params.count) : "other";
      const suffixed = `${key}_${category}`;
      lookup =
        active[suffixed] !== undefined || base[suffixed] !== undefined
          ? suffixed
          : `${key}_other`;
    }
    const template =
      active[lookup] ?? base[lookup] ?? active[key] ?? base[key] ?? key;
    return params
      ? template.replace(/\{\{(\w+)\}\}/g, (whole, name: string) =>
          name in params ? String(params[name]) : whole,
        )
      : template;
  };
}

function output<R>(reply: CallPluginResponse): R {
  if (reply.failure)
    throw new OperationFailure(reply.failure.code, reply.failure.message);
  if (!("output" in reply))
    throw new Error("plugin operation omitted its output");
  return parseJSON(reply.output!) as R;
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
  const captured = structuredClone(target);
  function request<K extends keyof Inputs & string>(
    method: K,
    input: Inputs[K],
    kind: "query" | "command",
    delivery: "unary" | "stream",
  ) {
    context.signal.throwIfAborted();
    const descriptor = pluginMethod({ contract, method });
    if (
      descriptor.streaming !== (delivery === "stream") ||
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
      input: stringifyJSON(input),
      target: captured,
    };
  }
  return {
    pendingCommands: <K extends keyof Inputs & string>(method: K) => {
      context.signal.throwIfAborted();
      return context.runtime.plugins
        .pendingCommands()
        .filter(
          ({ binding: original }) =>
            original.installationId === binding.installationId &&
            original.contribution === binding.contribution &&
            original.method === method &&
            original.target.scope === captured.scope &&
            original.target.reference === captured.reference,
        )
        .map((command) => ({
          idempotencyKey: command.idempotencyKey,
          retire: (signal: AbortSignal = context.signal): Promise<void> => {
            context.signal.throwIfAborted();
            return context.track(
              command.retire(AbortSignal.any([context.signal, signal])),
            );
          },
          recover: (
            signal: AbortSignal = context.signal,
          ): Promise<Outputs[K]> => {
            context.signal.throwIfAborted();
            return context.track(
              command.recover(
                (value) => value as Outputs[K],
                AbortSignal.any([context.signal, signal]),
              ),
            );
          },
        }));
    },
    queryStream: <K extends keyof Inputs & keyof Chunks & string>(
      method: K,
      input: Inputs[K],
      consume: (chunk: Chunks[K], signal: AbortSignal) => void | Promise<void>,
      signal: AbortSignal = context.signal,
    ): Promise<Outputs[K]> =>
      context.track(
        context.runtime.plugins.queryStream(
          request(method, input, "query", "stream"),
          (chunk, signal) => consume(chunk as Chunks[K], signal),
          (value) => value as Outputs[K],
          AbortSignal.any([context.signal, signal]),
        ),
      ),
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
