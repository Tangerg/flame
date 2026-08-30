// In-memory Transport for unit tests. Backed by a push-pull async channel
// (`channel.ts`) for the inbound side; `inject()` is just a thin alias
// for `channel.push()`. Outbound side stores messages in an array that
// tests inspect via `outbox()`.

import { createPushPullChannel } from "../channel";
import type {
  Transport,
  TransportEvent,
  TransportRequest,
  TransportResponseMetadata,
} from "../transport";
import type { WireStreamingMethodName } from "@flame/runtime-contract/methods";
import { isResponse, type RpcId, type RpcMessage } from "../types";

export interface MemoryTransport extends Transport {
  inject(msg: RpcMessage, metadata?: TransportResponseMetadata, requestRpcId?: RpcId): void;
  endStream(method: WireStreamingMethodName, requestRpcId: RpcId): void;
  outbox(): TransportRequest[];
}

export function createMemoryTransport(): MemoryTransport {
  const sent: TransportRequest[] = [];
  // Test fixtures deliberately allow arbitrary pre-injection before a reader
  // exists; production transports must choose a finite/rendezvous capacity.
  const channel = createPushPullChannel<TransportEvent>({ capacity: "unbounded" });

  return {
    async send(msg) {
      if (channel.closed) throw new Error("transport closed");
      sent.push(msg);
    },
    recv: () => channel.iterator(),
    async close() {
      channel.close();
    },
    inject(msg, metadata, requestRpcId) {
      if (channel.closed) throw new Error("transport closed");
      const owner = requestRpcId ?? (isResponse(msg) ? msg.id : undefined);
      if (owner === undefined) {
        throw new Error("notification injection requires its owning request RPC id");
      }
      channel.push({ type: "message", message: msg, requestRpcId: owner, metadata });
    },
    endStream(method, requestRpcId) {
      if (channel.closed) throw new Error("transport closed");
      channel.push({ type: "streamEnd", method, requestRpcId });
    },
    outbox() {
      return [...sent];
    },
  };
}
