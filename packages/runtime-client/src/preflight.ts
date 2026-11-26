import type { ClientCapabilities, ServerCapabilities } from "@flame/runtime-client/wire";
import {
  WIRE_CAPABILITY_POLICY,
  type WireCapabilityCondition,
  type WireFeature,
  type WireMethodName,
} from "@flame/runtime-client/methods";

export function unnegotiated(
  method: WireMethodName,
  params: unknown,
  capabilities: ServerCapabilities | null | undefined,
  clientCapabilities?: ClientCapabilities,
): WireFeature[] {
  const rules = WIRE_CAPABILITY_POLICY[method];
  if (!rules || !capabilities) return [];

  const missing: WireFeature[] = [];
  for (const rule of rules) {
    if (rule.when && !rule.when.every((condition) => matches(condition, params))) continue;
    for (const feature of rule.requires) {
      const advertised = capabilities.features[feature];
      const supported = advertised?.enabled === true;
      const optedIn =
        advertised?.clientOptIn !== true ||
        clientCapabilities?.features?.[feature]?.enabled === true;
      if ((!supported || !optedIn) && !missing.includes(feature)) {
        missing.push(feature);
      }
    }
  }
  return missing;
}

function matches(condition: WireCapabilityCondition, params: unknown): boolean {
  const value = lookup(params, condition.field);
  switch (condition.operator) {
    case "present":
      return value !== undefined;
    case "equals":
      return value === condition.value;
    case "true":
      return value === true;
  }
}

function lookup(params: unknown, path: string): unknown {
  let value = params;
  for (const segment of path.split(".")) {
    if (typeof value !== "object" || value === null || !Object.hasOwn(value, segment))
      return undefined;
    value = (value as Record<string, unknown>)[segment];
  }
  return value;
}
