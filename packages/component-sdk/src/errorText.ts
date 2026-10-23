import { RpcError } from "@flame/runtime-client";

export function failureText(
  error: unknown,
  fallback = "The operation could not be completed.",
): string {
  if (error instanceof RpcError) return error.data.detail || error.message;
  return error instanceof Error ? error.message : fallback;
}
