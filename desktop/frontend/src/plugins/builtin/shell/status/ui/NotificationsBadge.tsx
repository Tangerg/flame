import * as stylex from "@stylexjs/stylex";
import { EmptyState, IconButton, Popover, SectionLabel, StatusDot, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { space, type as typeStep } from "@/styles/tokens.stylex";
import { useNotificationStore, type NotificationLevel } from "@/plugins/sdk";
import {
  notificationBadgeText,
  unreadNotificationCount,
} from "../application/notificationsReadout";

const dotTone: Record<NotificationLevel, "err" | "waiting" | "idle"> = {
  error: "err",
  warn: "waiting",
  info: "idle",
};

const styles = stylex.create({
  panel: {
    width: "min(360px, calc(100vw - 16px))",
    maxHeight: "min(480px, var(--available-height))",
    display: "flex",
    flexDirection: "column",
  },
  header: { paddingInline: space.s3, paddingBlock: space.s2, flexShrink: 0 },
  list: { overflowY: "auto", overscrollBehavior: "contain", minHeight: 0 },
  row: {
    display: "flex",
    alignItems: "flex-start",
    gap: space.s2,
    paddingInline: space.s3,
    paddingBlock: space.s2,
  },
  dot: { marginTop: space.s1_5, flexShrink: 0 },
});

export function NotificationsBadge() {
  const t = useT();
  const log = useNotificationStore((state) => state.log);
  const dismiss = useNotificationStore((state) => state.dismiss);
  const clearAll = useNotificationStore((state) => state.clearAll);
  const unread = unreadNotificationCount(log);

  return (
    <Popover.Root>
      <Popover.Trigger
        render={
          <IconButton
            icon="bell"
            size="sm"
            quiet
            badge={notificationBadgeText(unread) ?? undefined}
            title={
              unread > 0
                ? t("status.notifications.unread", { count: unread })
                : t("status.notifications")
            }
            aria-label={t("status.notifications")}
          />
        }
      />
      <Popover.Content
        side="top"
        align="start"
        sideOffset={6}
        aria-label={t("notifications.title")}
        className={stylex.props(styles.panel).className}
      >
        <SectionLabel
          className={stylex.props(styles.header).className}
          trailing={
            <IconButton
              icon="x"
              iconSize="sm"
              title={t("notifications.clearAll")}
              disabled={log.length === 0}
              onClick={clearAll}
            />
          }
        >
          {t("notifications.title")}
        </SectionLabel>
        <div {...stylex.props(styles.list)}>
          {log.length === 0 && <EmptyState icon="bell" title={t("notifications.empty.title")} />}
          {log.toReversed().map((entry) => (
            <div key={entry.id} {...stylex.props(styles.row)}>
              <StatusDot
                tone={dotTone[entry.level]}
                className={stylex.props(styles.dot).className}
              />
              <div {...stylex.props(vocab.fill)}>
                <div
                  {...stylex.props(vocab.wrapText, entry.dismissed && vocab.faint, typeStep.uiMd)}
                >
                  {entry.message}
                </div>
                <div {...stylex.props(vocab.faint, typeStep.uiSm)}>
                  {formatRelative(entry.timestamp)}
                </div>
              </div>
              {!entry.dismissed && (
                <IconButton
                  icon="x"
                  iconSize="xs"
                  title={t("notifications.dismiss")}
                  onClick={() => dismiss(entry.id)}
                />
              )}
            </div>
          ))}
        </div>
      </Popover.Content>
    </Popover.Root>
  );
}
