import { OperationFailure } from "./errors";
import { MutationLookupError, mutationSettlementIsUnknown } from "./mutation";
import { MutationJournalError } from "./durableMutationJournal";
import type { MutationJournal, PluginCommandInput, PluginCommandBinding } from "./mutationJournal";
import type { WireCallPath } from "./wireCallPath";

export async function callPluginCommand<T>(
  journal: MutationJournal,
  wire: Pick<WireCallPath, "perform" | "openMutation">,
  intent: string,
  input: PluginCommandInput,
  prepare: () => Promise<PluginCommandBinding>,
  decode: (output: unknown) => T,
  signal?: AbortSignal,
): Promise<T> {
  const { request, reservation } = await journal.capturePluginCommand(intent, input, prepare);
  return reservation.track(() =>
    wire.openMutation(
      "plugins.call",
      request,
      async (key, attempt, prepared) => {
        let reply;
        try {
          reply = await wire.perform("plugins.call", prepared, {
            signal: attempt.signal,
            idempotencyKey: key,
            idempotencyNamespace: reservation.authorizeDispatch(),
          });
        } catch (error) {
          if (error instanceof MutationJournalError || mutationSettlementIsUnknown(error))
            throw error;
          throw new MutationLookupError(error);
        }
        if (reply.failure) throw new OperationFailure(reply.failure.code, reply.failure.message);
        try {
          return await decode(reply.output);
        } catch (error) {
          throw new MutationLookupError(error);
        }
      },
      signal,
      reservation.idempotencyKey,
    ),
  );
}
