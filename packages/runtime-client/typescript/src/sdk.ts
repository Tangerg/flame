import { createRpcClient, type RpcClient } from "./client";
import { createMethods, type Methods, type MethodsOptions } from "./methods";
import type { Transport } from "./transport";
import { createMutationJournal } from "./mutationJournal";

export interface FlameClient extends Methods {
  queryJSON: RpcClient["callJSON"];
  close(): Promise<void>;
}

export function createFlameClient(transport: Transport, opts?: MethodsOptions): FlameClient {
  const mutationJournal =
    opts?.mutationJournal ??
    createMutationJournal({
      scope: () => opts?.capabilities?.()?.limits.idempotency,
    });
  const rpc = createRpcClient(transport, { requestMeta: opts?.requestMeta });
  let closePromise: Promise<void> | undefined;
  const close = (): Promise<void> => {
    closePromise ??= (async () => {
      let journalFailure: unknown;
      try {
        mutationJournal.dispose();
      } catch (error) {
        journalFailure = error;
      }
      let transportFailure: unknown;
      try {
        await rpc.close();
      } catch (error) {
        transportFailure = error;
      }
      if (journalFailure !== undefined && transportFailure !== undefined) {
        throw new AggregateError(
          [journalFailure, transportFailure],
          "Runtime client ownership and transport cleanup both failed",
        );
      }
      if (journalFailure !== undefined) throw journalFailure;
      if (transportFailure !== undefined) throw transportFailure;
    })();
    return closePromise;
  };
  return Object.assign(createMethods(rpc, { ...opts, mutationJournal }), {
    close,
    queryJSON: rpc.callJSON,
  });
}
