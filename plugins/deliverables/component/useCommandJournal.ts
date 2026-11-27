import { useCallback, useMemo, useSyncExternalStore } from "react";
import {
  failureText,
  isErrorType,
  mutationSettlementIsUnknown,
  OperationFailure,
} from "@flame/component-sdk";
import {
  METHODS as DECLARATIONS,
  type MethodInputs,
  type MethodOutputs,
} from "./methods.generated";
import { METHODS } from "../contracts/methods";
import type { CommandMethod } from "./types";
import type { DeliverableOperations } from "./operations";

const commandMethods = (
  Object.keys(DECLARATIONS) as Array<keyof typeof DECLARATIONS>
).filter(
  (method): method is CommandMethod => DECLARATIONS[method].kind === "command",
);

type CommandRecovery = ReturnType<
  DeliverableOperations["pendingCommands"]
>[number];

interface CommandIdentity {
  readonly id: string;
  readonly method: CommandMethod;
  readonly settled?: (result: unknown) => void;
}

type RunningCommand = CommandIdentity & { readonly status: "running" };
type UnknownCommand = CommandIdentity & {
  readonly status: "unknown";
  readonly error?: string;
  readonly recovery?: CommandRecovery;
};

export type CommandObservation = RunningCommand | UnknownCommand;

interface CommandSettlement {
  settle(command: CommandObservation, result: unknown): void;
  fail(error: unknown): void;
}

interface CommandEffect {
  // An announced result the notice offers to open.
  notice?: "asset" | "revision" | "command";
  reload: "views" | "uploads" | "none";
  // Commands that author a document carry the current project scope.
  scoped?: true;
}

export const COMMAND_EFFECTS: Record<CommandMethod, CommandEffect> = {
  [METHODS.create]: {
    notice: "revision",
    reload: "views",
    scoped: true,
  },
  [METHODS.import]: {
    notice: "revision",
    reload: "views",
    scoped: true,
  },
  [METHODS.copy]: {
    notice: "revision",
    reload: "views",
    scoped: true,
  },
  [METHODS.edit]: { notice: "revision", reload: "views" },
  [METHODS.recalculate]: {
    notice: "revision",
    reload: "views",
  },
  [METHODS.restore]: { notice: "revision", reload: "views" },
  [METHODS.comment]: { reload: "views" },
  [METHODS.reply]: { reload: "views" },
  [METHODS.resolve]: { reload: "views" },
  [METHODS.reopen]: { reload: "views" },
  [METHODS.suggest]: { reload: "views" },
  [METHODS.settle]: { reload: "views" },
  [METHODS.export]: { notice: "asset", reload: "none" },
  [METHODS.render]: { notice: "command", reload: "views" },
  [METHODS.review]: { notice: "command", reload: "views" },
  [METHODS.remove]: { notice: "command", reload: "views" },
  [METHODS.discard]: { notice: "command", reload: "uploads" },
  [METHODS.upload]: { reload: "uploads" },
  [METHODS.append]: { reload: "none" },
};

// A failure that does not prove the command was refused leaves its outcome
// open; the observation stays recoverable instead of reporting an error.
function outcomeUnknown(error: unknown, recoverable: boolean): boolean {
  return (
    mutationSettlementIsUnknown(error) ||
    (recoverable &&
      !(error instanceof OperationFailure) &&
      !isErrorType(error, "plugin_command_retired"))
  );
}

class CommandJournal {
  #commands: readonly CommandObservation[];
  readonly #listeners = new Set<() => void>();

