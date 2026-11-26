import type { JSONRPC_VERSION, RpcId, RpcMessage } from "./rpcMessage";
import type { WireMethodName } from "@flame/runtime-client/methods";

export const RUNTIME_SUBSCRIBE_METHOD = "runtime.subscribe" satisfies WireMethodName;

export interface TransportRequest {
  jsonrpc: typeof JSONRPC_VERSION;
  id: RpcId;
  method: WireMethodName;
  params?: unknown;
}

export interface TransportResponseMetadata {
  requestId?: string;
}

export interface TransportMessageMetadata extends TransportResponseMetadata {
  readonly eventStream?: { readonly cursor: string | undefined };
}

export type TransportEvent =
  | {
      type: "message";
      message: RpcMessage;
      requestRpcId: RpcId;
      metadata?: TransportMessageMetadata;
      consumed?: (delivery: Promise<void>) => void;
    }
  | {
      type: "streamEnd";
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
