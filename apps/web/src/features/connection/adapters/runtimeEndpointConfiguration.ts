import { type KeyValueStore } from "@/extensions/storage";
import { type LifetimeOperations } from "dougong";
import {
  type RuntimeEndpointConfiguration,
  type RuntimeEndpointTarget,
} from "@/features/connection/application/runtimeEndpoint";
import { normalizeRuntimeEndpoint } from "@flame/runtime-client";

const STORAGE_KEY = "endpoint";

interface EndpointBindings extends Pick<LifetimeOperations, "cleanup"> {
  storage: KeyValueStore;
}

export type ReplaceRuntimeEndpoint = (commit: () => void) => void;

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
  type Target =
    | { kind: "host" | "retired"; endpoint: string }
    | ({ kind: "configured" } & RuntimeEndpointTarget);
  let target: Target = { kind: "host", endpoint: restored ?? defaultTarget().endpoint };
  ctx.cleanup(() => {
    target = { kind: "retired", endpoint: target.endpoint };
  });

  function select(next: Target) {
    if (target.kind === "retired") throw new Error("Runtime endpoint owner is retired");
    ctx.storage.set(STORAGE_KEY, next.endpoint);
    replaceConnection(() => {
      target = next;
    });
  }
  const configuration: RuntimeEndpointConfiguration & { usesHostCredential(): boolean } = {
    usesHostCredential: () =>
      target.kind === "host" && target.endpoint === defaultTarget().endpoint,
    read() {
      const { endpoint } = target;
      if (target.kind === "configured") return { endpoint, localToken: target.localToken };
      const host = target.kind === "host" ? defaultTarget() : undefined;
      return { endpoint, localToken: host?.endpoint === endpoint ? host.localToken : undefined };
    },
    defaultTarget,
    replace(next) {
      const current = configuration.read();
      if (current.endpoint === next.endpoint && current.localToken === next.localToken)
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
