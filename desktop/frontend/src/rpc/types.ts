// JSON-RPC 2.0 envelope for the Flame Runtime Protocol (API.md §1). Notifications are used
// ONLY for runtime→client event delivery; every mutation is a correlated request.

import { z } from "zod";

export const JSONRPC_VERSION = "2.0" as const;

// JSON-RPC allows string | number; Flame locks to STRING (API.md §1.1) so dispatch and
// correlation never branch on id type. The client allocates monotonic integers and
// stringifies them before they hit the wire.
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

// Standard JSON-RPC codes and deliberately NOTHING else: a business failure is identified
// by `error.data.type`, never by number. The numeric space is the runtime's to assign, it
// has retired codes and left holes, and mirroring it here is a second copy of a table only
// one side edits.
export const RPC_METHOD_NOT_FOUND = -32601;

/** The canonical way to branch on an error (§8.2). Never compare codes. */
export function errorType(data: unknown): string | undefined {
  if (data && typeof data === "object" && "type" in data) {
    const t = (data as { type: unknown }).type;
    return typeof t === "string" ? t : undefined;
  }
  return undefined;
}

// Absence must stay OBSERVABLE so the layer that owns user-facing copy can supply it.
export function errorDetail(data: unknown): string | undefined {
  if (data && typeof data === "object") {
    const d = (data as { detail?: unknown }).detail;
    if (typeof d === "string" && d) return d;
  }
  return undefined;
}

/** Read a positive provider-requested retry delay without trusting input. */
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

// The inbound trust boundary (CLAUDE.md §3). Payloads stay `unknown` here and are checked
// against generated per-method schemas after correlation. The envelope is open to
// extension, but its three JSON-RPC shapes stay mutually exclusive.
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

/** `null` when the text is not valid JSON or not an accepted envelope — the CALLER decides
 *  whether that means "skip this frame" or "fail this call". Rejecting here is what keeps
 *  correlation and notification dispatch from ever seeing a non-envelope. */
export function parseRpcMessage(text: string): RpcMessage | null {
  let json: unknown;
  try {
    json = JSON.parse(text);
  } catch {
    return null;
  }
  return RpcEnvelopeSchema.safeParse(json).success ? (json as RpcMessage) : null;
}
