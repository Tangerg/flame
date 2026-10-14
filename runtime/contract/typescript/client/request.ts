import {
  WIRE_METHOD_POLICY,
  wireMethodRequiresIdempotency,
  type WireMethodName,
  type WireParams,
} from "@flame/runtime-contract/methods";
import { validateMethodParams, validateWire } from "@flame/runtime-contract/validate";
import type { Descriptor, RequestMeta, CallPluginRequest } from "@flame/runtime-contract/wire";
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

// The consumer contract determines the envelope's semantics. Runtime still
// requires exact equality with the installed contribution before dispatch.
export function requestRequiresIdempotency<M extends WireMethodName>(
  method: M,
  params: WireParams<M>,
): boolean {
  if (WIRE_METHOD_POLICY[method].operation !== "extension") {
    return wireMethodRequiresIdempotency(method);
  }
  const request = checkRequest("plugins.call", params);
  return pluginMethod(request).kind === "command";
}

function pluginMethod(request: Pick<CallPluginRequest, "contract" | "method">) {
  const contract: unknown = JSON.parse(request.contract);
  const violations = validateWire("Descriptor", contract);
  if (violations.length) throw new RpcProtocolError("plugin contract", violations);
  const members = (contract as Descriptor).methods.filter(
    (member) => member.name === request.method,
  );
  if (members.length !== 1 || !["query", "command"].includes(members[0]!.kind)) {
    throw new TypeError("Plugin call requires one admitted query or command method");
  }
  return members[0]!;
}

export function pluginDeliveryMethod(
  request: Pick<CallPluginRequest, "contract" | "method">,
): "plugins.call" | "plugins.stream" {
  const method = pluginMethod(request);
  if (method.stream === undefined) return "plugins.call";
  if (method.kind !== "command")
    throw new TypeError("Installed stream requires a command contract");
  return "plugins.stream";
}
