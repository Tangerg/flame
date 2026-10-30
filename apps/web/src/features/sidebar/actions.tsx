import { useWorkspace } from "@/features/workspace/public/workspace";
import * as stylex from "@stylexjs/stylex";
import { ariaKeyShortcuts, comboGlyph } from "@/lib/combo";
import { useSettingsPaneForIntent } from "@/features/settings/public/intent";
import {
  SESSION_SEARCH_COMMAND,
  openSessionSearch,
} from "@/features/navigation/public/search/actions";
import { AgentRow } from "@/ui/agent/index";
import { Kbd } from "@/ui/index";
import { vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { contributeWorkIndexItem } from "@/features/navigation/public/workIndex";
import { useWorkIndexActions } from "@/features/navigation/public/workIndex";
import { openWorkspaceSettingsPane } from "@/features/workspace/public/navigation";
import { definePlugin } from "@/extensions/definePlugin";
import { useCommandCombo } from "@/features/command/public/shortcutOverrides";
import { space } from "@flame/ui/tokens.stylex";
const sb = stylex.create({
  stack: { display: "flex", flexDirection: "column", gap: space.s2 },
});

export function SidebarActions() {
  const workspaceAccess = useWorkspace();

  const t = useT();
  const actions = useWorkIndexActions();
  const toolsPane = useSettingsPaneForIntent("mcpServers");
  const combo = useCommandCombo(SESSION_SEARCH_COMMAND);

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
      <div {...stylex.props(vocab.column)}>
        <AgentRow
          icon="compose"
          disabled={!actions.canCreateSession}
          onClick={actions.createSession}
        >
          {t("sidebar.action.newSession")}
        </AgentRow>
        <AgentRow
          icon="server"
          onClick={() => openWorkspaceSettingsPane(workspaceAccess, toolsPane)}
        >
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
