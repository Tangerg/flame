import { z } from "zod";
import { normalizeRuntimeEndpoint } from "@flame/runtime-contract/client/endpoint";
import {
  configuredRuntimeEndpoint,
  runtimeEndpointConfiguration,
  type RuntimeEndpointTarget,
} from "./ports/runtimeEndpoint";

const UrlSchema = z.url();
const TokenSchema = z.string().regex(/^[\x21-\x7e]*$/);

export type RuntimeEndpointRejection = "invalid_url" | "unsupported_scheme" | "invalid_token";

export type RuntimeEndpointChange =
  | { kind: "applied"; endpoint: string; changed: boolean }
  | { kind: "rejected"; input: string; reason: RuntimeEndpointRejection };

export function configuredRuntimeTarget(): RuntimeEndpointTarget | null {
  return configuredRuntimeEndpoint()?.read() ?? null;
}

export function currentRuntimeEndpoint(): string {
  return runtimeEndpointConfiguration().read().endpoint;
}

export function hasRuntimeAccessToken(): boolean {
  return Boolean(runtimeEndpointConfiguration().read().localToken);
}

export function defaultRuntimeEndpoint(): string {
  return runtimeEndpointConfiguration().defaultTarget().endpoint;
}

function replaceTarget(target: RuntimeEndpointTarget): RuntimeEndpointChange {
  const configuration = runtimeEndpointConfiguration();
  const current = configuration.read();
  const changed = current.endpoint !== target.endpoint || current.localToken !== target.localToken;
  if (changed) configuration.replace(target);
  return { kind: "applied", endpoint: target.endpoint, changed };
}

export function applyRuntimeEndpoint(input: string, localToken?: string): RuntimeEndpointChange {
  const configuration = runtimeEndpointConfiguration();
  const trimmed = input.trim() || configuration.defaultTarget().endpoint;
  if (localToken !== undefined && !TokenSchema.safeParse(localToken.trim()).success) {
    return { kind: "rejected", input, reason: "invalid_token" };
  }
  const parsed = UrlSchema.safeParse(trimmed);
  if (!parsed.success) return { kind: "rejected", input, reason: "invalid_url" };
  const protocol = new URL(parsed.data).protocol;
  if (protocol !== "http:" && protocol !== "https:") {
    return { kind: "rejected", input, reason: "unsupported_scheme" };
  }
  const endpoint = normalizeRuntimeEndpoint(trimmed);
  if (!endpoint) return { kind: "rejected", input, reason: "invalid_url" };
  const current = configuration.read();
  const token =
    localToken === undefined
      ? current.endpoint === endpoint
        ? current.localToken
        : undefined
      : localToken.trim() || undefined;
  return replaceTarget({ endpoint, localToken: token });
}

export function resetRuntimeEndpoint(): RuntimeEndpointChange {
  return replaceTarget(runtimeEndpointConfiguration().defaultTarget());
}
