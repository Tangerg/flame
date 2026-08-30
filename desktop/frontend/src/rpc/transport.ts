// TRANSPORT.md §2. `send()` is fire-and-forget — it returns once the message is handed off,
// not once the peer processed it — and `recv()` yields inbound messages plus transport
// lifecycle events until close. Correlating responses by `id` is the RpcClient's job.

import type { RpcId, RpcMessage, RpcRequest } from "./types";
import type { WireMethodName, WireStreamingMethodName } from "@flame/runtime-contract/methods";

/** The one non-run streaming method, shared by transport and stream lifecycle. */
export const RUNTIME_SUBSCRIBE_METHOD = "runtime.subscribe" satisfies WireMethodName;

export type TransportRequest = Omit<RpcRequest, "method"> & { method: WireMethodName };

export interface TransportResponseMetadata {
  requestId?: string;
}

export type TransportEvent =
  | {
      type: "message";
      message: RpcMessage;
      requestRpcId: RpcId;
      metadata?: TransportResponseMetadata;
    }
  | { type: "requestError"; rpcId: RpcId; error: Error }
  | {
      type: "streamEnd";
      method: WireStreamingMethodName;
      requestRpcId: RpcId;
      error?: Error;
      metadata?: TransportResponseMetadata;
    };

export interface Transport {
  /** Queue an outbound request. Client notifications are not in this protocol. */
  send(msg: TransportRequest, signal?: AbortSignal, options?: TransportSendOptions): Promise<void>;
  /**
   * Stream of inbound messages and lifecycle events. Yields until the transport disconnects,
   * after which the iterator returns. Multiple readers are not supported
   * — RpcClient is the sole consumer.
   */
  recv(): AsyncIterable<TransportEvent>;
  close(): Promise<void>;
}

export interface TransportSendOptions {
  idempotencyKey?: string;
  /** Opaque durable replay-store identity, carried as
   * `Idempotency-Namespace` beside the key. */
  idempotencyNamespace?: string;
  /** Stream resume cursor, carried as `Last-Event-Id` (TRANSPORT §9.2). Transport
   *  metadata, never request params. */
  lastEventId?: string;
}
