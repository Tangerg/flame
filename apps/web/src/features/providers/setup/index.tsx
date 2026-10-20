import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { ProviderSetupPrompt } from "@/features/providers/setup/ui/ProviderSetupPrompt";
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
