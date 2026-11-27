import { isErrorType, OperationFailure } from "./errors";
import {
  createMutationPromise,
  MutationLookupError,
  mutationSettlementIsUnknown,
} from "./mutation";
import { MutationJournalError } from "./mutationJournalErrors";
import type {
  PluginCommandInput,
  PluginCommandBinding,
  PluginCommandIdentity,
  PluginCommandReservation,
  ActivePluginCommand,
  MutationReservation,
} from "./mutationJournal";
import type { CallPluginRequest } from "@flame/runtime-client/wire";
import type { WireCallPath } from "./wireCallPath";
import { deliverPluginStream } from "./pluginStream";
import { pluginMethod } from "./request";
import { parseJSON } from "@frame/sdk/json";

export interface PluginCommandRecovery extends PluginCommandIdentity {
  readonly state: "recoverable";
  recover<T>(decode: (output: unknown) => T, signal?: AbortSignal): Promise<T>;
  retire(signal?: AbortSignal): Promise<void>;
}

export type PluginCommand = ActivePluginCommand | PluginCommandRecovery;

export function pluginCommandRecovery(
  command: PluginCommandReservation,
  wire: Pick<WireCallPath, "callUnkeyed" | "dispatchCommand">,
): PluginCommandRecovery {
  return {
    idempotencyKey: command.idempotencyKey,
    binding: command.binding,
    state: command.state,
    matchesIntent: command.matchesIntent,
    recover: async <T>(decode: (output: unknown) => T, signal?: AbortSignal) => {
      const reservation = command.reserve();
      return reservation.track(() =>
        createMutationPromise<T>(
          async (key, attempt) => {
            let request: CallPluginRequest;
            try {
              reservation.authorizeDispatch();
              request = await wire.callUnkeyed(
                "plugins.getCommand",
                {
                  namespace: command.namespace,
                  key,
                },
                { signal: attempt.signal },
              );
              command.validate(request);
            } catch (error) {
              if (
                isErrorType(error, "plugin_command_retired") ||
                error instanceof MutationJournalError ||
                mutationSettlementIsUnknown(error)
              )
                throw error;
              throw new MutationLookupError(error);
            }
            if (pluginMethod(request.route).delivery === "plugins.stream")
              return deliverPluginStream(
                wire,
                request,
                reservation,
                () => undefined,
                decode,
                attempt.signal,
              );
            return deliverPluginCommand(wire, request, reservation, decode, attempt.signal);
          },
          command.idempotencyKey,
          { signal },
        ),
      );
    },
    retire: (signal) => {
      const reservation = command.reserve();
      return reservation.track(() =>
        createMutationPromise<void>(
          async (key, attempt) => {
            try {
              await wire.callUnkeyed(
                "plugins.retireCommand",
                {
                  namespace: reservation.lookupNamespace(),
                  key,
                },
                { signal: attempt.signal },
              );
            } catch (error) {
              throw new MutationLookupError(error);
            }
          },
          command.idempotencyKey,
          { signal },
        ),
      );
    },
  };
}

export async function callPluginCommand<T>(
  wire: Pick<WireCallPath, "dispatchCommand" | "openMutation" | "capturePluginCommand">,
  intent: string,
  input: PluginCommandInput,
  prepare: () => Promise<PluginCommandBinding>,
  decode: (output: unknown) => T,
  signal?: AbortSignal,
): Promise<T> {
  const method = pluginMethod(input);
  if (method.delivery !== "plugins.call" || method.kind !== "command")
    throw new TypeError("Plugin command requires a unary command method");
  parseJSON(input.input);
  const { request, reservation } = await wire.capturePluginCommand(intent, input, prepare, signal);
  return wire.openMutation(
    "plugins.call",
    request,
    (prepared, original, attemptSignal) =>
      deliverPluginCommand(wire, prepared, original, decode, attemptSignal),
    {
      signal,
      original: { idempotencyKey: reservation.idempotencyKey, reserve: () => reservation },
    },
  );
}

async function deliverPluginCommand<T>(
  wire: Pick<WireCallPath, "dispatchCommand">,
  request: CallPluginRequest,
  reservation: MutationReservation,
  decode: (output: unknown) => T,
  signal?: AbortSignal,
): Promise<T> {
  let reply;
  try {
    reply = await wire.dispatchCommand("plugins.call", request, reservation, { signal });
  } catch (error) {
    if (
      isErrorType(error, "plugin_command_retired") ||
      error instanceof MutationJournalError ||
      mutationSettlementIsUnknown(error)
    )
      throw error;
    throw new MutationLookupError(error);
  }
  if (reply.failure) throw new OperationFailure(reply.failure.code, reply.failure.message);
  try {
    return await decode(parseJSON(reply.output!));
  } catch (error) {
    throw new MutationLookupError(error);
  }
}
