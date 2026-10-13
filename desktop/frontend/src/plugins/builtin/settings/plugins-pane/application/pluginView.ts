import type {
  ReadPluginViewRequest,
  ReadPluginRendererRequest,
} from "@flame/runtime-contract/wire";

export type PluginViewStage =
  | "carrier"
  | "resource"
  | "initial"
  | "handshake"
  | "read"
  | "publication"
  | "protocol"
  | "guest"
  | "retirement";
export type PluginViewStatus =
  | { type: "loading" }
  | { type: "ready" }
  | { type: "failure"; stage: PluginViewStage; reason: string };

export interface PluginViewReads<T> {
  readonly binding: Readonly<ReadPluginViewRequest | ReadPluginRendererRequest>;
  resource(signal: AbortSignal): Promise<string>;
  initial(signal: AbortSignal): Promise<T>;
  read(cursor: string | undefined, signal: AbortSignal): Promise<T>;
}
