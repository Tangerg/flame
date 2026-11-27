import {
  WIRE_METHOD_POLICY,
  wireMethodRequiresIdempotency,
  type WireMethodName,
  type WireParams,
} from "@flame/runtime-client/methods";
import { validateMethodParams, validateWire } from "@flame/runtime-client/validation";
import type { PluginCallRoute, RequestMeta } from "@flame/runtime-client/wire";
import { RpcProtocolError } from "./errors";
import { cloneJSON, parseJSON } from "@frame/sdk/json";

export function captureRequestMeta(value: unknown): RequestMeta | null {
  const captured = cloneJSON(value);
  if (captured === undefined || captured === null) return null;
  const violations = validateWire("RequestMeta", captured, "request");
  if (violations.length > 0) throw new RpcProtocolError("request metadata", violations);
  return captured as RequestMeta;
}

export function checkRequest<M extends WireMethodName>(method: M, value: unknown): WireParams<M> {
  const violations = validateMethodParams(method, value);
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
  return pluginMethod(request.route).kind === "command";
}

// This projection reads routing metadata without re-encoding the feature's
// canonical source. Runtime alone admits the complete callable contract.
export const PLUGIN_COMPOSITIONS = ["collection", "selection", "decoration"] as const;
export type PluginComposition = (typeof PLUGIN_COMPOSITIONS)[number];

function isPluginComposition(value: unknown): value is PluginComposition {
  return (PLUGIN_COMPOSITIONS as readonly unknown[]).includes(value);
}

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
    !isPluginComposition(contract.composition) ||
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

export type PluginMethodReference = Pick<PluginCallRoute, "contract" | "method">;

export function pluginMethod(request: PluginMethodReference) {
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
    delivery: Object.hasOwn(method, "stream") ? "plugins.stream" : "plugins.call",
  } as const;
}

// Frame's installation target scope: the method addresses the installation that
// provides it, so callers bind that installation as the target reference.
export const INSTALLATION_SCOPE = "installation";

export type PluginDeliveryMethod = "plugins.call" | "plugins.stream";

export function isPluginDeliveryMethod(method: string): method is PluginDeliveryMethod {
  return method === "plugins.call" || method === "plugins.stream";
}
