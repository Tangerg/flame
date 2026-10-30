import { type FlameClient } from "@flame/runtime-client";
import { registerProviderDataProviders } from "@/features/providers/adapters/runtimeDataProviders";
import { definePlugin } from "@/extensions/definePlugin";

export function createProviderCatalogPlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.provider-catalog",
    setup(ctx) {
      registerProviderDataProviders(ctx, runtimeClient);
    },
  });
}
