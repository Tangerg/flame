import type { ConfigService, KeyValueStore } from "@/plugins/sdk";
import { getContainer } from "@/main/container";
import {
  configureRuntimeEndpoint,
  type RuntimeEndpointTarget,
} from "../application/ports/runtimeEndpoint";
import { normalizeRuntimeEndpoint } from "@flame/runtime-contract/client/endpoint";

const CONFIG_KEY = "runtime.endpoint";
const STORAGE_KEY = "endpoint";

interface EndpointBindings {
  config: ConfigService;
  storage: KeyValueStore;
}

export type ReplaceRuntimeEndpoint = (commit: () => void) => void;

export function installRuntimeEndpointConfiguration(
  ctx: EndpointBindings,
  replaceConnection: ReplaceRuntimeEndpoint,
  bootstrap: RuntimeEndpointTarget = getContainer().bootstrap().runtime,
): () => void {
  const defaultEndpoint = normalizeRuntimeEndpoint(bootstrap.endpoint);
  if (!defaultEndpoint) throw new Error("invalid bootstrap Runtime endpoint");
  const defaultTarget = { ...bootstrap, endpoint: defaultEndpoint };
  let credential = { endpoint: defaultEndpoint, value: bootstrap.localToken };
  const stored = ctx.storage.get(STORAGE_KEY);
  const restored = typeof stored === "string" ? normalizeRuntimeEndpoint(stored) : null;
  ctx.config.set(CONFIG_KEY, restored ?? defaultEndpoint);

  const disposePort = configureRuntimeEndpoint({
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
  });

  const disposeStorageMirror = ctx.config.onChange(CONFIG_KEY, (value) => {
    if (typeof value === "string") ctx.storage.set(STORAGE_KEY, value);
  });

  return () => {
    credential = { endpoint: "", value: undefined };
    disposeStorageMirror.dispose();
    disposePort();
  };
}
