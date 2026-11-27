import type { ComponentContext } from "@flame/component-sdk";
import { stringifyJSON } from "@frame/sdk/json";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";
import { isAuthoringMethod, type AuthoringMethod } from "./methods";
import { type Command } from "./types";

type CommandContext = Pick<
  ComponentContext,
  "binding" | "runtime" | "signal" | "track"
>;

export function bindScheduleCommands(context: CommandContext) {
  if (!context.binding)
    throw new Error("Schedules requires its own callable contribution");
  const { installationId, digest, contribution } = context.binding;
  const target = { scope: "installation", reference: installationId } as const;
  const intent = `flame.schedules/authoring/${installationId}`;
  const pendingCommand = () => {
    context.signal.throwIfAborted();
    const original = context.runtime.plugins
      .pendingCommands()
      .find((command) => command.matchesIntent(intent));
    if (!original) return;
    const { method } = original.binding;
    if (!isAuthoringMethod(method)) return;
    return {
      method,
      idempotencyKey: original.idempotencyKey,
      recover: () =>
        context.track((signal) =>
          original.recover(
            (output) => output as MethodOutputs[AuthoringMethod],
            signal,
          ),
        ),
    };
  };
  return {
    target,
    pendingCommand,
    submit<K extends AuthoringMethod>(
      method: K,
      input: MethodInputs[K],
    ): Promise<MethodOutputs[K]> | undefined {
      if (pendingCommand()) return;
      return context.track((signal) =>
        context.runtime.plugins.command(
          intent,
          { contract: CAPABILITY_JSON, method, input: stringifyJSON(input) },
          async () => ({ installationId, digest, contribution, target }),
          (output) => output as MethodOutputs[K],
          signal,
        ),
      );
    },
  };
}

export class ScheduleCommandObservation {
  #active?: Command;
  readonly #listeners = new Set<() => void>();

  constructor(private readonly lifetime: AbortSignal) {}

  snapshot = () => this.#active;

  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  async run(command: Command, observe: () => Promise<void>): Promise<void> {
    if (this.#active || this.lifetime.aborted) return;
    this.#publish(command);
    try {
      await observe();
    } finally {
      this.#publish(undefined);
    }
  }

  #publish(command: Command | undefined) {
    this.#active = command;
    for (const listener of this.#listeners) listener();
  }
}
