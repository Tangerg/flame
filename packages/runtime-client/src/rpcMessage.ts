import { parseJSON, type JSONValue } from "@frame/sdk/json";
import { z } from "zod";

export const JSONRPC_VERSION = "2.0";

const version = z.literal(JSONRPC_VERSION);
const responseFields = { jsonrpc: version, id: z.string() };

const RpcResultSchema = z.strictObject({ ...responseFields, result: z.unknown() });
const RpcErrorSchema = z.object({
  code: z.number().int(),
  message: z.string(),
  data: z.unknown().optional(),
});
const RpcFailureSchema = z.strictObject({ ...responseFields, error: RpcErrorSchema });
const RpcNotificationSchema = z.strictObject({
  jsonrpc: version,
  method: z.string().min(1),
  params: z.unknown().optional(),
});
const RpcMessageSchema = z.union([RpcResultSchema, RpcFailureSchema, RpcNotificationSchema]);

export type RpcId = z.output<typeof responseFields.id>;
export type RpcErrorPayload = z.output<typeof RpcErrorSchema>;
export type RpcResponse = z.output<typeof RpcResultSchema> | z.output<typeof RpcFailureSchema>;
export type RpcNotification = z.output<typeof RpcNotificationSchema>;
export type RpcMessage = z.output<typeof RpcMessageSchema>;

export function isResponse(message: RpcMessage): message is RpcResponse {
  return "id" in message;
}

export function parseRpcMessage(text: string): RpcMessage | undefined {
  let document: JSONValue;
  try {
    document = parseJSON(text);
  } catch {
    return undefined;
  }
  const admitted = RpcMessageSchema.safeParse(document);
  return admitted.success ? admitted.data : undefined;
}
