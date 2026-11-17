import { AgentAppShell } from "@/ui/agent/index";
import { useT } from "@/lib/i18n/index";
import { Slot } from "@/extensions/host/Slot";
import { useActiveWorkspaceViewId } from "@/features/workspace/public/navigation";
import { useSidebarDrawer, useSidebarLayout } from "@/features/workspace/public/sidebarDrawer";
export function AgentClientPage() {
  const t = useT();
  const drawer = useSidebarDrawer();
  const { layout, save } = useSidebarLayout();
  const activeViewId = useActiveWorkspaceViewId();
  const singleMode = activeViewId === "settings";

  return (
    <AgentAppShell
      sidebarLabel={t("shell.region.workIndex")}
      sidebarResizeLabel={t("sidebar.action.resize")}
      sidebarOpen={!drawer.collapsed}
      sidebarLayout={layout}
      onLayoutChanged={save}
      onSidebarToggle={drawer.toggle}
      sidebarExpandLabel={t("sidebar.action.expand")}
      sidebarCollapseLabel={t("sidebar.action.collapse")}
      sidebar={singleMode ? undefined : <Slot name="app.sidebar" />}
      main={<Slot name="app.main" />}
      overlay={<Slot name="app.overlay" />}
    />
  );
}
