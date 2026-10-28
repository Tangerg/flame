import { isErrorType, OperationFailure } from "./errors";
import {
  createMutationPromise,
  MutationLookupError,
  mutationSettlementIsUnknown,
} from "./mutation";
import { MutationJournalError } from "./durableMutationJournal";
import type {
  MutationJournal,
  PluginCommandInput,
  PluginCommandBinding,
  PendingPluginCommand,
  MutationReservation,
} from "./mutationJournal";
import type { CallPluginRequest } from "@flame/runtime-client/wire";
import type { WireCallPath } from "./wireCallPath";
import type { RpcClient } from "./client";
import { deliverPluginStream } from "./pluginStream";
import { pluginDeliveryMethod } from "./request";
import { parseJSON } from "@qimu/sdk/json";

export interface PluginCommandRecovery {
  readonly idempotencyKey: string;
  readonly binding: Omit<CallPluginRequest, "input">;
  matchesIntent(intent: string): boolean;
  recover<T>(decode: (output: unknown) => T, signal?: AbortSignal): Promise<T>;
  retire(signal?: AbortSignal): Promise<void>;
}

export function pendingPluginCommand(
  command: PendingPluginCommand,
  client: RpcClient,
  wire: Pick<WireCallPath, "perform">,
): PluginCommandRecovery {
  return {
    idempotencyKey: command.idempotencyKey,
    binding: command.binding,
    matchesIntent: (intent) => command.matchesIntent(intent),
    recover: async <T>(decode: (output: unknown) => T, signal?: AbortSignal) => {
      const reservation = command.reserve();
      return reservation.track(() =>
        createMutationPromise<T>(
          async (key, attempt) => {
            let request: CallPluginRequest;
            try {
              reservation.authorizeDispatch();
              request = await wire.perform(
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
            if (pluginDeliveryMethod(request) === "plugins.stream")
              return deliverPluginStream(
                client,
                wire,
                request,
                reservation,
                key,
                () => undefined,
                decode,
                attempt.signal,
              );
            return deliverPluginCommand(wire, request, reservation, key, decode, attempt.signal);
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
              await wire.perform(
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
  journal: MutationJournal,
  wire: Pick<WireCallPath, "perform" | "openMutation">,
  intent: string,
  input: PluginCommandInput,
  prepare: () => Promise<PluginCommandBinding>,
  decode: (output: unknown) => T,
  signal?: AbortSignal,
): Promise<T> {
  if (pluginDeliveryMethod(input) !== "plugins.call")
    throw new TypeError("Unary plugin command requires a unary method");
  parseJSON(input.input);
  const { request, reservation } = await journal.capturePluginCommand(intent, input, prepare);
  return wire.openMutation(
    "plugins.call",
    request,
    (key, attempt, prepared) =>
      deliverPluginCommand(wire, prepared, reservation, key, decode, attempt.signal),
    signal,
    undefined,
    reservation.idempotencyKey,
    () => reservation,
  );
}

async function deliverPluginCommand<T>(
  wire: Pick<WireCallPath, "perform">,
  request: CallPluginRequest,
  reservation: MutationReservation,
  key: string,
  decode: (output: unknown) => T,
  signal?: AbortSignal,
): Promise<T> {
  let reply;
  try {
    reply = await wire.perform("plugins.call", request, {
      signal,
      idempotencyKey: key,
      idempotencyNamespace: reservation.authorizeDispatch(),
    });
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
