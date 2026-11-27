import {
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react";
import { parseJSON } from "@frame/sdk/json";
import { bindOperations, failureText } from "@flame/component-sdk";
import { METHODS } from "../contracts/methods";
import {
  METHODS as declarations,
  type MethodChunks,
  type MethodInputs,
  type MethodOutputs,
} from "./methods.generated";

type Method = keyof typeof declarations;
type CommandMethod = {
  [M in Method]: (typeof declarations)[M]["kind"] extends "command" ? M : never;
}[Method];

const commandMethods = (Object.keys(declarations) as Method[]).filter(
  (method): method is CommandMethod => declarations[method].kind === "command",
);

type Operations = ReturnType<
  typeof bindOperations<MethodInputs, MethodOutputs, MethodChunks>
>;
type CommandDetail = { target?: string; error?: string };
type ResourceCoordinates = {
  sessionId?: unknown;
  workerId?: unknown;
  surfaceDigest?: unknown;
  operationId?: unknown;
  requestId?: unknown;
};

function commandTarget(
  method: CommandMethod,
  input: unknown,
): string | undefined {
  if (!input || typeof input !== "object" || Array.isArray(input)) return;
  const { sessionId, workerId, surfaceDigest, operationId, requestId } =
    input as ResourceCoordinates;
  let parts: readonly unknown[];
  switch (method) {
    case METHODS.open:
      parts = [];
      break;
    case METHODS.close:
      parts = [sessionId];
      break;
    case METHODS.stop:
      parts = [sessionId, operationId];
      break;
    case METHODS.reply:
      parts = [sessionId, requestId];
      break;
    case METHODS.execute:
      parts = [sessionId, workerId, surfaceDigest];
      break;
  }
  if (parts.every((part) => typeof part === "string" && part.length > 0))
    return JSON.stringify(parts);
}

export function useComputerCommands(
  operations: Operations,
  signal: AbortSignal,
  current: ResourceCoordinates,
  onConfirmed: () => void,
) {
  const version = useSyncExternalStore(
    operations.subscribeCommands,
    operations.commandsVersion,
  );
  const readCommands = useCallback(
    () => (signal.aborted ? [] : operations.commands(...commandMethods)),
    [operations, signal],
  );
  const originalCommands = useMemo(readCommands, [readCommands, version]);
  const [details, setDetails] = useState<ReadonlyMap<string, CommandDetail>>(
    () => new Map(),
  );
  const [notice, setNotice] = useState<
    { kind: "success"; method: CommandMethod } | { kind: "error"; text: string }
  >();

  const updateDetail = useCallback((id: string, detail: CommandDetail) => {
    setDetails((previous) =>
      new Map(previous).set(id, { ...previous.get(id), ...detail }),
    );
  }, []);

  const readOriginalTargets = useEffectEvent((lifetime: AbortSignal) => {
    for (const { idempotencyKey: id, method, state } of originalCommands) {
      if (
        state !== "recoverable" ||
        method === METHODS.open ||
        details.get(id)?.target !== undefined
      )
        continue;
      void operations
        .query(METHODS.result, { id }, lifetime)
        .then((record) => {
          if (lifetime.aborted) return;
          if (
            record.id !== id ||
            record.kind !== "command" ||
            record.method !== method
          )
            throw new Error(
              "computer: original command evidence differs from its identity",
            );
          const target = commandTarget(method, parseJSON(record.inputJSON));
          if (target === undefined)
            throw new Error("computer: original command target is unavailable");
          updateDetail(id, { target });
        })
        .catch((error: unknown) => {
          if (!lifetime.aborted)
            setDetails((previous) => {
              const detail = previous.get(id);
              if (detail?.error !== undefined) return previous;
              return new Map(previous).set(id, {
                ...detail,
                error: failureText(error),
              });
            });
        });
    }
  });

  useEffect(() => {
    const keys = new Set(
      originalCommands.map((command) => command.idempotencyKey),
    );
    setDetails((previous) => {
      const retained = new Map([...previous].filter(([key]) => keys.has(key)));
      return retained.size === previous.size ? previous : retained;
    });
    if (signal.aborted) return;
    const request = new AbortController();
    readOriginalTargets(AbortSignal.any([signal, request.signal]));
    return () => request.abort();
  }, [originalCommands, signal]);

  function blocked(
    original: ReturnType<typeof readCommands>,
    method: CommandMethod,
    target: string | undefined,
  ) {
    return original.some((command) => {
      if (command.method !== method) return false;
      const originalTarget = details.get(command.idempotencyKey)?.target;
      if (originalTarget !== undefined) return originalTarget === target;
      // Revocation and permission controls can proceed for a new resource
      // even when an earlier command has no readable target evidence.
      return method === METHODS.execute || method === METHODS.open;
    });
  }

  async function observe(
    id: string,
    method: CommandMethod,
    execute: () => Promise<unknown>,
  ): Promise<boolean> {
    setNotice(undefined);
    try {
      await execute();
    } catch (error) {
      if (signal.aborted) return false;
      const text = failureText(error);
      const retained = operations
        .commands(method)
        .find((command) => command.idempotencyKey === id);
      if (retained?.state === "recoverable") updateDetail(id, { error: text });
      else setNotice({ kind: "error", text });
      return false;
    }
    if (signal.aborted) return false;
    setNotice({ kind: "success", method });
    onConfirmed();
    return true;
  }

  async function submit<K extends CommandMethod>(
    method: K,
    input: MethodInputs[K],
  ) {
    if (signal.aborted) return false;
    try {
      const target = commandTarget(method, input);
      if (target === undefined)
        throw new Error(
          "computer: command requires its captured resource target",
        );
      if (blocked(readCommands(), method, target)) return false;
      const result = operations.command(method, input);
      updateDetail(result.idempotencyKey, { target });
      return await observe(result.idempotencyKey, method, () => result);
    } catch (error) {
      if (!signal.aborted)
        setNotice({ kind: "error", text: failureText(error) });
      return false;
    }
  }

  async function recover(id: string) {
    if (signal.aborted) return;
    try {
      const original = readCommands().find(
        (command) => command.idempotencyKey === id,
      );
      if (original?.state === "recoverable")
        await observe(id, original.method, () => original.recover(signal));
    } catch (error) {
      if (!signal.aborted)
        setNotice({ kind: "error", text: failureText(error) });
    }
  }

  return {
    notice,
    commands: originalCommands.map(({ idempotencyKey: id, method, state }) => ({
      id,
      method,
      state,
      error: state === "recoverable" ? details.get(id)?.error : undefined,
    })),
    submit,
    busy: (method: CommandMethod) =>
      blocked(originalCommands, method, commandTarget(method, current)),
    recover,
  };
}
