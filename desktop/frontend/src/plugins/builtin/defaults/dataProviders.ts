import { definePlugin } from "@/plugins/sdk";
import { registerDefaultDataProviders } from "./adapters/runtimeDataProviders";

export const defaultDataProviders = definePlugin({
  name: "flame.builtin.default-data",
  setup(ctx) {
    registerDefaultDataProviders(ctx);
  },
});
