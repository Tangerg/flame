import { PluginStorageError } from "@/extensions/storage";
import { z } from "zod";
import { endpointAcceptsBearer, normalizeRuntimeEndpoint } from "@flame/runtime-client";

export interface RuntimeEndpointTarget {
  endpoint: string;
  localToken?: string;
  expectedRuntimeStore?: string;
}

export interface TrustedRuntime {
  endpoint: string;
  name: string;
  store: string;
}

export interface RuntimeEndpointConfiguration {
  read(): RuntimeEndpointTarget;
  defaultTarget(): RuntimeEndpointTarget;
  replace(target: RuntimeEndpointTarget): boolean;
  reset(): boolean;
}

interface TrustedRuntimeConfiguration {
  read(): TrustedRuntime | undefined;
  save(profile: TrustedRuntime, target: RuntimeEndpointTarget): boolean;
  forget(): void;
  probe(target: RuntimeEndpointTarget, signal: AbortSignal): Promise<string>;
}

const UrlSchema = z.url();
const TokenSchema = z.string().regex(/^[\x21-\x7e]*$/);

export type RuntimeEndpointRejection =
  | "invalid_url"
  | "unsupported_scheme"
  | "invalid_token"
  | "insecure_bearer"
  | "storage_failed"
  | "trust_failed";

type Rejected = { kind: "rejected"; input: string; reason: RuntimeEndpointRejection };
export type RuntimeEndpointChange =
  { kind: "applied"; endpoint: string; changed: boolean } | Rejected;

export function createRuntimeEndpoint(
  configuration: RuntimeEndpointConfiguration,
  trusted?: TrustedRuntimeConfiguration,
) {
  function validatedTarget(input: string, localToken?: string): RuntimeEndpointTarget | Rejected {
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
    if (token && !endpointAcceptsBearer(endpoint)) {
      return { kind: "rejected", input, reason: "insecure_bearer" };
    }
    const saved = trusted?.read();
    return {
      endpoint,
      localToken: token,
      expectedRuntimeStore: saved?.endpoint === endpoint ? saved.store : undefined,
    };
  }

  function applyRuntimeEndpoint(input: string, localToken?: string): RuntimeEndpointChange {
    const target = validatedTarget(input, localToken);
    if ("kind" in target) return target;
    try {
      return { kind: "applied", endpoint: target.endpoint, changed: configuration.replace(target) };
    } catch (error) {
      if (!(error instanceof PluginStorageError)) throw error;
      return { kind: "rejected", input, reason: "storage_failed" };
    }
  }

  async function trustRuntime(
    input: string,
    name: string,
    id: string,
    token?: string,
  ): Promise<RuntimeEndpointChange> {
    const target = validatedTarget(input, token);
    if ("kind" in target) return target;
    name = name.trim();
    id = id.trim();
    if (!trusted || !name || name.length > 80 || !target.localToken) {
      return { kind: "rejected", input, reason: "trust_failed" };
    }
    const expected = target.expectedRuntimeStore || id;
    const authority = configuration.read();
    let store: string;
    try {
      store = await trusted.probe(
        { ...target, expectedRuntimeStore: expected || undefined },
        AbortSignal.timeout(10000),
      );
    } catch {
      return { kind: "rejected", input, reason: "trust_failed" };
    }
    const current = configuration.read();
    if (
      !store ||
      (expected && expected !== store) ||
      (id && id !== store) ||
      authority.endpoint !== current.endpoint ||
      authority.localToken !== current.localToken ||
      authority.expectedRuntimeStore !== current.expectedRuntimeStore
    ) {
      return { kind: "rejected", input, reason: "trust_failed" };
    }
    try {
      const changed = trusted.save(
        { endpoint: target.endpoint, name, store },
        { ...target, expectedRuntimeStore: store },
      );
      return { kind: "applied", endpoint: target.endpoint, changed };
    } catch (error) {
      if (!(error instanceof PluginStorageError)) throw error;
      return { kind: "rejected", input, reason: "storage_failed" };
    }
  }

  function resetRuntimeEndpoint(): RuntimeEndpointChange {
    try {
      return {
        kind: "applied",
        changed: configuration.reset(),
        endpoint: configuration.read().endpoint,
      };
    } catch (error) {
      if (!(error instanceof PluginStorageError)) throw error;
      return {
        kind: "rejected",
        input: configuration.defaultTarget().endpoint,
        reason: "storage_failed",
      };
    }
  }

  function forgetTrustedRuntime(): RuntimeEndpointChange {
    if (!trusted) return resetRuntimeEndpoint();
    try {
      if (configuration.read().expectedRuntimeStore) configuration.reset();
      trusted.forget();
      return { kind: "applied", changed: true, endpoint: configuration.read().endpoint };
    } catch (error) {
      if (!(error instanceof PluginStorageError)) throw error;
      return {
        kind: "rejected",
        input: configuration.defaultTarget().endpoint,
        reason: "storage_failed",
      };
    }
  }

  return {
    currentRuntimeEndpoint: () => configuration.read().endpoint,
    hasRuntimeAccessToken: () => Boolean(configuration.read().localToken),
    defaultRuntimeEndpoint: () => configuration.defaultTarget().endpoint,
    trustedRuntime: () => trusted?.read(),
    trustRuntime,
    forgetTrustedRuntime,
    applyRuntimeEndpoint,
    resetRuntimeEndpoint,
  };
}
export type RuntimeEndpoint = ReturnType<typeof createRuntimeEndpoint>;
