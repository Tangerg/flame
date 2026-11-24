import { useWorkspace } from "@/features/workspace/public/workspace";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n/index";
import { normalizeHotkey, formatForDisplay } from "@tanstack/hotkeys";
import { Icon } from "@/ui/index";
import { Kbd } from "@/ui/index";
import { SearchOverlay } from "@/ui/index";
import { EmptyState } from "@flame/ui";
import { knownIconName } from "@/ui/icons/index";
import { useEffectiveCommands } from "@/features/command/public/shortcutOverrides";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { useWorkspaceViews } from "@/features/workspace/public/views";
import { useContextDockCatalog } from "@/features/workspace/public/contextDockCatalog";
import {
  WORKSPACE_SETTINGS_VIEW,
  openWorkspaceView,
  openWorkspaceViewInDock,
} from "@/features/workspace/public/navigation";
import { matchCommands } from "@/features/command/menu/application/commandMatches";
import { useCommandMenuStore } from "@/features/command/menu/application/commandMenuState";
import { COMMAND_MENU_COMMAND } from "@/features/command/menu/public/commandMenu";
import { color, leading } from "@flame/ui/tokens.stylex";
const cmd = stylex.create({
  glyph: { flexShrink: 0, color: color.fgMuted },
  glyphSlot: { height: "var(--icon-sm)", width: "var(--icon-sm)", flexShrink: 0 },
  label: {
    minWidth: 0,
    flex: 1,
    whiteSpace: "normal",
    overflowWrap: "anywhere",
    lineHeight: leading.body,
  },
  keys: { flexShrink: 0 },
});

interface CommandChoice {
  key: string;
  label: string;
  icon?: string;
  combo?: string;
  run: (origin?: HTMLElement) => void;
}

export function CommandMenu() {
  const workspaceAccess = useWorkspace();

  const t = useT();
  const preparedActions = useCommandMenuStore((state) => state.preparedActions);
  const setOpen = useCommandMenuStore((state) => state.setOpen);
  const commands = useEffectiveCommands();
  const catalog = useContextDockCatalog();
  const views = useWorkspaceViews();

  const docked = new Set(
    catalog.flatMap((group) => group.destinations.map((destination) => destination.viewId)),
  );
  const viewRow = (id: string, title: string, icon: string | undefined): CommandChoice => ({
    key: `view:${id}`,
    label: t("commandMenu.view", { title: t(title) }),
    icon,
    run: (origin) =>
      docked.has(id)
        ? openWorkspaceViewInDock(workspaceAccess, id, origin)
        : openWorkspaceView(workspaceAccess, id, origin),
  });

  const choices: CommandChoice[] = [
    ...commands.flatMap((command) => {
      // Inside the open menu its own toggle would only close it.
      if (command.id === COMMAND_MENU_COMMAND) return [];
      let run = () => void command.run();
      if (command.prepareForMenu) {
        const registration = lookupExtensionByKey(COMMAND, command.id);
        const action = registration && preparedActions?.get(registration);
        if (!action) return [];
        run = () => {
          if (lookupExtensionByKey(COMMAND, command.id) === registration) void action();
        };
      }
      return [{ key: command.id, label: t(command.label), combo: command.combo, run }];
    }),
    ...views
      .filter((view) => view.id !== WORKSPACE_SETTINGS_VIEW)
      .map((view) => viewRow(view.id, view.title, view.icon)),
  ];

  return (
    <SearchOverlay
      open={preparedActions !== null}
      onOpenChange={setOpen}
      label={t("command.openCommandMenu")}
      placeholder={t("commandMenu.placeholder")}
      empty={
        <EmptyState
          icon={iconGlyph("command")}
          size="compact"
          title={t("commandMenu.empty.title")}
          sub={t("commandMenu.empty.sub")}
        />
      }
      options={(query) =>
        matchCommands(choices, query).map((choice) => ({
          key: choice.key,
          onSelect: choice.run,
          children: (
            <>
              {knownIconName(choice.icon) ? (
                <Icon
                  name={knownIconName(choice.icon)!}
                  size="sm"
                  className={stylex.props(cmd.glyph).className}
                />
              ) : (
                <span aria-hidden {...stylex.props(cmd.glyphSlot)} />
              )}
              <span {...stylex.props(cmd.label)}>{choice.label}</span>
              {choice.combo && (
                <Kbd variant="inline" {...stylex.props(cmd.keys)}>
                  {formatForDisplay(normalizeHotkey(choice.combo))}
                </Kbd>
              )}
            </>
          ),
        }))
      }
    />
  );
}
