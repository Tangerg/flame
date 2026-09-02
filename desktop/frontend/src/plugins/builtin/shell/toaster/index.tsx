import { Toaster } from "sonner";
import { contributeLayout, definePlugin } from "@/plugins/sdk";

// Presentation only. Raising a toast is `notifyInfo` / `notifyError` / `host.notify`, which
// call sonner directly — this contributes the surface they land on and nothing else.
export function AppToaster() {
  return (
    <Toaster
      position="bottom-right"
      theme="system"
      duration={4000}
      toastOptions={{
        classNames: {
          toast: "rounded-xl bg-canvas text-fg shadow-[var(--shadow-overlay)]",
          title: "text-ui-md font-medium",
          description: "text-ui-md text-fg-muted",
        },
      }}
    />
  );
}

export default definePlugin({
  name: "flame.builtin.toaster",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", {
      id: "toaster",
      order: 100,
      component: AppToaster,
    });
  },
});
