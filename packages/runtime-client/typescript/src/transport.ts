import type { RpcId, RpcMessage, RpcRequest } from "./types";
import type { WireMethodName, WireStreamingMethodName } from "@flame/runtime-client/methods";

export const RUNTIME_SUBSCRIBE_METHOD = "runtime.subscribe" satisfies WireMethodName;

export type TransportRequest = Omit<RpcRequest, "method"> & {
  method: WireMethodName;
};

export interface TransportResponseMetadata {
  requestId?: string;
}

export type TransportEvent =
  | {
      type: "message";
      message: RpcMessage;
      requestRpcId: RpcId;
      metadata?: TransportResponseMetadata;
      consumed?: (delivery: Promise<void>) => void;
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
  send(msg: TransportRequest, signal?: AbortSignal, options?: TransportSendOptions): Promise<void>;
  recv(): AsyncIterable<TransportEvent>;
  close(): Promise<void>;
}

export interface TransportSendOptions {
  idempotencyKey?: string;
  idempotencyNamespace?: string;
  lastEventId?: string;
}
