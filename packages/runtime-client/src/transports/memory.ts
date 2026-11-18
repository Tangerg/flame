import { createPushPullChannel } from "../channel";
import type {
  Transport,
  TransportEvent,
  TransportRequest,
  TransportResponseMetadata,
} from "../transport";
import type { WireMethodName, WireStreamingMethodName } from "@flame/runtime-client/methods";
import { isResponse, type RpcId, type RpcMessage } from "../types";

export interface MemoryTransport extends Transport {
  inject(msg: RpcMessage, metadata?: TransportResponseMetadata, requestRpcId?: RpcId): void;
  endStream(method: WireStreamingMethodName, requestRpcId: RpcId): void;
  outbox(): TransportRequest[];
  waitForRequest<M extends WireMethodName>(
    method: M,
    matches?: (message: TransportRequest) => boolean,
  ): Promise<TransportRequest & { method: M }>;
}

export function createMemoryTransport(): MemoryTransport {
  const sent: TransportRequest[] = [];
  const channel = createPushPullChannel<TransportEvent>({
    capacity: "unbounded",
  });
  let outbound = Promise.withResolvers<void>();

  return {
    async send(msg) {
      if (channel.closed) throw new Error("transport closed");
      sent.push(msg);
      outbound.resolve();
      outbound = Promise.withResolvers<void>();
    },
    recv: () => channel.iterator(),
    async close() {
      channel.close();
      outbound.resolve();
    },
    inject(msg, metadata, requestRpcId) {
      if (channel.closed) throw new Error("transport closed");
      const owner = requestRpcId ?? (isResponse(msg) ? msg.id : undefined);
      if (owner === undefined) {
        throw new Error("notification injection requires its owning request RPC id");
      }
      channel.push({
        type: "message",
        message: msg,
        requestRpcId: owner,
        metadata,
      });
    },
    endStream(method, requestRpcId) {
      if (channel.closed) throw new Error("transport closed");
      channel.push({ type: "streamEnd", method, requestRpcId });
    },
    outbox() {
      return [...sent];
    },
    async waitForRequest<M extends WireMethodName>(
      method: M,
      matches?: (message: TransportRequest) => boolean,
    ) {
      while (true) {
        const request = sent.find(
          (message): message is TransportRequest & { method: M } =>
            message.method === method && (matches?.(message) ?? true),
        );
        if (request) return request;
        if (channel.closed) throw new Error(`transport closed before sending ${method}`);
        await outbound.promise;
      }
    },
  };
}
