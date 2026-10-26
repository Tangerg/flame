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
  bootstrap: RuntimeEndpointTarget,
): RuntimeEndpointConfiguration {
  const defaultEndpoint = normalizeRuntimeEndpoint(bootstrap.endpoint);
  if (!defaultEndpoint) throw new Error("invalid bootstrap Runtime endpoint");
  const defaultTarget = { ...bootstrap, endpoint: defaultEndpoint };
  let credential = { endpoint: defaultEndpoint, value: bootstrap.localToken };
  ctx.cleanup(() => {
    credential = { endpoint: "", value: undefined };
  });
  const stored = ctx.storage.get(STORAGE_KEY);
  const restored = typeof stored === "string" ? normalizeRuntimeEndpoint(stored) : null;
  ctx.config.set(CONFIG_KEY, restored ?? defaultEndpoint);

  const configuration: RuntimeEndpointConfiguration = {
    read: () => {
      const value = ctx.config.get(CONFIG_KEY);
      const endpoint = typeof value === "string" ? normalizeRuntimeEndpoint(value) : null;
      if (!endpoint) throw new Error("invalid configured Runtime endpoint");
      return {
        endpoint,
        localToken: credential.endpoint === endpoint ? credential.value : undefined,
      };
    },
    defaultTarget: () => defaultTarget,
    replace: (target) =>
      replaceConnection(() => {
        credential = { endpoint: target.endpoint, value: target.localToken };
        ctx.config.set(CONFIG_KEY, target.endpoint);
      }),
  };

  const disposeStorageMirror = ctx.config.onChange(CONFIG_KEY, (value) => {
    if (typeof value === "string") ctx.storage.set(STORAGE_KEY, value);
  });

  ctx.cleanup(() => disposeStorageMirror.dispose());
  return configuration;
}
