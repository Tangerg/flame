import { useState } from "react";
import { TextButton } from "@/ui";
import { SessionRow } from "./SessionRow";
import { useT } from "@/lib/i18n";
import type { WorkIndexActions, WorkSession } from "@/plugins/builtin/navigation/public/workIndex";
import { cn } from "@/lib/classNames";

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
    <div className="flex flex-col">
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
          size="sm"
          onClick={() => setShowAll((open) => !open)}
          className={cn(
            // TextButton carries no height of its own — right for a link inside a sentence, which
            // WCAG exempts, and wrong for a row control like this one: at the smallest UI size its
            // box is the text line, which lands under the 24px target minimum.
            "min-h-6 rounded-[var(--row-radius)] border-0 bg-transparent px-2 py-1 text-left text-ui-xs text-fg-faint transition-colors hover:bg-hover hover:text-fg",
            indented && "pl-[calc(0.5rem+var(--icon-sm)+var(--density-row-gap))]",
          )}
        >
          {hidden > 0 ? t("projects.showMore", { count: hidden }) : t("projects.showLess")}
        </TextButton>
      )}
    </div>
  );
}
