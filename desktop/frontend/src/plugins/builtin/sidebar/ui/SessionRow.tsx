import { useState } from "react";
import { AgentRow } from "@/ui/agent";
import { ConfirmDialog, ContextMenu, Icon, TextField } from "@/ui";
import { useT } from "@/lib/i18n";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { cn } from "@/lib/classNames";
import type { WorkSession } from "@/plugins/builtin/navigation/public/workIndex";

interface Props {
  session: WorkSession;
  active: boolean;
  indented?: boolean;
  showTime?: boolean;
  onSelect: (id: string) => void;
  onRename?: (id: string, expectedRevision: number, title: string) => void;
  onFork?: (id: string) => void;
  onDelete?: (id: string) => void;
  onToggleFavorite?: (id: string, expectedRevision: number, favorite: boolean) => void;
}

/**
 * The row's title while it is being renamed. Enter and blur are the same intention, so they
 * share one `commit` — written twice, the two could drift, and a rename that depends on which
 * key ended it is not a thing anyone asked for.
 */
function SessionTitleField({
  title,
  onCommit,
  onSettle,
}: {
  title: string;
  onCommit: (next: string) => void;
  onSettle: () => void;
}) {
  const t = useT();
  const commit = (value: string) => {
    const next = value.trim();
    if (next && next !== title) onCommit(next);
    onSettle();
  };

  return (
    <TextField
      variant="bare"
      font="sans"
      defaultValue={title}
      aria-label={t("session.row.titleLabel")}
      // oxlint-disable-next-line jsx-a11y/no-autofocus
      autoFocus
      onClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => {
        if (e.nativeEvent.isComposing) return;
        e.stopPropagation();
        if (e.key === "Escape") onSettle();
        if (e.key === "Enter") commit(e.currentTarget.value);
      }}
      onBlur={(e) => commit(e.currentTarget.value)}
      className="flex-1 rounded-xs bg-surface-3 px-1 leading-body"
    />
  );
}

export function SessionRow({
  session,
  active,
  indented = false,
  showTime = true,
  onSelect,
  onRename,
  onFork,
  onDelete,
  onToggleFavorite,
}: Props) {
  const [renaming, setRenaming] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const t = useT();
  const attentionLabel =
    session.attention === "running"
      ? t("session.status.running")
      : session.attention === "waiting"
        ? t("session.status.waiting")
        : undefined;
  const when = formatRelative(session.time);
  const accessibleStatus = attentionLabel ? `${attentionLabel} · ${when}` : when;
  const title = session.title.trim() || t("session.untitled");

  const row = (
    <div className="relative select-none">
      <AgentRow
        onClick={() => onSelect(session.id)}
        data-chrome-focus=""
        aria-current={active ? "page" : undefined}
        aria-label={`${title} — ${accessibleStatus}`}
        active={active}
        indent={indented ? "nested" : "none"}
        revealOverflow={!renaming}
        className="font-normal text-fg-muted hover:text-fg data-[active]:text-fg"
        trailing={
          renaming ? undefined : (
            <span className="flex shrink-0 items-center gap-1.5">
              {session.favorite && <Icon name="star" size="xs" className="text-accent" />}
              {session.attention !== "none" ? (
                <span
                  className={cn(
                    "h-1.5 w-1.5 shrink-0 rounded-full",
                    session.attention === "running" ? "bg-accent animate-pulse-dot" : "bg-warning",
                  )}
                  title={accessibleStatus}
                />
              ) : (
                showTime && (
                  <span className="text-ui-2xs leading-none text-fg-faint tabular-nums">
                    {when}
                  </span>
                )
              )}
            </span>
          )
        }
      >
        {renaming ? (
          <SessionTitleField
            title={title}
            onCommit={(next) => onRename?.(session.id, session.revision, next)}
            onSettle={() => setRenaming(false)}
          />
        ) : (
          title
        )}
      </AgentRow>
    </div>
  );

  if (!onDelete && !onFork && !onRename && !onToggleFavorite) return row;
  return (
    <>
      <ContextMenu.Root>
        <ContextMenu.Trigger render={row} />
        <ContextMenu.Content className="min-w-[160px]">
          {onToggleFavorite && (
            <ContextMenu.IconItem
              icon="star"
              onSelect={() => onToggleFavorite(session.id, session.revision, !session.favorite)}
            >
              {session.favorite ? t("session.action.unpin") : t("session.action.pin")}
            </ContextMenu.IconItem>
          )}
          {onRename && (
            <ContextMenu.IconItem icon="edit" onSelect={() => setRenaming(true)}>
              {t("session.action.rename")}
            </ContextMenu.IconItem>
          )}
          {onFork && (
            <ContextMenu.IconItem icon="branch" onSelect={() => onFork(session.id)}>
              {t("session.action.fork")}
            </ContextMenu.IconItem>
          )}
          {onDelete && (
            <ContextMenu.IconItem
              icon="trash"
              destructive
              onSelect={() => setConfirmingDelete(true)}
            >
              {t("session.action.delete")}
            </ContextMenu.IconItem>
          )}
        </ContextMenu.Content>
      </ContextMenu.Root>
      {onDelete && (
        <ConfirmDialog
          open={confirmingDelete}
          onOpenChange={setConfirmingDelete}
          title={t("session.delete.title")}
          body={t("session.delete.body", { title })}
          confirmLabel={t("session.action.delete")}
          cancelLabel={t("common.cancel")}
          destructive
          onConfirm={() => onDelete(session.id)}
        />
      )}
    </>
  );
}
