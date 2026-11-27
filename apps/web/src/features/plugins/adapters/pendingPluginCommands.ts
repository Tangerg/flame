import { stringifyJSON } from "@frame/sdk/json";
import {
  isErrorType,
  OperationFailure,
  type Methods,
  type PluginCommandRecovery,
} from "@flame/runtime-client";

type PendingRead<Command> =
  | { readonly type: "ready"; readonly commands: readonly Command[] }
  | { readonly type: "failed"; readonly error: unknown };

export type PluginCommandOutcome =
  | { readonly type: "confirmed"; readonly key: string; readonly output: string }
  | { readonly type: "retired"; readonly key: string }
  | { readonly type: "refused" | "unknown"; readonly key: string; readonly error: unknown };

type Observation = { readonly type: "checking"; readonly key: string } | PluginCommandOutcome;
type PendingCommand = Pick<PluginCommandRecovery, "idempotencyKey" | "binding">;
type PendingSnapshot = Readonly<{
  read: PendingRead<PendingCommand>;
  observation?: Observation;
}>;

export class PendingPluginCommands {
  #snapshot?: PendingSnapshot;
  readonly #listeners = new Set<() => void>();

  constructor(
    private readonly signal: AbortSignal,
    private readonly journal: Pick<
      Methods["plugins"],
      "pendingCommands" | "subscribePendingCommands"
    >,
    private readonly track: <P extends Promise<unknown>>(
      operation: (signal: AbortSignal) => P,
    ) => P,
  ) {
    if (signal.aborted) return;
    const stop = journal.subscribePendingCommands(this.refresh);
    signal.addEventListener("abort", stop, { once: true });
    this.refresh();
  }

  snapshot = (): PendingSnapshot | undefined => this.#snapshot;

  subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  refresh = (): void => {
    if (!this.signal.aborted) this.#publish(this.#read(), this.#snapshot?.observation);
  };

  async observe(key: string, action: "recover" | "retire"): Promise<void> {
    if (this.signal.aborted || this.#snapshot?.observation?.type === "checking") return;
    // Another plugin view can settle the original after this panel lists it.
    const read = this.#read();
    const original =
      read.type === "ready"
        ? read.commands.find((command) => command.idempotencyKey === key)
        : undefined;
    if (!original) {
      this.#publish(read, this.#snapshot?.observation);
      return;
    }
    this.#publish(read, { type: "checking", key });
    let outcome: PluginCommandOutcome;
    try {
      if (action === "retire") {
        await this.track((signal) => original.retire(signal));
        outcome = { type: "retired", key };
      } else {
        const output = await this.track((signal) => original.recover(stringifyJSON, signal));
        outcome = { type: "confirmed", key, output };
      }
    } catch (error) {
      outcome = isErrorType(error, "plugin_command_retired")
        ? { type: "retired", key }
        : { type: error instanceof OperationFailure ? "refused" : "unknown", key, error };
    }
    if (!this.signal.aborted) this.#publish(this.#read(), outcome);
  }

  #read(): PendingRead<PluginCommandRecovery> {
    try {
      return { type: "ready", commands: this.journal.pendingCommands() };
    } catch (error) {
      return { type: "failed", error };
    }
  }

  #publish(read: PendingRead<PluginCommandRecovery>, observation?: Observation): void {
    this.#snapshot = {
      read:
        read.type === "ready"
          ? {
              type: "ready",
              commands: read.commands.map(({ idempotencyKey, binding }) => ({
                idempotencyKey,
                binding,
              })),
            }
          : read,
      observation,
    };
    for (const listener of this.#listeners) listener();
  }
}
