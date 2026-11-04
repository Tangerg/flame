import {
  Backend,
  decodeReceipt,
  declareMethod,
  parseJSON,
  type CallContext,
  type ReceiptRequest,
  type MethodDescriptor,
} from "@flame/backend-sdk/mcp";
import { validCommand } from "@flame/backend-sdk/mcp/contract";
import wire from "./protocol.generated.json";

export interface ToolReceipt {
  result?: {
    output: { content?: readonly unknown[]; details?: unknown };
    isError?: boolean;
  };
}

export function toolCommandId(context: CallContext): string {
  if (!context.invocation)
    throw new Error("plugin: tool invocation is required");
  const identity = wire.constants.toolCommandPrefix! + context.invocation.id;
  if (!validCommand(identity))
    throw new Error("plugin: tool command identity exceeds its bound");
  return identity;
}

export function toolReceipts(
  backend: Backend,
  handler: (
    context: CallContext,
    request: ReceiptRequest,
  ) => ToolReceipt | Promise<ToolReceipt>,
): void {
  const method = declareMethod(parseJSON(wire.toolReceipt) as MethodDescriptor);
  backend.implement(method, decodeReceipt, handler);
}
