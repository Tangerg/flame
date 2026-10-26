import { useAgent } from "@/features/agent/public/agent";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n/index";
import { Icon } from "@/ui/index";
import { SearchOverlay } from "@/ui/index";
import { EmptyState, vocab } from "@flame/ui";
import { selectAgentSession } from "@/features/agent/public/session";
import { useAgentSessions } from "@/features/agent/public/session";
import { matchSessions } from "@/features/navigation/session-search/sessionMatches";
import { useSessionSearchStore } from "@/features/navigation/session-search/sessionSearchState";
import { color, type as typeStep } from "@flame/ui/tokens.stylex";
const cmd = stylex.create({
  glyph: { flexShrink: 0, color: color.fgMuted },
  label: {
    minWidth: 0,
    flex: 1,
  },
  stamp: { flexShrink: 0, color: color.fgFaint },
});

export function SessionSearch() {
  const agentAccess = useAgent();

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
          icon={iconGlyph("chat")}
          size="compact"
          title={t("sessionSearch.empty.title")}
          sub={t("sessionSearch.empty.sub")}
        />
      }
      options={(query) =>
        matchSessions(sessions ?? [], query).map((session) => ({
          key: session.id,
          onSelect: () => selectAgentSession(agentAccess, session.id),
          children: (
            <>
              <Icon name="chat" size="sm" className={stylex.props(cmd.glyph).className} />
              <span {...stylex.props(cmd.label, vocab.truncate)}>{session.title}</span>
              <span {...stylex.props(cmd.stamp, typeStep.uiSm)}>
                {formatRelative(session.time)}
              </span>
            </>
          ),
        }))
      }
    />
  );
}
