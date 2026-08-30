import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { ProviderSetupPrompt } from "./ui/ProviderSetupPrompt";

export default definePlugin({
  name: "flame.builtin.provider-setup",
  setup(ctx) {
    contributeLayout(ctx, "chat.empty", {
      id: "provider-setup",
      order: 0,
      component: ProviderSetupPrompt,
    });
  },
});
