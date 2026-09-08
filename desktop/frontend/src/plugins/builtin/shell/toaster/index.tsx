import * as stylex from "@stylexjs/stylex";
import { Toaster } from "sonner";
import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { color, radius, surface, type as typeStep, weight } from "@/styles/tokens.stylex";

/**
 * A toast's material, stated here rather than taken from `FLOATING_PANEL`.
 *
 * The shared material is three slices and this host can only accept two of them: sonner owns
 * the toast's entrance, its exit, its stacking and its swipe, so composing `motion` would put
 * a `transition-property` on an element another library is already animating. The two it does
 * take — the panel corner and an opaque plate — are the design's own tokens, not the
 * `rounded-xl bg-canvas` this used to spell in a framework that no longer exists.
 *
 * Opaque on purpose: `--app-floating-surface` is 90% and reads correctly only behind the blur
 * that `face` carries on a pseudo-element, which is the slice this cannot take either.
 */
const ts = stylex.create({
  toast: {
    borderRadius: radius.floatingPanel,
    backgroundColor: surface.card,
    color: color.fg,
    boxShadow: "var(--shadow-overlay)",
  },
  title: { fontWeight: weight.medium },
  description: { color: color.fgMuted },
});

// sonner takes class NAMES, one per part, so each is resolved once here rather than on
// every render of a component that renders whenever any toast does.
const CLASS_NAMES = {
  toast: stylex.props(ts.toast).className,
  title: stylex.props(typeStep.uiMd, ts.title).className,
  description: stylex.props(typeStep.uiMd, ts.description).className,
};

// Presentation only. Raising a toast is `notifyInfo` / `notifyError` / `host.notify`, which
// call sonner directly — this contributes the surface they land on and nothing else.
export function AppToaster() {
  return (
    <Toaster
      position="bottom-right"
      theme="system"
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
