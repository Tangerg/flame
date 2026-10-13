import type { OpenPluginResourcesRequest, PluginResourceLease } from "@flame/runtime-contract/wire";
import type { RpcClient } from "./client";
import type { RpcId } from "./types";
import type { WireCall } from "./wireCallPath";
import { RpcConnectionError } from "./errors";

export interface ResourceSubscription<T> {
  readonly result: T;
  readonly closed: Promise<void>;
  close(): Promise<void>;
}

export async function openResourceLease(
  client: Pick<RpcClient, "closedSignal" | "onStreamEnd">,
  call: WireCall,
  request: OpenPluginResourcesRequest,
  signal?: AbortSignal,
): Promise<ResourceSubscription<PluginResourceLease>> {
  const lifetime = new AbortController();
  const owned = signal ? AbortSignal.any([lifetime.signal, signal]) : lifetime.signal;
  const closed = Promise.withResolvers<void>();
  let id: RpcId | undefined;
  let admitted = false;
  let ended: Error | undefined;
  const connectionClosed = () => {
    unsubscribe();
    if (owned.aborted || !admitted) closed.resolve();
    else closed.reject(client.closedSignal.reason);
  };
  const unsubscribe = client.onStreamEnd((event) => {
    if (event.method !== "plugins.openResources" || event.requestRpcId !== id) return;
    unsubscribe();
    client.closedSignal.removeEventListener("abort", connectionClosed);
    if (!owned.aborted)
      ended =
        event.error ?? new RpcConnectionError("plugin resource subscription ended unexpectedly");
    if (owned.aborted || !admitted) closed.resolve();
    else closed.reject(ended);
  });
  client.closedSignal.addEventListener("abort", connectionClosed, { once: true });
  try {
    const result = await call("plugins.openResources", request, {
      signal: owned,
      onRequestRpcId: (value) => {
        id = value;
      },
    });
    client.closedSignal.throwIfAborted();
    if (ended) throw ended;
    admitted = true;
    return {
      result,
      closed: closed.promise,
      async close() {
        lifetime.abort();
        await closed.promise;
      },
    };
  } catch (error) {
    lifetime.abort();
    unsubscribe();
    client.closedSignal.removeEventListener("abort", connectionClosed);
    throw error;
  }
}
