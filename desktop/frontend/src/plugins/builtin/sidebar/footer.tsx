import { AnimatePresence, motion } from "motion/react";
import { AgentRow, AgentWorkIndexFooter } from "@/ui/agent";
import { Button, Icon } from "@/ui";
import { useT } from "@/lib/i18n";
import { useWorkIndexActions } from "@/plugins/builtin/navigation/public/workIndex";
import { isLightTheme, toggleThemeScheme } from "@/plugins/builtin/theme/public/scheme";
import { Slot } from "@/plugins/host/Slot";
import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";

function ThemeToggle() {
  const t = useT();
  const theme = useUiStore((s) => s.theme);
  const isLight = isLightTheme(theme);
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      onClick={() => toggleThemeScheme()}
      data-chrome-focus=""
      title={t(isLight ? "theme.switchToDark" : "theme.switchToLight")}
    >
      <AnimatePresence initial={false} mode="popLayout">
        <motion.span
          key={isLight ? "moon" : "sun"}
          className="grid place-items-center"
          initial={{ opacity: 0, scale: 0.25, filter: "blur(4px)" }}
          animate={{ opacity: 1, scale: 1, filter: "blur(0px)" }}
          exit={{ opacity: 0, scale: 0.25, filter: "blur(4px)" }}
          transition={{ type: "spring", duration: 0.3, bounce: 0 }}
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
      <AgentRow icon="settings" className="min-w-0 flex-1" onClick={actions.openSettings}>
        {t("sidebar.action.settings")}
      </AgentRow>
      <Slot name="sidebar.footer.status" className="flex items-center gap-0.5" />
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
