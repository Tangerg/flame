import { type CapturedSession } from "@flame/component-sdk";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { SESSION_ACTION } from "@/features/navigation/public/sessionActions";
import { type SessionActionSpec } from "@/features/navigation/public/sessionActions";
import { SessionActionDialog } from "@/features/sidebar/ui/SessionActionDialog";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { useRef, useState } from "react";
import { AgentRow } from "@/ui/agent/index";
import { ConfirmDialog } from "@/ui/index";
import { ContextMenu, DropdownMenu } from "@/ui/index";
import { Icon } from "@/ui/index";
import { StatusDot } from "@/ui/atoms/status-dot";
import { IconButton, vocab } from "@flame/ui";
import { type IconName } from "@/ui/icons/index";
import { presentationTitle } from "@/lib/i18n/presentationTitle";
import { useLocale, useT, type Translate } from "@/lib/i18n/index";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { type WorkSession } from "@/features/navigation/public/workIndex";
import { sessionDisplayTitle } from "@/features/agent/public/session";
import { MoveToProjectDialog } from "@/features/projects/public/projects";
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
  stamp: { lineHeight: 1, color: color.fgFaint, fontVariantNumeric: "tabular-nums" },
});

interface Props {
  session: WorkSession;
  active: boolean;
  indented?: boolean;
  showTime?: boolean;
  onSelect: (id: string) => void;
  onFork?: (id: string) => void;
  onDelete?: (id: string) => void;
  onToggleFavorite?: (id: string, expectedRevision: number, favorite: boolean) => void;
}

interface SessionMenuItem {
  id: string;
  icon: IconName;
  label: string;
  destructive?: boolean;
  run: () => void;
}

function sessionAttention(t: Translate, session: WorkSession): string | undefined {
  if (sessionIsRunning(session)) return t("session.status.running");
  if (sessionIsWaiting(session)) return t("session.status.waiting");
  return undefined;
}

// Pinning, forking and deleting appear only where the owner supports them;
// moving to a Project is always available, and installed actions follow it.
function sessionMenu(
  t: Translate,
  session: WorkSession,
  actions: Pick<Props, "onToggleFavorite" | "onFork" | "onDelete"> & {
    contributed: SessionMenuItem[];
    move: () => void;
    confirmDelete: () => void;
  },
): SessionMenuItem[] {
  const { onToggleFavorite, onFork, onDelete } = actions;
  const items: SessionMenuItem[] = [];
  if (onToggleFavorite)
    items.push({
      id: "pin",
      icon: "star",
      label: session.favorite ? t("session.action.unpin") : t("session.action.pin"),
      run: () => onToggleFavorite(session.id, session.revision, !session.favorite),
    });
  if (onFork)
    items.push({
      id: "fork",
      icon: "fork",
      label: t("session.action.fork"),
      run: () => onFork(session.id),
    });
  items.push(
    {
      id: "project",
      icon: "blocks",
      label: t("session.action.moveToProject"),
      run: actions.move,
    },
    ...actions.contributed,
  );
  if (onDelete)
    items.push({
      id: "delete",
      icon: "trash",
      label: t("session.action.delete"),
      destructive: true,
      run: actions.confirmDelete,
    });
  return items;
}

function SessionRowTrailing({
  session,
  showTime,
  when,
  status,
}: {
  session: WorkSession;
  showTime: boolean;
  when: string;
  status: string;
}) {
  const running = sessionIsRunning(session);
  return (
    <span {...stylex.props(sr.trailing)}>
      {session.favorite && (
        <Icon name="star" size="xs" className={stylex.props(sr.favorite).className} />
      )}
      {sessionNeedsAttention(session) ? (
        <StatusDot tone={running ? "running" : "waiting"} ring={running} label={status} />
      ) : (
        showTime && <span {...stylex.props(sr.stamp, typeStep.uiXs)}>{when}</span>
      )}
    </span>
  );
}

export function SessionRow({
  session,
  active,
  indented = false,
  showTime = true,
  onSelect,
  onFork,
  onDelete,
  onToggleFavorite,
}: Props) {
  const rowRef = useRef<HTMLButtonElement>(null);
  const capturedSession = Object.freeze({
    id: session.id,
    revision: session.revision,
    title: session.title,
  });
  const contributedActions = useExtensionPoint(SESSION_ACTION);
  const [action, setAction] = useState<{ entry: SessionActionSpec; session: CapturedSession }>();
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [moving, setMoving] = useState(false);
  const t = useT();
  const locale = useLocale();
  const attentionLabel = sessionAttention(t, session);
  const when = formatRelative(session.time);
  const accessibleStatus = attentionLabel ? `${attentionLabel} · ${when}` : when;
  const title = sessionDisplayTitle(t, session.title);

  const menu = sessionMenu(t, session, {
    onToggleFavorite,
    onFork,
    onDelete,
    contributed: contributedActions.map((entry) => ({
      id: entry.id,
      icon: "plug" as const,
      label: presentationTitle(entry.title, entry.localizedTitles, locale),
      run: () =>
        setAction({
          entry,
          session: capturedSession,
        }),
    })),
    move: () => setMoving(true),
    confirmDelete: () => setConfirmingDelete(true),
  });

  const more = (
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
  );

  const row = (
    <div {...stylex.props(sr.host)}>
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
          <SessionRowTrailing
            session={session}
            showTime={showTime}
            when={when}
            status={accessibleStatus}
          />
        }
      >
        {title}
      </AgentRow>
    </div>
  );

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
          finalFocus={rowRef}
          close={() => setAction(undefined)}
        />
      )}
      {moving && (
        <MoveToProjectDialog
          session={session}
          finalFocus={rowRef}
          onClose={() => setMoving(false)}
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
