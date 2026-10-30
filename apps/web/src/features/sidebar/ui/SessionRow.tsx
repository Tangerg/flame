import { type CapturedSession } from "@flame/component-sdk";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { SESSION_ACTION } from "@/features/navigation/public/sessionActions";
import { type SessionActionSpec } from "@/features/navigation/public/sessionActions";
import { SessionActionDialog } from "@/features/sidebar/ui/SessionActionDialog";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { useEffect, useId, useRef, useState } from "react";
import { failureText } from "@/lib/rpcErrors";
import { AgentRow, AgentRowEditor } from "@/ui/agent/index";
import { ConfirmDialog } from "@/ui/index";
import { ContextMenu, DropdownMenu } from "@/ui/index";
import { Icon } from "@/ui/index";
import { StatusDot } from "@/ui/atoms/status-dot";
import { IconButton, isImeKey, TextField, vocab } from "@flame/ui";
import { type IconName } from "@/ui/icons/index";
import { useT } from "@/lib/i18n/index";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { type WorkSession } from "@/features/navigation/public/workIndex";
import {
  sessionIsRunning,
  sessionIsWaiting,
  sessionNeedsAttention,
} from "@/features/navigation/public/workIndex";
import { color, space, type as typeStep } from "@flame/ui/tokens.stylex";
const sr = stylex.create({
  host: { position: "relative", userSelect: "none" },
  trailing: { display: "flex", flexShrink: 0, alignItems: "center", gap: space.s1_5 },
  favorite: { color: color.accentText },
  titleEdit: { display: "flex", flexDirection: "column", gap: space.s1, flex: 1, minWidth: 0 },
  editError: { color: color.negative },
  stamp: { lineHeight: 1, color: color.fgFaint, fontVariantNumeric: "tabular-nums" },
});

