import { parseJSON, stringifyJSON } from "@frame/sdk/json";
import {
  OperationFailure,
  pluginMethod,
  type MutationPromise,
} from "@flame/runtime-client";
import type { CallPluginResponse, Target } from "@flame/runtime-client/wire";
import type { ComponentContext } from "./index";

function output<R>(reply: CallPluginResponse): R {
  if (reply.failure)
    throw new OperationFailure(reply.failure.code, reply.failure.message);
  if (!("output" in reply))
    throw new Error("plugin operation omitted its output");
  return parseJSON(reply.output!) as R;
}

type OperationContext = Pick<
  ComponentContext,
  "binding" | "runtime" | "signal" | "track"
>;

type BoundCommand<K extends string, O> = {
  readonly method: K;
  readonly idempotencyKey: string;
} & (
  | { readonly state: "active" }
  | {
      readonly state: "recoverable";
      recover(signal?: AbortSignal): Promise<O>;
      retire(signal?: AbortSignal): Promise<void>;
    }
);

// The typed output must retain the reservation used by an uncertain delivery.
function mapMutation<R>(
  source: MutationPromise<CallPluginResponse>,
  context: OperationContext,
): MutationPromise<R> {
  const mapped = Object.defineProperties(source.then(output<R>), {
    idempotencyKey: { enumerable: true, value: source.idempotencyKey },
    retry: {
      enumerable: true,
      value: (options?: { signal?: AbortSignal }) => {
        context.signal.throwIfAborted();
        return context.track(() =>
          mapMutation<R>(
            source.retry({
              signal: options?.signal
                ? AbortSignal.any([context.signal, options.signal])
                : context.signal,
            }),
            context,
          ),
        );
      },
    },
  }) as MutationPromise<R>;
  return mapped;
}

export function bindOperations<
  Inputs,
  Outputs extends { [K in keyof Inputs]: unknown },
  Chunks extends { [K in keyof Inputs]?: unknown } = {},
>(context: OperationContext, contract: string, target: Target) {
  if (!context.binding)
    throw new Error("component has no declared callable contribution");
  const binding = structuredClone(context.binding);
  const captured = structuredClone(target);
  const journal = context.runtime.plugins;
  function request<K extends keyof Inputs & string>(
    method: K,
    input: Inputs[K],
    kind: "query" | "command",
    delivery: ReturnType<typeof pluginMethod>["delivery"],
  ) {
    context.signal.throwIfAborted();
    const descriptor = pluginMethod({ contract, method });
    if (
      descriptor.delivery !== delivery ||
      descriptor.kind !== kind ||
      descriptor.target !== captured.scope
    ) {
      throw new Error(
        "plugin operation differs from its required contract or target",
      );
    }
    return {
      route: {
        installationId: binding.installationId,
        digest: binding.digest,
        contribution: binding.contribution,
        contract,
        method,
        target: captured,
      },
      input: stringifyJSON(input),
    };
  }
  function commands<K extends keyof Inputs & string>(
    ...methods: readonly K[]
  ): { [M in K]: BoundCommand<M, Outputs[M]> }[K][] {
    context.signal.throwIfAborted();
    return journal
      .commands()
      .flatMap<BoundCommand<K, Outputs[K]>>((command) => {
        const original = command.binding;
        const method = methods.find((method) => method === original.method);
        if (
          method === undefined ||
          original.installationId !== binding.installationId ||
          original.digest !== binding.digest ||
          original.contract !== contract ||
          original.contribution !== binding.contribution ||
          original.target.scope !== captured.scope ||
          original.target.reference !== captured.reference
        )
          return [];
        const identity = { method, idempotencyKey: command.idempotencyKey };
        if (command.state === "active")
          return [{ ...identity, state: "active" }];
        return [
          {
            ...identity,
            state: "recoverable",
            retire: (signal = context.signal) => {
              context.signal.throwIfAborted();
              return context.track(() =>
                command.retire(AbortSignal.any([context.signal, signal])),
              );
            },
            recover: (signal = context.signal) => {
              context.signal.throwIfAborted();
              return context.track(() =>
                command.recover(
                  (value) => value as Outputs[K],
                  AbortSignal.any([context.signal, signal]),
                ),
              );
            },
          },
        ];
      }) as { [M in K]: BoundCommand<M, Outputs[M]> }[K][];
  }
  return {
    commands,
    commandsVersion: journal.commandsVersion,
    subscribeCommands: journal.subscribeCommands,
    authoring: (name: string) => {
      const intent = `${name}/${binding.installationId}`;
      const originalCommand = () => {
        context.signal.throwIfAborted();
        const original = journal
          .commands()
          .find((command) => command.matchesIntent(intent));
        if (!original) return;
        const identity = {
          method: original.binding.method,
          idempotencyKey: original.idempotencyKey,
        };
        if (original.state === "active")
          return { ...identity, state: "active" } as const;
        return {
          ...identity,
          state: "recoverable",
          recover: () =>
            context.track((signal) =>
              original.recover(
                (value) => value as Outputs[keyof Inputs],
                signal,
              ),
            ),
        } as const;
      };
      return {
        originalCommand,
        commandsVersion: journal.commandsVersion,
        subscribeCommands: journal.subscribeCommands,
        submit<K extends keyof Inputs & string>(
          method: K,
          input: Inputs[K],
        ): Promise<Outputs[K]> | undefined {
          if (originalCommand()) return;
          const invocation = request(method, input, "command", "plugins.call");
          const { contract, method: name, ...binding } = invocation.route;
          return context.track((signal) =>
            context.runtime.plugins.command(
              intent,
              { contract, method: name, input: invocation.input },
              async () => binding,
              (value) => value as Outputs[K],
              signal,
            ),
          );
        },
      };
    },
    queryStream: <K extends keyof Inputs & keyof Chunks & string>(
      method: K,
      input: Inputs[K],
      consume: (chunk: Chunks[K], signal: AbortSignal) => void | Promise<void>,
      signal: AbortSignal = context.signal,
    ): Promise<Outputs[K]> =>
      context.track(() =>
        context.runtime.plugins.queryStream(
          request(method, input, "query", "plugins.stream"),
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
      const invocation = request(method, input, "command", "plugins.stream");
      const { contract, method: name, ...binding } = invocation.route;
      return context.track(() =>
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
      context.track<Promise<Outputs[K]>>(() =>
        context.runtime.plugins
          .call(request(method, input, "query", "plugins.call"), {
            signal: AbortSignal.any([context.signal, signal]),
          })
          .then(output<Outputs[K]>),
      ),
    command: <K extends keyof Inputs & string>(
      method: K,
      input: Inputs[K],
    ): MutationPromise<Outputs[K]> =>
      context.track(() => {
        const mutation = context.runtime.plugins.call(
          request(method, input, "command", "plugins.call"),
          { signal: context.signal },
        );
        if (!("retry" in mutation) || !("idempotencyKey" in mutation))
          throw new Error("Runtime omitted the command identity");
        return mapMutation<Outputs[K]>(
          mutation as MutationPromise<CallPluginResponse>,
          context,
        );
      }),
  };
}
