import type { WireMethodName, WireParams } from "@flame/runtime-contract/methods";
import { validateMethodParams, validateWire } from "@flame/runtime-contract/validate";
import type { RequestMeta } from "@flame/runtime-contract/wire";
import { RpcProtocolError } from "./errors";

export function checkRequest<M extends WireMethodName>(
  method: M,
  value: unknown,
  requestMeta?: RequestMeta | null,
): WireParams<M> {
  const violations = validateMethodParams(method, value);
  if (requestMeta !== undefined && requestMeta !== null) {
    violations.push(...validateWire("RequestMeta", requestMeta, "request"));
  }
  if (violations.length > 0) throw new RpcProtocolError(`${method} params`, violations);
  return value as WireParams<M>;
}
