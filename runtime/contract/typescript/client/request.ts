import {
  WIRE_METHOD_POLICY,
  wireMethodRequiresIdempotency,
  type WireMethodName,
  type WireParams,
} from "@flame/runtime-contract/methods";
import { validateMethodParams, validateWire } from "@flame/runtime-contract/validate";
import type { RequestMeta, CallPluginRequest } from "@flame/runtime-contract/wire";
import { RpcProtocolError } from "./errors";
import { parseJSON } from "./json";

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

// This projection reads routing metadata without re-encoding the feature's
// canonical source. Runtime alone admits the complete callable contract.
export function pluginContract(text: string) {
  const contract = parseJSON(text);
  if (
    contract === null ||
    typeof contract !== "object" ||
    !("methods" in contract) ||
    !Array.isArray(contract.methods) ||
    !("reference" in contract) ||
    typeof contract.reference !== "string" ||
    !("composition" in contract) ||
    typeof contract.composition !== "string" ||
    !("scope" in contract) ||
    typeof contract.scope !== "string"
  )
    throw new TypeError("Plugin call requires a callable contract");
  return {
    reference: contract.reference,
    composition: contract.composition,
    scope: contract.scope,
    methods: contract.methods as unknown[],
  };
}

export function pluginMethod(request: Pick<CallPluginRequest, "contract" | "method">) {
  const contract = pluginContract(request.contract);
  const members = contract.methods.filter(
    (member: unknown): member is Record<string, unknown> =>
      member !== null &&
      typeof member === "object" &&
      "name" in member &&
      member.name === request.method,
  );
  const method = members[0];
  if (
    members.length !== 1 ||
    !method ||
    (method.kind !== "query" && method.kind !== "command") ||
    typeof method.target !== "string"
  ) {
    throw new TypeError("Plugin call requires one admitted query or command method");
  }
  return {
    kind: method.kind,
    target: method.target,
    streaming: Object.hasOwn(method, "stream"),
  } as const;
}

export function pluginDeliveryMethod(
  request: Pick<CallPluginRequest, "contract" | "method">,
): "plugins.call" | "plugins.stream" {
  const method = pluginMethod(request);
  if (!method.streaming) return "plugins.call";
  return "plugins.stream";
}

export function pluginOperationKind(
  request: Pick<CallPluginRequest, "contract" | "method">,
): "query" | "command" {
  return pluginMethod(request).kind;
}
