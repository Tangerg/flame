import { useTheme } from "@/features/theme/public/theme";
import * as stylex from "@stylexjs/stylex";
import { AnimatePresence, motion } from "motion/react";
import { useMotion } from "@/lib/motion";
import { AgentRow } from "@/ui/agent/index";
import { AgentWorkIndexFooter } from "@/ui/agent/index";
import { Icon } from "@/ui/index";
import { Button } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { useWorkIndexActions } from "@/features/navigation/public/workIndex";
import { isLightTheme, toggleThemeScheme } from "@/features/theme/public/scheme";
import { Slot } from "@/extensions/host/Slot";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { useAppearance } from "@/features/theme/public/appearance";
import { space } from "@flame/ui/tokens.stylex";
const sf = stylex.create({
  centre: { display: "grid", placeItems: "center" },
  statusLine: { display: "flex", alignItems: "center", gap: space.s0_5 },
});

function ThemeToggle() {
  const { glyphSwapTransition } = useMotion();
  const t = useT();
  const themeAccess = useTheme();
  const theme = useAppearance((s) => s.theme);
  const isLight = isLightTheme(themeAccess, theme);
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      onClick={() => toggleThemeScheme(themeAccess)}
      data-chrome-focus=""
      title={t(isLight ? "theme.switchToDark" : "theme.switchToLight")}
    >
      <AnimatePresence initial={false} mode="popLayout">
        <motion.span
          key={isLight ? "moon" : "sun"}
          className={stylex.props(sf.centre).className}
          initial={{ opacity: 0, scale: 0.25, filter: "blur(4px)" }}
          animate={{ opacity: 1, scale: 1, filter: "blur(0px)" }}
          exit={{ opacity: 0, scale: 0.25, filter: "blur(4px)" }}
          transition={glyphSwapTransition}
        >
          <Icon name={isLight ? "moon" : "sun"} size="sm" />
        </motion.span>
      </AnimatePresence>
    </Button>
  );
}

function SidebarFooter() {
  const t = useT();
  const actions = useWorkIndexActions();

  return (
    <AgentWorkIndexFooter>
      <AgentRow icon="settings" flex="fill" onClick={actions.openSettings}>
        {t("sidebar.action.settings")}
      </AgentRow>
      <Slot name="sidebar.footer.status" className={stylex.props(sf.statusLine).className} />
      <ThemeToggle />
    </AgentWorkIndexFooter>
  );
}

export const sidebarFooter = definePlugin({
  name: "flame.builtin.sidebar-footer",
  setup(ctx) {
    contributeLayout(ctx, "sidebar.footer", {
      id: "user-card",
      order: 0,
      component: SidebarFooter,
    });
  },
});
