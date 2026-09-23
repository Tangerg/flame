import * as stylex from "@stylexjs/stylex";
import { useRef, useState } from "react";
import { AgentRow, AgentRowEditor } from "@/ui/agent";
import { ConfirmDialog, ContextMenu, Icon, TextField, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { formatRelative } from "@/lib/i18n/relativeTime";
import type { WorkSession } from "@/plugins/builtin/navigation/public/workIndex";
import { color, corner, space, type as typeStep } from "@/styles/tokens.stylex";

const sr = stylex.create({
  host: { position: "relative", userSelect: "none" },
  trailing: { display: "flex", flexShrink: 0, alignItems: "center", gap: space.s1_5 },
  favorite: { color: color.accent },
  mark: { height: space.s1_5, width: space.s1_5, flexShrink: 0 },
  markRunning: {
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: color.accent,
    animation: "var(--animate-pulse-dot)",
  },
  markWaiting: { backgroundColor: color.warning },
  stamp: { lineHeight: 1, color: color.fgFaint, fontVariantNumeric: "tabular-nums" },
});

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

function SessionTitleField({
  title,
  onCommit,
  onSettle,
}: {
  title: string;
  onCommit: (next: string) => void;
  onSettle: (restoreFocus: boolean) => void;
}) {
  const t = useT();
  const settled = useRef(false);
  const settle = (restoreFocus: boolean, value?: string) => {
    if (settled.current) return;
    settled.current = true;
    if (value !== undefined) {
      const next = value.trim();
      if (next && next !== title) onCommit(next);
    }
    onSettle(restoreFocus);
  };

  return (
    <TextField
      variant="inline"
      defaultValue={title}
      aria-label={t("session.row.titleLabel")}
      // oxlint-disable-next-line jsx-a11y/no-autofocus
      autoFocus
      onClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => {
        if (e.nativeEvent.isComposing || e.nativeEvent.keyCode === 229) return;
        e.stopPropagation();
        if (e.key === "Escape" || e.key === "Enter") {
          e.preventDefault();
          settle(true, e.key === "Enter" ? e.currentTarget.value : undefined);
        }
      }}
      onBlur={(e) => settle(false, e.currentTarget.value)}
      className={stylex.props(vocab.grow).className}
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
  const rowRef = useRef<HTMLButtonElement>(null);
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
    <div {...stylex.props(sr.host)}>
      {renaming ? (
        <AgentRowEditor indent={indented ? "nested" : "none"}>
          <SessionTitleField
            title={title}
            onCommit={(next) => onRename?.(session.id, session.revision, next)}
            onSettle={(restoreFocus) => {
              setRenaming(false);
              if (restoreFocus) requestAnimationFrame(() => rowRef.current?.focus());
            }}
          />
        </AgentRowEditor>
      ) : (
        <AgentRow
          ref={rowRef}
          onClick={() => onSelect(session.id)}
          data-chrome-focus=""
          aria-current={active ? "page" : undefined}
          aria-label={`${title} — ${accessibleStatus}`}
          active={active}
          indent={indented ? "nested" : "none"}
          revealOverflow
          look="quiet"
          trailing={
            <span {...stylex.props(sr.trailing)}>
              {session.favorite && (
                <Icon name="star" size="xs" className={stylex.props(sr.favorite).className} />
              )}
              {session.attention !== "none" ? (
                <span
                  {...stylex.props(
                    sr.mark,
                    corner.pill,
                    session.attention === "running" ? sr.markRunning : sr.markWaiting,
                  )}
                  title={accessibleStatus}
                />
              ) : (
                showTime && <span {...stylex.props(sr.stamp, typeStep.uiXs)}>{when}</span>
              )}
            </span>
          }
        >
          {title}
        </AgentRow>
      )}
    </div>
  );

  if (!onDelete && !onFork && !onRename && !onToggleFavorite) return row;
  return (
    <>
      <ContextMenu.Root>
        <ContextMenu.Trigger render={row} />
        <ContextMenu.Content>
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
