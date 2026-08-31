import { comboGlyph } from "@/lib/combo";
import { MCP_SERVERS_PANE, SCHEDULES_PANE } from "@/plugins/builtin/settings/kit/panes";
import { openSessionSearch } from "@/plugins/builtin/command/session-search/public/actions";
import { AgentRow } from "@/ui/agent";
import { Kbd } from "@/ui";
import { useT } from "@/lib/i18n";
import {
  contributeWorkIndexItem,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { definePlugin } from "@/plugins/sdk";

export function SidebarActions() {
  const t = useT();
  const actions = useWorkIndexActions();

  return (
    <div className="flex flex-col gap-2">
      <AgentRow
        icon="search"
        onClick={openSessionSearch}
        aria-haspopup="dialog"
        aria-keyshortcuts="Meta+K Control+K"
        className="bg-sunken font-normal text-fg-muted hover:bg-hover hover:text-fg"
        trailing={
          <Kbd className="h-auto min-w-0 bg-transparent px-0 font-mono text-ui-2xs font-normal text-fg-faint">
            {comboGlyph("Mod+K")}
          </Kbd>
        }
      >
        {t("sessionSearch.placeholder")}
      </AgentRow>
      <div className="flex flex-col">
        <AgentRow icon="edit" disabled={!actions.canCreateSession} onClick={actions.createSession}>
          {t("sidebar.action.newSession")}
        </AgentRow>
        <AgentRow icon="clock" onClick={() => openWorkspaceSettingsPane(SCHEDULES_PANE)}>
          {t("settings.pane.schedules")}
        </AgentRow>
        <AgentRow icon="tool" onClick={() => openWorkspaceSettingsPane(MCP_SERVERS_PANE)}>
          {t("sidebar.action.tools")}
        </AgentRow>
      </div>
    </div>
  );
}

export const sidebarActions = definePlugin({
  name: "flame.builtin.sidebar-actions",
  setup(ctx) {
    contributeWorkIndexItem(ctx, {
      id: "actions",
      scope: "global",
      variant: "expanded",
      order: -10,
      component: SidebarActions,
    });
  },
});
