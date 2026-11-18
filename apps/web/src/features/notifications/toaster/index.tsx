import * as stylex from "@stylexjs/stylex";
import { useLayoutEffect, useRef } from "react";
import { Toast } from "@base-ui/react/toast";
import { Button, IconButton } from "@flame/ui";
import { iconGlyph } from "@/ui/icons/index";
import { useT } from "@/lib/i18n/index";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import {
  color,
  motion,
  radius,
  space,
  surface,
  type as typeStep,
  weight,
} from "@flame/ui/tokens.stylex";

const TOAST_WIDTH = "356px";
const BAND_TOP = "var(--notification-top, var(--surface-header-height))";
const BAND_BOTTOM = "var(--notification-bottom, 0px)";
const BAND_HEIGHT = `calc(100dvh - ${BAND_TOP} - ${BAND_BOTTOM})`;
const VERTICAL_GUTTER = `min(var(--notification-gutter), max(0px, calc((${BAND_HEIGHT} - var(--notification-min-reading-height)) / 2)))`;

const styles = stylex.create({
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
    position: "relative",
    display: "flex",
    flexDirection: "column",
    gap: space.s2,
    width: "100%",
    maxHeight: `max(0px, calc(${BAND_HEIGHT} - 2 * ${VERTICAL_GUTTER}))`,
    margin: 0,
    padding: "var(--notification-list-padding)",
    overflowY: "auto",
    overscrollBehavior: "contain",
    pointerEvents: "auto",
    listStyleType: "none",
  },
  item: { minWidth: 0, flexShrink: 0 },
  toast: {
    position: "relative",
    padding: "var(--notification-toast-padding)",
    borderRadius: radius.floatingPanel,
    borderWidth: 0,
    backgroundColor: surface.card,
    color: color.fg,
    fontFamily: "var(--font-sans)",
    boxShadow: "var(--shadow-overlay)",
    transform: "translate(var(--toast-swipe-movement-x, 0px), var(--toast-swipe-movement-y, 0px))",
    opacity: { default: 1, ":is([data-starting-style], [data-ending-style])": 0 },
    transitionProperty: "opacity",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
  },
  content: { display: "flex", alignItems: "start", gap: space.s2, minWidth: 0 },
  text: { flex: 1, minWidth: 0, overflowWrap: "anywhere" },
  title: { fontWeight: weight.medium },
  description: { marginBlockStart: space.s1, color: color.fgMuted, whiteSpace: "pre-wrap" },
  action: { marginBlockStart: space.s2 },
});

export function AppToaster() {
  const t = useT();
  const listRef = useRef<HTMLOListElement>(null);
  const { toasts } = Toast.useToastManager();
  const newestId = toasts[0]?.id;
  useLayoutEffect(() => {
    const list = listRef.current;
    if (newestId === undefined || !list) return;
    if (list.matches(":hover") || list.contains(list.ownerDocument.activeElement)) return;
    list.scrollTop = 0;
  }, [newestId]);

  return (
    <Toast.Portal>
      <Toast.Viewport aria-label={t("notifications.title")} {...stylex.props(styles.frame)}>
        <ol
          ref={listRef}
          // The bounded notification reader must accept keyboard scrolling independently of its buttons.
          tabIndex={0}
          aria-label={t("notifications.title")}
          {...stylex.props(styles.list)}
        >
          {toasts.map((toast) => (
            <li key={toast.id} {...stylex.props(styles.item)}>
              <Toast.Root
                data-slot="notification-toast"
                toast={toast}
                {...stylex.props(styles.toast)}
              >
                <Toast.Content {...stylex.props(styles.content)}>
                  <div {...stylex.props(styles.text)}>
                    <Toast.Title {...stylex.props(styles.title, typeStep.uiMd)} />
                    <Toast.Description {...stylex.props(styles.description, typeStep.uiMd)} />
                    <Toast.Action
                      tabIndex={0}
                      render={<Button size="sm" variant="soft" />}
                      {...stylex.props(styles.action)}
                    />
                  </div>
                  <Toast.Close
                    aria-hidden={false}
                    tabIndex={0}
                    render={
                      <IconButton
                        icon={iconGlyph("x")}
                        size="xs"
                        aria-label={t("notifications.dismiss")}
                      />
                    }
                  />
                </Toast.Content>
              </Toast.Root>
            </li>
          ))}
        </ol>
      </Toast.Viewport>
    </Toast.Portal>
  );
}

export default definePlugin({
  name: "flame.builtin.toaster",
  setup(ctx) {
    contributeLayout(ctx.contributions, "app.overlay", {
      id: "toaster",
      order: 100,
      component: AppToaster,
    });
  },
});
