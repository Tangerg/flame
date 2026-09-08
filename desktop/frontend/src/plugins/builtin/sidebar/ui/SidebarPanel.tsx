import * as stylex from "@stylexjs/stylex";
import { AgentSurfaceHeader, AgentWorkIndexBody, AgentWorkIndexSection } from "@/ui/agent";
import { useWorkIndexItems } from "@/plugins/builtin/navigation/public/workIndex";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { Slot } from "@/plugins/host/Slot";
import { space } from "@/styles/tokens.stylex";

const sp = stylex.create({
  panel: { display: "flex", minHeight: 0, minWidth: 0, flex: 1, flexDirection: "column" },
  // Takes the bar's spare width so the title beside it truncates rather than pushing.
  spacer: { minWidth: space.s2, flex: 1 },
  foot: { marginTop: "auto", flexShrink: 0 },
});

export function SidebarPanel() {
  const items = useWorkIndexItems();

  return (
    <div {...stylex.props(sp.panel)}>
      <AgentSurfaceHeader divider={false} corner="drawer">
        <span {...stylex.props(sp.spacer)} />
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

      <div {...stylex.props(sp.foot)}>
        <Slot name="sidebar.footer" />
      </div>
    </div>
  );
}
