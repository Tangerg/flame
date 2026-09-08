import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import { splitCombo } from "@/lib/combo";
import { EmptyState, Icon, Kbd, SearchOverlay } from "@/ui";
import { knownIconName } from "@/ui/icons";
import { COMMAND, useExtensionPoint, useWorkspaceViews } from "@/plugins/sdk";
import { useContextDockCatalog } from "@/plugins/builtin/workspace/public/contextDockCatalog";
import {
  WORKSPACE_SETTINGS_VIEW,
  openWorkspaceView,
  openWorkspaceViewInDock,
} from "@/plugins/builtin/workspace/public/navigation";
import { matchCommands, type CommandChoice } from "../application/commandMatches";
import { useCommandMenuStore } from "../application/commandMenuState";
import { color, space } from "@/styles/tokens.stylex";

const cmd = stylex.create({
  glyph: { flexShrink: 0, color: color.fgMuted },
  glyphSlot: { height: "var(--icon-sm)", width: "var(--icon-sm)", flexShrink: 0 },
  label: {
    minWidth: 0,
    flex: 1,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
  keys: { display: "flex", flexShrink: 0, alignItems: "center", gap: space.s1 },
  stamp: { flexShrink: 0, color: color.fgFaint },
});

export function CommandMenu() {
  const t = useT();
  const open = useCommandMenuStore((state) => state.open);
  const setOpen = useCommandMenuStore((state) => state.setOpen);
  const commands = useExtensionPoint(COMMAND);
  const catalog = useContextDockCatalog();
  const views = useWorkspaceViews();

  // Views are rows here for the same reason the reference lists its panels: one you can only
  // reach by opening the dock and browsing is one the keyboard cannot reach. Where it opens
  // comes from the dock catalogue rather than a second list — a view it does not carry takes
  // the whole content card, which is what settings and the icon gallery are.
  const docked = new Set(
    catalog.flatMap((group) => group.destinations.map((destination) => destination.viewId)),
  );
  const viewRow = (id: string, title: string, icon: string | undefined): CommandChoice => ({
    key: `view:${id}`,
    label: t("commandMenu.view", { title: t(title) }),
    icon,
    run: () => (docked.has(id) ? openWorkspaceViewInDock(id) : openWorkspaceView(id)),
  });

  const choices: CommandChoice[] = [
    ...commands.map((command) => ({
      key: command.id,
      label: t(command.label),
      combo: command.combo,
      run: () => void command.run(),
    })),
    // Settings has a command of its own, carrying the key the platform reserves for it; a
    // second row for the same surface would only be noise.
    ...views
      .filter((view) => view.id !== WORKSPACE_SETTINGS_VIEW)
      .map((view) => viewRow(view.id, view.title, view.icon)),
  ];

  return (
    <SearchOverlay
      open={open}
      onOpenChange={setOpen}
      label={t("command.openCommandMenu")}
      placeholder={t("commandMenu.placeholder")}
      empty={
        <EmptyState
          icon="command"
          size="compact"
          title={t("commandMenu.empty.title")}
          sub={t("commandMenu.empty.sub")}
        />
      }
      options={(query) =>
        matchCommands(choices, query).map((choice) => ({
          key: choice.key,
          onSelect: () => {
            setOpen(false);
            choice.run();
          },
          children: (
            <>
              {/* A command has no glyph of its own, and a ⌘ on every row would say what the
                  key beside it already does — twice. The slot is held so the labels of both
                  kinds of row start on the same edge. */}
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
                <span {...stylex.props(cmd.keys)}>
                  {splitCombo(choice.combo).map((part, index) => (
                    <Kbd key={index}>{part}</Kbd>
                  ))}
                </span>
              )}
            </>
          ),
        }))
      }
    />
  );
}
