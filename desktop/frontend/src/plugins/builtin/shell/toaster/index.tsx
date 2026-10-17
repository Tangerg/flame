import * as stylex from "@stylexjs/stylex";
import { Toaster } from "sonner";
import { useScheme } from "@/lib/appearance";
import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { color, radius, surface, type as typeStep, weight } from "@flame/ui/tokens.stylex";

const ts = stylex.create({
  toast: {
    borderRadius: radius.floatingPanel,
    borderWidth: 0,
    backgroundColor: surface.card,
    color: color.fg,
    fontFamily: "var(--font-sans)",
    boxShadow: "var(--shadow-overlay)",
  },
  title: { fontWeight: weight.medium },
  description: { color: color.fgMuted },
});

const CLASS_NAMES = {
  toast: stylex.props(ts.toast).className,
  title: stylex.props(typeStep.uiMd, ts.title).className,
  description: stylex.props(typeStep.uiMd, ts.description).className,
};

export function AppToaster() {
  const scheme = useScheme();
  return (
    <Toaster
      position="bottom-right"
      theme={scheme}
      duration={4000}
      toastOptions={{ classNames: CLASS_NAMES }}
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
