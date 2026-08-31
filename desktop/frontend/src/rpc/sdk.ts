// The protocol is transport-agnostic (TRANSPORT.md), so the SDK takes a `Transport` and
// NOTHING else. Sidecar metadata is an HTTP-only concern and lives in sidecar.ts.

import { createRpcClient } from "./client";
import { createMethods, type Methods, type MethodsOptions } from "./methods";
import type { Transport } from "./transport";

export interface FlameClient extends Methods {
  close(): Promise<void>;
}

export function createFlameClient(transport: Transport, opts?: MethodsOptions): FlameClient {
  const rpc = createRpcClient(transport, { requestMeta: opts?.requestMeta });
  let closePromise: Promise<void> | undefined;
  const close = (): Promise<void> => {
    closePromise ??= (async () => {
      let journalFailure: unknown;
      try {
        opts?.mutationJournal?.dispose();
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
  return Object.assign(createMethods(rpc, opts), { close });
}
