import { useT } from "@/lib/i18n";
import { splitCombo } from "@/lib/combo";
import { EmptyState, Icon, Kbd, SearchOverlay } from "@/ui";
import { COMMAND, useExtensionPoint } from "@/plugins/sdk";
import { matchCommands } from "../application/commandMatches";
import { useCommandMenuStore } from "../application/commandMenuState";

export function CommandMenu() {
  const t = useT();
  const open = useCommandMenuStore((state) => state.open);
  const setOpen = useCommandMenuStore((state) => state.setOpen);
  const commands = useExtensionPoint(COMMAND);

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
        matchCommands(
          commands.map((command) => ({
            id: command.id,
            label: t(command.label),
            combo: command.combo,
          })),
          query,
        ).map((command) => ({
          key: command.id,
          onSelect: () => {
            setOpen(false);
            void commands.find((c) => c.id === command.id)?.run();
          },
          children: (
            <>
              <Icon name="command" size="sm" className="shrink-0 text-fg-muted" />
              <span className="min-w-0 flex-1 truncate">{command.label}</span>
              {command.combo && (
                <span className="flex shrink-0 items-center gap-1">
                  {splitCombo(command.combo).map((part, index) => (
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
