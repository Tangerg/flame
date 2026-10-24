import * as stylex from "@stylexjs/stylex";
import { type CSSProperties, useLayoutEffect, useRef } from "react";
import { Toaster, useSonner } from "sonner";
import { useScheme } from "@/lib/appearance";
import { useT } from "@/lib/i18n/index";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { color, radius, surface, type as typeStep, weight } from "@flame/ui/tokens.stylex";

const TOAST_WIDTH = "356px";
const BAND_TOP = "var(--notification-top, var(--surface-header-height))";
const BAND_BOTTOM = "var(--notification-bottom, 0px)";
const BAND_HEIGHT = `calc(100dvh - ${BAND_TOP} - ${BAND_BOTTOM})`;
const VERTICAL_GUTTER = `min(var(--notification-gutter), max(0px, calc((${BAND_HEIGHT} - var(--notification-min-reading-height)) / 2)))`;

const ts = stylex.create({
  frame: {
    position: "fixed",
    insetInlineEnd: "var(--notification-gutter)",
    bottom: `calc(${BAND_BOTTOM} + ${VERTICAL_GUTTER})`,
    zIndex: "var(--layer-notification)",
    width: `min(calc(${TOAST_WIDTH} + 2 * var(--notification-list-padding)), calc(100% - 2 * var(--notification-gutter)))`,
    pointerEvents: "none",
  },
  list: {
    outlineColor: { default: null, ":focus-visible": surface.fieldFocus },
    outlineStyle: { default: null, ":focus-visible": "solid" },
    outlineWidth: { default: null, ":focus-visible": "2px" },
    outlineOffset: "-2px",
  },
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

// Sonner positions its list and stacks toasts absolutely; these public style props turn the
// same list into a bounded normal-flow scroller inside the measured reading band.
const LIST_STYLE: CSSProperties = {
  position: "relative",
  top: 0,
  right: 0,
  bottom: 0,
  left: 0,
  display: "flex",
  flexDirection: "column",
  gap: "var(--gap)",
  width: "100%",
  height: "auto",
  maxHeight: `max(0px, calc(${BAND_HEIGHT} - 2 * ${VERTICAL_GUTTER}))`,
  padding: "var(--notification-list-padding)",
  overflowY: "auto",
  overscrollBehavior: "contain",
  pointerEvents: "auto",
};

// Only the stacking offset is neutralized; Sonner's swipe transform still composes with --y.
const TOAST_STYLE = {
  position: "relative",
  width: "100%",
  height: "auto",
  flexShrink: 0,
  padding: "var(--notification-toast-padding)",
  "--y": "translate(0, 0)",
} as CSSProperties;

export function AppToaster() {
  const t = useT();
  const scheme = useScheme();
  const frameRef = useRef<HTMLDivElement>(null);
  const { toasts } = useSonner();
  const newestId = toasts[0]?.id;

  useLayoutEffect(() => {
    // Wails otherwise follows WebKit's reduced Tab order for native buttons.
    const buttons = frameRef.current?.querySelectorAll<HTMLButtonElement>(
      "[data-sonner-toast] [data-button], [data-sonner-toast] [data-close-button]",
    );
    for (const button of buttons ?? []) {
      if (!button.hasAttribute("tabindex")) button.tabIndex = 0;
    }
  }, [toasts]);

  useLayoutEffect(() => {
    const list = frameRef.current?.querySelector<HTMLElement>("[data-sonner-toaster]");
    if (newestId === undefined || !list) return;
    if (list.matches(":hover") || list.contains(document.activeElement)) return;
    list.scrollTop = 0;
  }, [newestId]);

  return (
    <div ref={frameRef} {...stylex.props(ts.frame)}>
      <Toaster
        position="bottom-right"
        theme={scheme}
        duration={4000}
        expand
        visibleToasts={Infinity}
        className={stylex.props(ts.list).className}
        containerAriaLabel={t("notifications.title")}
        style={LIST_STYLE}
        toastOptions={{
          classNames: CLASS_NAMES,
          style: TOAST_STYLE,
          closeButtonAriaLabel: t("notifications.dismiss"),
        }}
      />
    </div>
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