interface Props {
  session: WorkSession;
  active: boolean;
  indented?: boolean;
  showTime?: boolean;
  onSelect: (id: string) => void;
  onRename?: (id: string, expectedRevision: number, title: string) => Promise<boolean>;
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
  onCommit: (next: string) => Promise<boolean>;
  onSettle: (restoreFocus: boolean) => void;
}) {
  const t = useT();
  const inputRef = useRef<HTMLInputElement>(null);
  const errorId = useId();
  const mounted = useRef(true);
  const admission = useRef<"open" | "failed" | "closed" | Promise<boolean>>("open");
  const [feedback, setFeedback] = useState<
    { state: "idle" | "busy" } | { state: "error"; message: string }
  >({ state: "idle" });
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const close = (restoreFocus: boolean) => {
    admission.current = "closed";
    onSettle(restoreFocus && document.activeElement === inputRef.current);
  };
  const settle = (trigger: "enter" | "blur" | "cancel", value?: string) => {
    if (!mounted.current) return;
    // A trailing blur observes a refused Enter attempt instead of retrying it.
    if (admission.current !== "open" && !(admission.current === "failed" && trigger !== "blur"))
      return;
    const restoreFocus = trigger !== "blur";
    const next = value?.trim();
    if (!next || next === title) {
      close(restoreFocus);
      return;
    }
    const { promise: pending, resolve, reject } = Promise.withResolvers<boolean>();
    admission.current = pending;
    setFeedback({ state: "busy" });
    try {
      resolve(onCommit(next));
    } catch (error) {
      reject(error);
    }
    const fail = (error?: unknown) => {
      if (!mounted.current || admission.current !== pending) return;
      admission.current = "failed";
      setFeedback({ state: "error", message: failureText(error, t("session.error.rename")) });
    };
    void pending.then((saved) => {
      if (!mounted.current || admission.current !== pending) return;
      if (saved) close(restoreFocus);
      else fail();
    }, fail);
  };

  return (
    <div {...stylex.props(sr.titleEdit)}>
      <TextField
        ref={inputRef}
        variant="inline"
        defaultValue={title}
        pending={feedback.state === "busy"}
        aria-busy={feedback.state === "busy"}
        invalid={feedback.state === "error"}
        aria-describedby={feedback.state === "error" ? errorId : undefined}
        placeholder={t("session.untitled")}
        aria-label={t("session.row.titleLabel")}
        // oxlint-disable-next-line jsx-a11y/no-autofocus
        autoFocus
        onClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          if (isImeKey(e.nativeEvent)) return;
          e.stopPropagation();
          if (e.key === "Escape" || e.key === "Enter") {
            e.preventDefault();
            settle(
              e.key === "Enter" ? "enter" : "cancel",
              e.key === "Enter" ? e.currentTarget.value : undefined,
            );
          }
        }}
        onBlur={(e) => settle("blur", e.currentTarget.value)}
        className={stylex.props(vocab.grow).className}
      />
      {feedback.state === "error" && (
        <span id={errorId} role="alert" {...stylex.props(sr.editError, typeStep.uiXs)}>
          {feedback.message}
        </span>
      )}
    </div>
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
  const [editTarget, setEditTarget] = useState<{
    target: CapturedSession;
    rename: NonNullable<Props["onRename"]>;
  } | null>(null);
  const capturedSession = Object.freeze({
    id: session.id,
    revision: session.revision,
    title: session.title,
  });
  const contributedActions = useExtensionPoint(SESSION_ACTION);
  const [action, setAction] = useState<{ entry: SessionActionSpec; session: CapturedSession }>();
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const t = useT();
  const attentionLabel = sessionIsRunning(session)
    ? t("session.status.running")
    : sessionIsWaiting(session)
      ? t("session.status.waiting")
      : undefined;
  const when = formatRelative(session.time);
  const accessibleStatus = attentionLabel ? `${attentionLabel} · ${when}` : when;
  const title = session.title.trim() || t("session.untitled");

  const menu: {
    id: string;
    icon: IconName;
    label: string;
    destructive?: boolean;
    run: () => void;
  }[] = [
    ...(onToggleFavorite
      ? [
          {
            id: "pin",
            icon: "star" as const,
            label: session.favorite ? t("session.action.unpin") : t("session.action.pin"),
            run: () => onToggleFavorite(session.id, session.revision, !session.favorite),
          },
        ]
      : []),
    ...(onRename
      ? [
          {
            id: "rename",
            icon: "edit" as const,
            label: t("session.action.rename"),
            run: () => setEditTarget({ target: capturedSession, rename: onRename }),
          },
        ]
      : []),
    ...(onFork
      ? [
          {
            id: "fork",
            icon: "fork" as const,
            label: t("session.action.fork"),
            run: () => onFork(session.id),
          },
        ]
      : []),
    ...contributedActions.map((entry) => ({
      id: entry.id,
      icon: "blocks" as const,
      label: entry.title,
      run: () =>
        setAction({
          entry,
          session: capturedSession,
        }),
    })),
    ...(onDelete
      ? [
          {
            id: "delete",
            icon: "trash" as const,
            label: t("session.action.delete"),
            destructive: true,
            run: () => setConfirmingDelete(true),
          },
        ]
      : []),
  ];

  const more =
    menu.length > 0 ? (
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <IconButton
              icon={iconGlyph("more")}
              size="sm"
              data-chrome-focus=""
              aria-label={t("session.row.more", { title })}
            />
          }
        />
        <DropdownMenu.Content align="start" sideOffset={4}>
          {menu.map((item) => (
            <DropdownMenu.Item
              key={item.id}
              layout="glyph"
              onClick={item.run}
              destructive={item.destructive}
            >
              <Icon name={item.icon} size="md" />
              <span {...stylex.props(vocab.truncate)}>{item.label}</span>
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    ) : undefined;

  const row = (
    <div {...stylex.props(sr.host)}>
      {editTarget ? (
        <AgentRowEditor indent={indented ? "nested" : "none"}>
          <SessionTitleField
            title={editTarget.target.title.trim()}
            onCommit={(next) =>
              editTarget.rename(editTarget.target.id, editTarget.target.revision, next)
            }
            onSettle={(restoreFocus) => {
              setEditTarget(null);
              if (restoreFocus) requestAnimationFrame(() => rowRef.current?.focus());
            }}
          />
        </AgentRowEditor>
      ) : (
        <AgentRow
          ref={rowRef}
          onClick={() => onSelect(session.id)}
          aria-current={active ? "page" : undefined}
          aria-label={[
            title,
            session.favorite ? t("session.status.pinned") : undefined,
            accessibleStatus,
          ]
            .filter((part) => part !== undefined)
            .join(" — ")}
          active={active}
          indent={indented ? "nested" : "none"}
          revealOverflow
          look="quiet"
          action={more}
          trailing={
            <span {...stylex.props(sr.trailing)}>
              {session.favorite && (
                <Icon name="star" size="xs" className={stylex.props(sr.favorite).className} />
              )}
              {sessionNeedsAttention(session) ? (
                <StatusDot
                  tone={sessionIsRunning(session) ? "running" : "waiting"}
                  ring={sessionIsRunning(session)}
                  label={accessibleStatus}
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

  if (menu.length === 0) return row;
  return (
    <>
      <ContextMenu.Root>
        <ContextMenu.Trigger render={row} />
        <ContextMenu.Content aria-label={title}>
          {menu.map((item) => (
            <ContextMenu.IconItem
              key={item.id}
              icon={item.icon}
              destructive={item.destructive}
              onSelect={item.run}
            >
              {item.label}
            </ContextMenu.IconItem>
          ))}
        </ContextMenu.Content>
      </ContextMenu.Root>
      {action && (
        <SessionActionDialog
          action={action.entry}
          session={action.session}
          close={() => {
            setAction(undefined);
            requestAnimationFrame(() => rowRef.current?.focus());
          }}
        />
      )}
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
