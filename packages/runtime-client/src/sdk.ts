import { createRpcClient } from "./client";
import { createMethods, type Methods } from "./methods";
import type { Transport } from "./transport";
import { createMutationJournal, type MutationJournalStorage } from "./mutationJournal";
import type { RequestMeta, ServerCapabilities } from "@flame/runtime-client/wire";
import { requireCapabilities } from "./preflight";
import { createWireCallPath, type WireCallPath } from "./wireCallPath";

export interface FlameClient extends Methods {
  queryJSON: WireCallPath["queryJSON"];
  close(): Promise<void>;
}

export interface FlameClientOptions {
  capabilities?: () => ServerCapabilities | null | undefined;
  requestMeta?: () => RequestMeta | undefined;
  mutationStorage?: MutationJournalStorage;
}

export function createFlameClient(
  transport: Transport,
  { capabilities, requestMeta, mutationStorage }: FlameClientOptions = {},
): FlameClient {
  const mutationJournal = createMutationJournal({
    storage: mutationStorage,
    scope: () => capabilities?.()?.limits.idempotency,
    admitNewCommand: (method, params, metadata) => {
      requireCapabilities(method, params, capabilities?.(), metadata?.clientCapabilities);
    },
  });
  const rpc = createRpcClient(transport);
  const wire = createWireCallPath(rpc, { capabilities, requestMeta, mutationJournal });
  let closePromise: Promise<void> | undefined;
  const close = (): Promise<void> => {
    if (!closePromise) {
      mutationJournal.dispose();
      closePromise = rpc.close();
    }
    return closePromise;
  };
  return Object.assign(createMethods(wire, { capabilities, mutationJournal }), {
    close,
    queryJSON: wire.queryJSON,
  });
}