  constructor(
    private readonly operations: DeliverableOperations,
    private readonly signal: AbortSignal,
  ) {
    this.#commands = commandMethods.flatMap((method) =>
      operations.pendingCommands(method).map((recovery): UnknownCommand => ({
        id: recovery.idempotencyKey,
        method,
        status: "unknown",
        recovery,
      })),
    );
  }

  snapshot = () => this.#commands;

  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  async run<K extends CommandMethod>(
    method: K,
    input: MethodInputs[K],
    settlement: CommandSettlement,
    settled?: (value: MethodOutputs[K]) => void,
  ): Promise<MethodOutputs[K] | undefined> {
    if (this.signal.aborted) return;
    if (method === METHODS.discard) {
      if (this.#commands.some((command) => command.method === method)) return;
    } else if (this.#commands.length) return;

    try {
      const result = this.operations.command(method, input);
      const command: RunningCommand = {
        id: result.idempotencyKey,
        method,
        status: "running",
        settled: settled
          ? (value) => settled(value as MethodOutputs[K])
          : undefined,
      };
      this.#publish([...this.#commands, command]);
      return await this.#observe(command, () => result, settlement);
    } catch (error) {
      if (!this.signal.aborted) settlement.fail(error);
    }
  }

  async recover(
    original: CommandObservation,
    settlement: CommandSettlement,
  ): Promise<void> {
    if (original.status !== "unknown" || !original.recovery) return;
    const recovery = original.recovery;
    const command = this.#claim(original);
    if (command)
      await this.#observe(
        command,
        () => recovery.recover(this.signal),
        settlement,
        recovery,
      );
  }

  async retire(original: CommandObservation): Promise<void> {
    if (original.status !== "unknown" || !original.recovery) return;
    const command = this.#claim(original);
    if (!command) return;
    try {
      await original.recovery.retire(this.signal);
      this.#replace(command);
    } catch (error) {
      this.#replace(command, { ...original, error: failureText(error) });
    }
  }

  #claim(original: UnknownCommand): RunningCommand | undefined {
    const { id, method, settled } = original;
    const command: RunningCommand = { id, method, settled, status: "running" };
    return this.#replace(original, command) ? command : undefined;
  }

  async #observe<T>(
    command: RunningCommand,
    execute: () => Promise<T>,
    settlement: CommandSettlement,
    previous?: CommandRecovery,
  ): Promise<T | undefined> {
    let result: T;
    try {
      result = await execute();
    } catch (error) {
      if (this.signal.aborted) return;
      const recovery =
        this.operations
          .pendingCommands(command.method)
          .find(({ idempotencyKey }) => idempotencyKey === command.id) ??
        previous;
      if (outcomeUnknown(error, recovery !== undefined)) {
        this.#replace(command, {
          ...command,
          status: "unknown",
          error: failureText(error),
          recovery,
        });
      } else if (this.#replace(command)) settlement.fail(error);
      return;
    }
    if (!this.#replace(command)) return;
    settlement.settle(command, result);
    return result;
  }

  #replace(current: CommandObservation, next?: CommandObservation): boolean {
    if (this.signal.aborted) return false;
    const index = this.#commands.indexOf(current);
    if (index === -1) return false;
    this.#publish(
      next
        ? this.#commands.toSpliced(index, 1, next)
        : this.#commands.toSpliced(index, 1),
    );
    return true;
  }

  #publish(commands: readonly CommandObservation[]) {
    this.#commands = commands;
    for (const listener of this.#listeners) listener();
  }
}

export function useCommandJournal({
  operations,
  signal,
  scoped,
  settle,
  fail,
}: {
  operations: DeliverableOperations;
  signal: AbortSignal;
  scoped: { project?: string };
} & CommandSettlement) {
  const journal = useMemo(
    () => new CommandJournal(operations, signal),
    [operations, signal],
  );
  const commands = useSyncExternalStore(journal.subscribe, journal.snapshot);
  const run = useCallback(
    <K extends CommandMethod>(
      method: K,
      input: MethodInputs[K],
      settled?: (value: MethodOutputs[K]) => void,
    ) =>
      journal.run(
        method,
        COMMAND_EFFECTS[method].scoped ? { ...input, ...scoped } : input,
        { settle, fail },
        settled,
      ),
    [journal, scoped, settle, fail],
  );
  return {
    commands,
    blocked: commands.length > 0,
    run,
    recover: (command: CommandObservation) => {
      void journal.recover(command, { settle, fail }).catch(fail);
    },
    retire: (command: CommandObservation) => journal.retire(command),
  };
}
