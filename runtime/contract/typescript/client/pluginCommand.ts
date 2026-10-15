import { OperationFailure } from "./errors";
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
import type { CallPluginRequest } from "@flame/runtime-contract/wire";
import type { WireCallPath } from "./wireCallPath";
import { pluginDeliveryMethod } from "./request";
import { parseJSON } from "./json";

export interface PluginCommandRecovery {
  readonly idempotencyKey: string;
  readonly binding: Omit<CallPluginRequest, "input">;
  recover<T>(decode: (output: unknown) => T, signal?: AbortSignal): Promise<T>;
}

export function pendingPluginCommand(
  command: PendingPluginCommand,
  wire: Pick<WireCallPath, "perform">,
): PluginCommandRecovery {
  return {
    idempotencyKey: command.idempotencyKey,
    binding: command.binding,
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
              if (error instanceof MutationJournalError || mutationSettlementIsUnknown(error))
                throw error;
              throw new MutationLookupError(error);
            }
            return deliverPluginCommand(wire, request, reservation, key, decode, attempt.signal);
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
  return reservation.track(() =>
    wire.openMutation(
      "plugins.call",
      request,
      (key, attempt, prepared) =>
        deliverPluginCommand(wire, prepared, reservation, key, decode, attempt.signal),
      signal,
      reservation.idempotencyKey,
    ),
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
    if (error instanceof MutationJournalError || mutationSettlementIsUnknown(error)) throw error;
    throw new MutationLookupError(error);
  }
  if (reply.failure) throw new OperationFailure(reply.failure.code, reply.failure.message);
  try {
    return await decode(parseJSON(reply.output!));
  } catch (error) {
    throw new MutationLookupError(error);
  }
}
