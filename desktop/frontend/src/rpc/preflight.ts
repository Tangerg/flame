// The rules are READ from the generated table, never restated — API.md §9 forbids a second
// switch. Refuses only what the server has already said it cannot do: with no negotiated
// snapshot every call goes out and the runtime stays authoritative, because a client
// guessing "probably unsupported" takes away a feature the server offers.

import type { ClientCapabilities, ServerCapabilities } from "@flame/runtime-contract/wire";
import {
  WIRE_CAPABILITY_POLICY,
  type WireCapabilityCondition,
  type WireFeature,
  type WireMethodName,
} from "@flame/runtime-contract/methods";

/**
 * The features this call needs that the server did not advertise.
 *
 * Empty when the call is allowed, when the method is ungated, or when nothing has
 * been negotiated yet.
 */
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
      // §9: a key the server did not advertise reads as off, which is the same
      // reading the dispatcher's gate applies to its own advertised map. A
      // clientOptIn feature also requires an explicit declaration on THIS
      // request; server support alone never opts a caller into semantics it did
      // not ask for.
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
  if (condition.operator === "equals") return value === condition.value;
  return value !== undefined && !isEmpty(value);
}

function lookup(params: unknown, path: string): unknown {
  let value = params;
  for (const segment of path.split(".")) {
    if (typeof value !== "object" || value === null || Array.isArray(value)) return undefined;
    value = (value as Record<string, unknown>)[segment];
  }
  return value;
}

/**
 * An explicitly empty value counts as absent, so `{ watches: [] }` asks for the
 * same thing as omitting the field and is gated the same way.
 */
function isEmpty(value: unknown): boolean {
  if (value === null || value === "" || value === false) return true;
  if (Array.isArray(value)) return value.length === 0;
  if (typeof value === "object") return Object.keys(value).length === 0;
  return false;
}
