import { z } from "zod";

export const JSONRPC_VERSION = "2.0" as const;

export type RpcId = string;

export interface RpcRequest<P = unknown> {
  jsonrpc: typeof JSONRPC_VERSION;
  id: RpcId;
  method: string;
  params?: P;
}

interface RpcResponseSuccess<R = unknown> {
  jsonrpc: typeof JSONRPC_VERSION;
  id: RpcId;
  result: R;
}

export interface RpcResponseError {
  jsonrpc: typeof JSONRPC_VERSION;
  id: RpcId;
  error: RpcErrorPayload;
}

export type RpcResponse<R = unknown> = RpcResponseSuccess<R> | RpcResponseError;

export interface RpcNotification<P = unknown> {
  jsonrpc: typeof JSONRPC_VERSION;
  method: string;
  params?: P;
}

export type RpcMessage = RpcRequest | RpcResponse | RpcNotification;

interface RpcErrorPayload {
  code: number;
  message: string;
  data?: unknown;
}

export const RPC_METHOD_NOT_FOUND = -32601;

export function errorType(data: unknown): string | undefined {
  if (data && typeof data === "object" && "type" in data) {
    const t = (data as { type: unknown }).type;
    return typeof t === "string" ? t : undefined;
  }
  return undefined;
}

export function errorDetail(data: unknown): string | undefined {
  if (data && typeof data === "object") {
    const d = (data as { detail?: unknown }).detail;
    if (typeof d === "string" && d) return d;
  }
  return undefined;
}

export function errorActiveRun(data: unknown): { runId: string; status: string } | undefined {
  if (!data || typeof data !== "object") return undefined;
  const ref = (data as { activeRun?: unknown }).activeRun;
  if (!ref || typeof ref !== "object") return undefined;
  const { runId, status } = ref as { runId?: unknown; status?: unknown };
  if (typeof runId !== "string" || !runId) return undefined;
  if (typeof status !== "string" || !status) return undefined;
  return { runId, status };
}

export function errorRetryAfterSeconds(data: unknown): number | undefined {
  if (data && typeof data === "object") {
    const retryAfterSeconds = (data as { retryAfterSeconds?: unknown }).retryAfterSeconds;
    if (
      typeof retryAfterSeconds === "number" &&
      Number.isInteger(retryAfterSeconds) &&
      retryAfterSeconds > 0
    ) {
      return retryAfterSeconds;
    }
  }
  return undefined;
}

export function isResponse(msg: RpcMessage): msg is RpcResponse {
  return "id" in msg && msg.id !== undefined && !("method" in msg);
}

export function isNotification(msg: RpcMessage): msg is RpcNotification {
  return !("id" in msg) || msg.id === undefined;
}

export function isErrorResponse(msg: RpcResponse): msg is RpcResponseError {
  return "error" in msg;
}

const RpcEnvelopeSchema = z
  .looseObject({
    jsonrpc: z.literal(JSONRPC_VERSION),
    id: z.string().optional(),
    method: z.string().optional(),
    params: z.unknown().optional(),
    result: z.unknown().optional(),
    error: z
      .looseObject({ code: z.number().int(), message: z.string(), data: z.unknown().optional() })
      .optional(),
  })
  .superRefine((value, context) => {
    const hasId = value.id !== undefined;
    const hasMethod = value.method !== undefined;
    const hasParams = Object.hasOwn(value, "params");
    const hasResult = Object.hasOwn(value, "result");
    const hasError = value.error !== undefined;

    const validRequest = hasId && hasMethod && !hasResult && !hasError;
    const validResponse = hasId && !hasMethod && !hasParams && hasResult !== hasError;
    const validNotification = !hasId && hasMethod && !hasResult && !hasError;
    if (!validRequest && !validResponse && !validNotification) {
      context.addIssue({
        code: "custom",
        message: "expected exactly one JSON-RPC request, response, or notification shape",
      });
    }
  });

export function parseRpcMessage(text: string): RpcMessage | null {
  let json: unknown;
  try {
    json = JSON.parse(text);
  } catch {
    return null;
  }
  return RpcEnvelopeSchema.safeParse(json).success ? (json as RpcMessage) : null;
}
