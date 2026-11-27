import { useAgent } from "@/features/agent/public/agent";
import { useComposer } from "@/features/composer/public/composer";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n/index";
import { DataError, SearchOverlay, SkeletonList } from "@/ui/index";
import { EmptyState } from "@flame/ui";

import { sessionDisplayTitle, useAgentSessions } from "@/features/agent/public/session";
import { matchSessions } from "@/features/navigation/session-search/sessionMatches";
import { useSessionSearchStore } from "@/features/navigation/session-search/sessionSearchState";
import { color, face, leading, space, type as typeStep } from "@flame/ui/tokens.stylex";
const cmd = stylex.create({
  label: {
    minWidth: 0,
    flex: 1,
    whiteSpace: "normal",
    overflowWrap: "anywhere",
    lineHeight: leading.body,
  },
  path: {
    display: "block",
    marginBlockStart: space.s0_5,
    color: color.fgMuted,
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    lineHeight: leading.snug,
  },
  stamp: { flexShrink: 0, color: color.fgFaint },
});

export function SessionSearch() {
  const agentAccess = useAgent();
  const { captureEditorFocusReturn } = useComposer();

  const t = useT();
  const open = useSessionSearchStore((state) => state.open);
  const setOpen = useSessionSearchStore((state) => state.setOpen);
  const { data: sessions, isError, error, refetch } = useAgentSessions();

  return (
    <SearchOverlay
      open={open}
      onOpenChange={setOpen}
      captureFocusReturn={captureEditorFocusReturn}
      label={t("sessionSearch.label")}
      placeholder={t("sessionSearch.placeholder")}
      feedback={
        sessions !== undefined ? undefined : isError ? (
          <div role="alert">
            <DataError failure={error} onRetry={() => void refetch()} size="compact" />
          </div>
        ) : (
          <SkeletonList label={t("common.loading")} variant="compact" />
        )
      }
      empty={
        <EmptyState
          icon={iconGlyph("chat")}
          size="compact"
          title={t("sessionSearch.empty.title")}
        />
      }
      options={(query) => {
        if (sessions === undefined) return [];
        const matches = matchSessions(sessions, query).map((session) => ({
          session,
          title: sessionDisplayTitle(t, session.title),
          path: session.workspace.path,
        }));
        return matches.map(({ session, title, path }) => {
          const showPath =
            path !== "" &&
            matches.some(
              (other) => other.title === title && other.path !== "" && other.path !== path,
            );
          return {
            key: session.id,
            onSelect: () => agentAccess.selectAgentSession(session.id),
            children: (
              <>
                <span {...stylex.props(cmd.label)}>
                  {title}
                  {showPath && (
                    <span dir="ltr" {...stylex.props(cmd.path, face.mono, typeStep.uiXs)}>
                      {path}
                    </span>
                  )}
                </span>
                <span {...stylex.props(cmd.stamp, typeStep.uiSm)}>
                  {formatRelative(session.time)}
                </span>
              </>
            ),
          };
        });
      }}
    />
  );
}
