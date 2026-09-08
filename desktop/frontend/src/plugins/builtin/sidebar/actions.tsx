import * as stylex from "@stylexjs/stylex";
import { ariaKeyShortcuts, comboGlyph } from "@/lib/combo";
import { MCP_SERVERS_PANE, SCHEDULES_PANE } from "@/plugins/builtin/settings/kit/panes";
import {
  SESSION_SEARCH_COMMAND,
  openSessionSearch,
} from "@/plugins/builtin/command/session-search/public/actions";
import { AgentRow } from "@/ui/agent";
import { Kbd } from "@/ui";
import { useT } from "@/lib/i18n";
import {
  contributeWorkIndexItem,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { COMMAND, definePlugin, useExtensionByKey } from "@/plugins/sdk";
import { space } from "@/styles/tokens.stylex";

const sb = stylex.create({
  stack: { display: "flex", flexDirection: "column", gap: space.s2 },
  column: { display: "flex", flexDirection: "column" },
});

export function SidebarActions() {
  const t = useT();
  const actions = useWorkIndexActions();
  // The key belongs to the command, not to this row: a hint spelled here would keep saying ⌘K
  // after the command moved.
  const combo = useExtensionByKey(COMMAND, SESSION_SEARCH_COMMAND)?.combo;

  return (
    <div {...stylex.props(sb.stack)}>
      <AgentRow
        icon="search"
        onClick={openSessionSearch}
        aria-haspopup="dialog"
        aria-keyshortcuts={combo ? ariaKeyShortcuts(combo) : undefined}
        look="search"
        trailing={combo ? <Kbd variant="inline">{comboGlyph(combo)}</Kbd> : undefined}
      >
        {t("sessionSearch.placeholder")}
      </AgentRow>
      <div {...stylex.props(sb.column)}>
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
      order: -10,
      component: SidebarActions,
    });
  },
});
