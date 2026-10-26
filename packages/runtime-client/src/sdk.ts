import { createRpcClient, type RpcClient } from "./client";
import { createMethods, type Methods } from "./methods";
import type { Transport } from "./transport";
import { createMutationJournal, type MutationJournalStorage } from "./mutationJournal";
import type { RequestMeta, ServerCapabilities } from "@flame/runtime-client/wire";

export interface FlameClient extends Methods {
  queryJSON: RpcClient["callJSON"];
  close(): Promise<void>;
}

export interface FlameClientOptions {
  capabilities?: () => ServerCapabilities | null | undefined;
  requestMeta?: () => RequestMeta | undefined;
  mutationStorage?: MutationJournalStorage;
}

export function createFlameClient(transport: Transport, opts?: FlameClientOptions): FlameClient {
  const mutationJournal = createMutationJournal({
    storage: opts?.mutationStorage,
    scope: () => opts?.capabilities?.()?.limits.idempotency,
  });
  const rpc = createRpcClient(transport, { requestMeta: opts?.requestMeta });
  let closePromise: Promise<void> | undefined;
  const close = (): Promise<void> => {
    if (!closePromise) {
      mutationJournal.dispose();
      closePromise = rpc.close();
    }
    return closePromise;
  };
  return Object.assign(createMethods(rpc, { ...opts, mutationJournal }), {
    close,
    queryJSON: rpc.callJSON,
  });
}
