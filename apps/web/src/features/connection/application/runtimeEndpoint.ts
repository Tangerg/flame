import { PluginStorageError } from "@/extensions/storage";
import { z } from "zod";
import { endpointAcceptsBearer, normalizeRuntimeEndpoint } from "@flame/runtime-client";
export interface RuntimeEndpointTarget {
  endpoint: string;
  localToken?: string;
}
export interface RuntimeEndpointConfiguration {
  read(): RuntimeEndpointTarget;
  defaultTarget(): RuntimeEndpointTarget;
  replace(target: RuntimeEndpointTarget): boolean;
  reset(): boolean;
}
const UrlSchema = z.url();

const TokenSchema = z.string().regex(/^[\x21-\x7e]*$/);

export type RuntimeEndpointRejection =
  "invalid_url" | "unsupported_scheme" | "invalid_token" | "insecure_bearer" | "storage_failed";

export type RuntimeEndpointChange =
  | { kind: "applied"; endpoint: string; changed: boolean }
  | { kind: "rejected"; input: string; reason: RuntimeEndpointRejection };

export function createRuntimeEndpoint(configuration: RuntimeEndpointConfiguration) {
  function currentRuntimeEndpoint(): string {
    return configuration.read().endpoint;
  }

  function hasRuntimeAccessToken(): boolean {
    return Boolean(configuration.read().localToken);
  }

  function defaultRuntimeEndpoint(): string {
    return configuration.defaultTarget().endpoint;
  }

  function applyRuntimeEndpoint(input: string, localToken?: string): RuntimeEndpointChange {
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
    if (token && !endpointAcceptsBearer(endpoint))
      return { kind: "rejected", input, reason: "insecure_bearer" };
    let changed: boolean;
    try {
      changed = configuration.replace({ endpoint, localToken: token });
    } catch (error) {
      if (!(error instanceof PluginStorageError)) throw error;
      return { kind: "rejected", input, reason: "storage_failed" };
    }
    return { kind: "applied", endpoint, changed };
  }

  function resetRuntimeEndpoint(): RuntimeEndpointChange {
    let changed: boolean;
    try {
      changed = configuration.reset();
    } catch (error) {
      if (!(error instanceof PluginStorageError)) throw error;
      return {
        kind: "rejected",
        input: configuration.defaultTarget().endpoint,
        reason: "storage_failed",
      };
    }
    return { kind: "applied", endpoint: configuration.read().endpoint, changed };
  }

  return {
    currentRuntimeEndpoint,
    hasRuntimeAccessToken,
    defaultRuntimeEndpoint,
    applyRuntimeEndpoint,
    resetRuntimeEndpoint,
  };
}
export type RuntimeEndpoint = ReturnType<typeof createRuntimeEndpoint>;
