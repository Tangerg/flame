import { AgentSurfaceHeader, AgentWorkIndexBody, AgentWorkIndexSection } from "@/ui/agent";
import { useWorkIndexItems } from "@/plugins/builtin/navigation/public/workIndex";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { Slot } from "@/plugins/host/Slot";

export function SidebarPanel() {
  const items = useWorkIndexItems();

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <AgentSurfaceHeader divider={false} className="agent-drawer-header">
        <span className="min-w-2 flex-1" />
      </AgentSurfaceHeader>

      <AgentWorkIndexBody>
        {items.map((item) => {
          const Body = item.component;
          return (
            <AgentWorkIndexSection key={item.id}>
              <PluginBoundary plugin={`work-index:${item.id}`} label={`${item.id} work index item`}>
                <Body />
              </PluginBoundary>
            </AgentWorkIndexSection>
          );
        })}
      </AgentWorkIndexBody>

      <div className="mt-auto shrink-0">
        <Slot name="sidebar.footer" />
      </div>
    </div>
  );
}
