import { useCallback, useEffect, useMemo } from "react";
import {
  failureText,
  isErrorType,
  mutationSettlementIsUnknown,
  OperationFailure,
  useContextState,
  type ComponentContext,
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

export interface CommandObservation {
  id: string;
  method: CommandMethod;
  status: "running" | "unknown";
  error?: string;
  recover?: () => Promise<unknown>;
  retire?: () => Promise<void>;
  settled?: (result: unknown) => void;
}

interface CommandEffect {
  // An announced result the notice offers to open.
  result?: "asset" | "revision";
  announced: boolean;
  reload: "views" | "uploads" | "none";
  // Commands that author a document carry the current project scope.
  scoped?: true;
}

export const COMMAND_EFFECTS: Record<CommandMethod, CommandEffect> = {
  [METHODS.create]: {
    result: "revision",
    announced: true,
    reload: "views",
    scoped: true,
  },
  [METHODS.import]: {
    result: "revision",
    announced: true,
    reload: "views",
    scoped: true,
  },
  [METHODS.copy]: {
    result: "revision",
    announced: true,
    reload: "views",
    scoped: true,
  },
  [METHODS.edit]: { result: "revision", announced: true, reload: "views" },
  [METHODS.recalculate]: {
    result: "revision",
    announced: true,
    reload: "views",
  },
  [METHODS.restore]: { result: "revision", announced: true, reload: "views" },
  [METHODS.comment]: { announced: false, reload: "views" },
  [METHODS.reply]: { announced: false, reload: "views" },
  [METHODS.resolve]: { announced: false, reload: "views" },
  [METHODS.reopen]: { announced: false, reload: "views" },
  [METHODS.suggest]: { announced: false, reload: "views" },
  [METHODS.settle]: { announced: false, reload: "views" },
  [METHODS.export]: { result: "asset", announced: true, reload: "none" },
  [METHODS.render]: { announced: true, reload: "views" },
  [METHODS.review]: { announced: true, reload: "views" },
  [METHODS.remove]: { announced: true, reload: "views" },
  [METHODS.discard]: { announced: true, reload: "uploads" },
  [METHODS.upload]: { announced: false, reload: "uploads" },
  [METHODS.append]: { announced: false, reload: "none" },
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

export function useCommandJournal({
  context,
  operations,
  signal,
  scoped,
  settle,
  fail,
}: {
  context: ComponentContext;
  operations: DeliverableOperations;
  signal: AbortSignal;
  scoped: { project?: string };
  settle: (command: CommandObservation, result: unknown) => void;
  fail: (error: unknown) => void;
}) {
  const commands = useMemo(() => {
    const entries = new Map<string, CommandObservation>();
    for (const method of commandMethods)
      for (const original of operations.pendingCommands(method)) {
        entries.set(original.idempotencyKey, {
          id: original.idempotencyKey,
          method,
          status: "unknown",
          recover: () => original.recover(signal),
          retire: () => original.retire(signal),
        });
      }
    return entries;
  }, [operations, signal]);
  const [published, setPublished] =
    useContextState<CommandObservation[]>(context);
  const publish = useCallback(() => {
    if (!signal.aborted) setPublished([...commands.values()]);
  }, [commands, setPublished, signal]);
  useEffect(publish, [publish]);

  const observe = useCallback(
    async (
      original: CommandObservation,
      promise: Promise<unknown>,
    ): Promise<unknown> => {
      commands.set(original.id, {
        ...original,
        status: "running",
        error: undefined,
      });
      publish();
      try {
        const result = await promise;
        if (signal.aborted) return;
        commands.delete(original.id);
        settle(original, result);
        return result;
      } catch (error) {
        if (signal.aborted) return;
        const pending = operations
          .pendingCommands(original.method)
          .find((entry) => entry.idempotencyKey === original.id);
        if (outcomeUnknown(error, original.recover !== undefined)) {
          commands.set(original.id, {
            ...original,
            status: "unknown",
            error: failureText(error),
            recover: pending ? () => pending.recover(signal) : original.recover,
            retire: pending ? () => pending.retire(signal) : original.retire,
          });
        } else {
          commands.delete(original.id);
          fail(error);
        }
      } finally {
        publish();
      }
    },
    [commands, fail, operations, publish, settle, signal],
  );

  const run = useCallback(
    async <K extends CommandMethod>(
      method: K,
      input: MethodInputs[K],
      settled?: (value: MethodOutputs[K]) => void,
    ): Promise<MethodOutputs[K] | undefined> => {
      if (
        signal.aborted ||
        (commands.size && method !== METHODS.discard) ||
        [...commands.values()].some(
          (entry) =>
            entry.method === METHODS.discard && method === METHODS.discard,
        )
      )
        return;
      try {
        const promise = operations.command(
          method,
          COMMAND_EFFECTS[method].scoped ? { ...input, ...scoped } : input,
        );
        return (await observe(
          {
            id: promise.idempotencyKey,
            method,
            status: "running",
            settled: settled
              ? (value) => settled(value as MethodOutputs[K])
              : undefined,
          },
          promise,
        )) as MethodOutputs[K] | undefined;
      } catch (error) {
        fail(error);
      }
    },
    [commands, fail, observe, operations, scoped, signal],
  );

  return {
    commands: published ?? [...commands.values()],
    blocked: commands.size > 0,
    run,
    recover: (command: CommandObservation) => {
      if (!signal.aborted && command.status === "unknown" && command.recover)
        void observe(command, command.recover());
    },
    retire: async (command: CommandObservation) => {
      if (signal.aborted || command.status !== "unknown" || !command.retire)
        return;
      commands.set(command.id, {
        ...command,
        status: "running",
        error: undefined,
      });
      publish();
      try {
        await command.retire();
        if (signal.aborted) return;
        commands.delete(command.id);
      } catch (error) {
        if (!signal.aborted)
          commands.set(command.id, { ...command, error: failureText(error) });
      } finally {
        publish();
      }
    },
  };
}
