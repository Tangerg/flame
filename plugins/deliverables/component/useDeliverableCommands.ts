import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { failureText } from "@flame/component-sdk";
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

type CommandDetail = {
  onConfirmed?: (result: unknown) => void;
  error?: string;
};

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

export function useDeliverableCommands({
  operations,
  signal,
  scoped,
  settle,
  fail,
}: {
  operations: DeliverableOperations;
  signal: AbortSignal;
  scoped: { project?: string };
  settle(method: CommandMethod, result: unknown, followUp?: () => void): void;
  fail(error: unknown): void;
}) {
  const version = useSyncExternalStore(
    operations.subscribeCommands,
    operations.commandsVersion,
  );
  const originals = useMemo(
    () => (signal.aborted ? [] : operations.commands(...commandMethods)),
    [operations, signal, version],
  );
  const [details, setDetails] = useState<ReadonlyMap<string, CommandDetail>>(
    () => new Map(),
  );
  useEffect(() => {
    const keys = new Set(originals.map((command) => command.idempotencyKey));
    setDetails((previous) => {
      const retained = new Map([...previous].filter(([key]) => keys.has(key)));
      return retained.size === previous.size ? previous : retained;
    });
  }, [originals]);

  function updateDetail(id: string, detail: CommandDetail) {
    setDetails((previous) =>
      new Map(previous).set(id, { ...previous.get(id), ...detail }),
    );
  }

  function reportFailure(id: string, method: CommandMethod, error: unknown) {
    const original = operations
      .commands(method)
      .find((command) => command.idempotencyKey === id);
    if (original?.state === "recoverable")
      updateDetail(id, { error: failureText(error) });
    else fail(error);
  }

  async function observe<T>(
    id: string,
    method: CommandMethod,
    attempt: () => Promise<T>,
    onConfirmed?: (result: T) => void,
  ): Promise<T | undefined> {
    let result: T;
    try {
      result = await attempt();
    } catch (error) {
      if (!signal.aborted) reportFailure(id, method, error);
      return;
    }
    if (signal.aborted) return;
    settle(method, result, onConfirmed && (() => onConfirmed(result)));
    return result;
  }

  async function run<K extends CommandMethod>(
    method: K,
    input: MethodInputs[K],
    onConfirmed?: (value: MethodOutputs[K]) => void,
  ) {
    if (signal.aborted) return;
    try {
      const original = operations.commands(...commandMethods);
      if (
        method === METHODS.discard
          ? original.some((command) => command.method === method)
          : original.length > 0
      )
        return;
      const result = operations.command(
        method,
        COMMAND_EFFECTS[method].scoped ? { ...input, ...scoped } : input,
      );
      const id = result.idempotencyKey;
      if (onConfirmed)
        updateDetail(id, {
          onConfirmed: (value) => onConfirmed(value as MethodOutputs[K]),
        });
      return await observe(id, method, () => result, onConfirmed);
    } catch (error) {
      if (!signal.aborted) fail(error);
    }
  }

  async function recover(id: string, method: CommandMethod) {
    if (signal.aborted) return;
    try {
      const original = operations
        .commands(method)
        .find((command) => command.idempotencyKey === id);
      if (original?.state !== "recoverable") return;
      await observe(
        id,
        method,
        () => original.recover(signal),
        details.get(id)?.onConfirmed,
      );
    } catch (error) {
      if (!signal.aborted) fail(error);
    }
  }

  async function retire(id: string, method: CommandMethod) {
    if (signal.aborted) return;
    try {
      const original = operations
        .commands(method)
        .find((command) => command.idempotencyKey === id);
      if (original?.state !== "recoverable") return;
      await original.retire(signal).catch((error: unknown) => {
        if (!signal.aborted) reportFailure(id, method, error);
      });
    } catch (error) {
      if (!signal.aborted) fail(error);
    }
  }

  return {
    commands: originals.map(({ idempotencyKey: id, method, state }) => ({
      id,
      method,
      state,
      error: state === "recoverable" ? details.get(id)?.error : undefined,
    })),
    blocked: originals.length > 0,
    run,
    recover,
    retire,
  };
}
