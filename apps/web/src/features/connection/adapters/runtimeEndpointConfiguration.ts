import { type ConfigService } from "@/extensions/config";
import { type KeyValueStore } from "@/extensions/storage";
import { type LifetimeOperations } from "dougong";
import {
  type RuntimeEndpointConfiguration,
  type RuntimeEndpointTarget,
} from "@/features/connection/application/runtimeEndpoint";
import { normalizeRuntimeEndpoint } from "@flame/runtime-client";
const CONFIG_KEY = "runtime.endpoint";

const STORAGE_KEY = "endpoint";

interface EndpointBindings extends Pick<LifetimeOperations, "cleanup"> {
  config: ConfigService;
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
  let credential:
    { kind: "host" } | { kind: "configured"; target: RuntimeEndpointTarget } | { kind: "retired" } =
    { kind: "host" };
  ctx.cleanup(() => {
    credential = { kind: "retired" };
  });
  const stored = ctx.storage.get(STORAGE_KEY);
  const restored = typeof stored === "string" ? normalizeRuntimeEndpoint(stored) : null;
  ctx.config.set(CONFIG_KEY, restored ?? defaultTarget().endpoint);

  const configuration: RuntimeEndpointConfiguration & { usesHostCredential(): boolean } = {
    usesHostCredential: () =>
      credential.kind === "host" && configuration.read().endpoint === defaultTarget().endpoint,
    read: () => {
      const value = ctx.config.get(CONFIG_KEY);
      const endpoint = typeof value === "string" ? normalizeRuntimeEndpoint(value) : null;
      if (!endpoint) throw new Error("invalid configured Runtime endpoint");
      const source =
        credential.kind === "host"
          ? defaultTarget()
          : credential.kind === "configured"
            ? credential.target
            : null;
      return {
        endpoint,
        localToken: source?.endpoint === endpoint ? source.localToken : undefined,
      };
    },
    defaultTarget,
    replace(target) {
      const current = configuration.read();
      if (current.endpoint === target.endpoint && current.localToken === target.localToken) {
        return false;
      }
      replaceConnection(() => {
        credential = { kind: "configured", target: { ...target } };
        ctx.config.set(CONFIG_KEY, target.endpoint);
      });
      return true;
    },
    reset() {
      const target = defaultTarget();
      if (credential.kind === "host" && configuration.read().endpoint === target.endpoint) {
        return false;
      }
      replaceConnection(() => {
        credential = { kind: "host" };
        ctx.config.set(CONFIG_KEY, target.endpoint);
      });
      return true;
    },
  };

  const disposeStorageMirror = ctx.config.onChange(CONFIG_KEY, (value) => {
    if (typeof value === "string") ctx.storage.set(STORAGE_KEY, value);
  });

  ctx.cleanup(() => disposeStorageMirror.dispose());
  return configuration;
}
