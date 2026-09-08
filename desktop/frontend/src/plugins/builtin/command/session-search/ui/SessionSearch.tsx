import * as stylex from "@stylexjs/stylex";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n";
import { EmptyState, Icon, SearchOverlay } from "@/ui";
import { selectAgentSession, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { matchSessions } from "../application/sessionMatches";
import { useSessionSearchStore } from "../application/sessionSearchState";
import { color, space, type as typeStep } from "@/styles/tokens.stylex";

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
              <Icon name="chat" size="sm" className={stylex.props(cmd.glyph).className} />
              <span {...stylex.props(cmd.label)}>{session.title}</span>
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
