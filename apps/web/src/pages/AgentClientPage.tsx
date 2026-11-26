import { AgentAppShell } from "@/ui/agent/index";
import { useT } from "@/lib/i18n/index";
import { Slot } from "@/extensions/host/Slot";
import {
  useActiveWorkspaceViewId,
  WORKSPACE_SETTINGS_VIEW,
} from "@/features/workspace/public/navigation";
import { useSidebarDrawer, useSidebarLayout } from "@/features/workspace/public/sidebarDrawer";
export function AgentClientPage() {
  const t = useT();
  const drawer = useSidebarDrawer();
  const { layout, save } = useSidebarLayout();
  const activeViewId = useActiveWorkspaceViewId();

  return (
    <AgentAppShell
      sidebarLabel={t("shell.region.workIndex")}
      sidebarResizeLabel={t("sidebar.action.resize")}
      sidebarOpen={!drawer.collapsed}
      sidebarCovered={activeViewId === WORKSPACE_SETTINGS_VIEW}
      sidebarLayout={layout}
      onLayoutChanged={save}
      onSidebarToggle={drawer.toggle}
      sidebarExpandLabel={t("sidebar.action.expand")}
      sidebarCollapseLabel={t("sidebar.action.collapse")}
      sidebar={<Slot name="app.sidebar" />}
      main={<Slot name="app.main" />}
      overlay={<Slot name="app.overlay" />}
    />
  );
}
