import { type KeyValueStore } from "@/extensions/storage";
import { type LifetimeContext } from "dougong";
import {
  type RuntimeEndpointConfiguration,
  type RuntimeEndpointTarget,
  type TrustedRuntime,
} from "@/features/connection/application/runtimeEndpoint";
import { normalizeRuntimeEndpoint } from "@flame/runtime-client";

const STORAGE_KEY = "endpoint";
const TRUST_KEY = "trusted-runtime";

function readProfile(value: unknown): TrustedRuntime | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { endpoint, name, store } = value as Partial<TrustedRuntime>;
  if (
    typeof endpoint !== "string" ||
    normalizeRuntimeEndpoint(endpoint) !== endpoint ||
    typeof name !== "string" ||
    !name ||
    typeof store !== "string" ||
    !/^[!-~]+$/.test(store)
  )
    return undefined;
  return { endpoint, name, store };
}

interface EndpointBindings {
  lifetime: Pick<LifetimeContext, "defer">;
  storage: KeyValueStore;
}

type ReplaceRuntimeEndpoint = (commit: () => void) => void;

export function bindRuntimeEndpointConfiguration(
  ctx: EndpointBindings,
  replaceConnection: ReplaceRuntimeEndpoint,
  bootstrap: () => RuntimeEndpointTarget,
) {
  const defaultTarget = () => {
    const target = bootstrap();
    const endpoint = normalizeRuntimeEndpoint(target.endpoint);
    if (!endpoint) throw new Error("invalid bootstrap Runtime endpoint");
    return { ...target, endpoint };
  };
  const stored = ctx.storage.get(STORAGE_KEY);
  const restored = typeof stored === "string" ? normalizeRuntimeEndpoint(stored) : null;
  let profile = readProfile(ctx.storage.get(TRUST_KEY));
  type Target =
    | { kind: "host" | "retired"; endpoint: string }
    | ({ kind: "configured" } & RuntimeEndpointTarget);
  let target: Target =
    profile && restored === profile.endpoint
      ? { kind: "configured", endpoint: profile.endpoint, expectedRuntimeStore: profile.store }
      : { kind: "host", endpoint: restored ?? defaultTarget().endpoint };
  ctx.lifetime.defer(() => {
    target = { kind: "retired", endpoint: target.endpoint };
  });

  function select(next: Target) {
    if (target.kind === "retired") throw new Error("Runtime endpoint owner is retired");
    ctx.storage.set(STORAGE_KEY, next.endpoint);
    replaceConnection(() => {
      target = next;
    });
  }
  const configuration: RuntimeEndpointConfiguration & {
    usesHostCredential(): boolean;
    profile(): TrustedRuntime | undefined;
    register(profile: TrustedRuntime, target: RuntimeEndpointTarget): boolean;
    forget(): void;
  } = {
    profile: () => profile,
    register(next, target) {
      ctx.storage.set(TRUST_KEY, next);
      profile = next;
      return configuration.replace(target);
    },
    forget() {
      ctx.storage.remove(TRUST_KEY);
      profile = undefined;
    },
    usesHostCredential: () =>
      target.kind === "host" && target.endpoint === defaultTarget().endpoint,
    read() {
      const { endpoint } = target;
      if (target.kind === "configured") {
        return {
          endpoint,
          localToken: target.localToken,
          expectedRuntimeStore: target.expectedRuntimeStore,
        };
      }
      const host = target.kind === "host" ? defaultTarget() : undefined;
      return { endpoint, localToken: host?.endpoint === endpoint ? host.localToken : undefined };
    },
    defaultTarget,
    replace(next) {
      const current = configuration.read();
      if (
        current.endpoint === next.endpoint &&
        current.localToken === next.localToken &&
        current.expectedRuntimeStore === next.expectedRuntimeStore
      )
        return false;
      select({ kind: "configured", ...next });
      return true;
    },
    reset() {
      const { endpoint } = defaultTarget();
      if (target.kind === "host" && target.endpoint === endpoint) return false;
      select({ kind: "host", endpoint });
      return true;
    },
  };
  return configuration;
}
