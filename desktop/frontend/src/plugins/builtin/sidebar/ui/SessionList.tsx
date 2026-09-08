import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { TextButton } from "@/ui";
import { SessionRow } from "./SessionRow";
import { useT } from "@/lib/i18n";
import type { WorkIndexActions, WorkSession } from "@/plugins/builtin/navigation/public/workIndex";
import { space } from "@/styles/tokens.stylex";

const sl = stylex.create({
  column: { display: "flex", flexDirection: "column" },
  more: { paddingInline: space.s2, paddingBlock: space.s1 },
  // Lines up with the nested rows above it: their inset plus the glyph they leave room for.
  moreNested: { paddingLeft: "calc(0.5rem + var(--icon-sm) + var(--density-row-gap))" },
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
  const visible = showAll ? sessions : sessions.slice(0, VISIBLE_CAP);
  const hidden = sessions.length - visible.length;

  return (
    <div {...stylex.props(sl.column)}>
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
