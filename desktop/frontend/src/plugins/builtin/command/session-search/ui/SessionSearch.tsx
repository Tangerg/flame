import { formatRelative } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n";
import { EmptyState, Icon, SearchOverlay } from "@/ui";
import { selectAgentSession, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { matchSessions } from "../application/sessionMatches";
import { useSessionSearchStore } from "../application/sessionSearchState";

export function SessionSearch() {
  const t = useT();
  const open = useSessionSearchStore((state) => state.open);
  const setOpen = useSessionSearchStore((state) => state.setOpen);
  const { data: sessions } = useAgentSessions();

  return (
    <SearchOverlay
      open={open}
      onOpenChange={setOpen}
      label={t("sessionSearch.label")}
      placeholder={t("sessionSearch.placeholder")}
      empty={
        <EmptyState
          icon="chat"
          size="compact"
          title={t("sessionSearch.empty.title")}
          sub={t("sessionSearch.empty.sub")}
        />
      }
      options={(query) =>
        matchSessions(sessions ?? [], query).map((session) => ({
          key: session.id,
          onSelect: () => {
            selectAgentSession(session.id);
            setOpen(false);
          },
          children: (
            <>
              <Icon name="chat" size="sm" className="shrink-0 text-fg-muted" />
              <span className="min-w-0 flex-1 truncate">{session.title}</span>
              <span className="shrink-0 text-ui-sm text-fg-faint">
                {formatRelative(session.time)}
              </span>
            </>
          ),
        }))
      }
    />
  );
}
