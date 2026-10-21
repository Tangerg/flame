import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { TextButton } from "@/ui/index";
import { vocab } from "@flame/ui";
import { SessionRow } from "@/features/sidebar/ui/SessionRow";
import { useT } from "@/lib/i18n/index";
import { type WorkIndexActions } from "@/features/navigation/public/workIndex";
import { type WorkSession } from "@/features/navigation/public/workIndex";
import { space } from "@flame/ui/tokens.stylex";
const sl = stylex.create({
  more: { paddingInline: space.s2, paddingBlock: space.s1 },
  moreNested: { paddingLeft: "var(--nested-row-indent)" },
});

const VISIBLE_CAP = 5;

export function SessionList({
  sessions,
  actions,
  activeSessionId,
  indented = false,
  showTime = true,
}: {
  sessions: readonly WorkSession[];
  actions: WorkIndexActions;
  activeSessionId: string;
  indented?: boolean;
  showTime?: boolean;
}) {
  const t = useT();
  const [showAll, setShowAll] = useState(false);
  const visible = showAll
    ? sessions
    : sessions.filter((session, index) => index < VISIBLE_CAP || session.id === activeSessionId);
  const hidden = sessions.length - visible.length;

  return (
    <div {...stylex.props(vocab.column)}>
      {visible.map((session) => (
        <SessionRow
          key={session.id}
          session={session}
          active={session.id === activeSessionId}
          indented={indented}
          showTime={showTime}
          onSelect={actions.selectSession}
          onRename={actions.renameSession}
          onFork={actions.forkSession}
          onDelete={actions.deleteSession}
          onToggleFavorite={actions.toggleFavorite}
        />
      ))}
      {(hidden > 0 || showAll) && (
        <TextButton
          type="button"
          onClick={() => setShowAll((open) => !open)}
          shape="row"
          size="xs"
          tone="faint"
          className={stylex.props(sl.more, indented && sl.moreNested).className}
        >
          {hidden > 0 ? t("projects.showMore", { count: hidden }) : t("projects.showLess")}
        </TextButton>
      )}
    </div>
  );
}
