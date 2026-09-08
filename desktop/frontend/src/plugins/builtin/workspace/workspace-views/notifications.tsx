import * as stylex from "@stylexjs/stylex";
import { EmptyState, IconButton, StatusDot } from "@/ui";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { useNotificationStore } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./views/viewStyles";
import {
  notificationDotTone,
  notificationsSubtext,
  notificationsViewModel,
} from "@/plugins/builtin/workspace/application/notificationsViewModel";

export function NotificationsTab() {
  const t = useT();
  const log = useNotificationStore((s) => s.log);
  const dismiss = useNotificationStore((s) => s.dismiss);
  const clearAll = useNotificationStore((s) => s.clearAll);
  const view = notificationsViewModel(log);

  return (
    <WorkspaceViewLayout
      icon="chat"
      titleStrong
      title="notifications.title"
      sub={notificationsSubtext(t, view)}
      scrollClassName="py-1"
      actions={
        <IconButton icon="x" iconSize="sm" title={t("notifications.clearAll")} onClick={clearAll} />
      }
    >
      {view.isEmpty && (
        <EmptyState
          icon="chat"
          title={t("notifications.empty.title")}
          sub={t("notifications.empty.sub")}
        />
      )}
      {view.entries.map((e) => (
        <NotificationRow
          key={e.id}
          level={e.level}
          message={e.message}
          plugin={e.plugin}
          timestamp={e.timestamp}
          dismissed={e.dismissed}
          onDismiss={() => dismiss(e.id)}
        />
      ))}
    </WorkspaceViewLayout>
  );
}

interface RowProps {
  level: "info" | "warn" | "error";
  message: string;
  plugin: string;
  timestamp: number;
  dismissed?: boolean;
  onDismiss: () => void;
}

function NotificationRow({ level, message, plugin, timestamp, dismissed, onDismiss }: RowProps) {
  const t = useT();
  return (
    <div {...stylex.props(vs.rowTop, vs.gutter, vs.rowPad, dismissed && vs.dismissed)}>
      <StatusDot tone={notificationDotTone(level)} className={stylex.props(vs.dotTop).className} />
      <div {...stylex.props(vs.fill)}>
        <div {...stylex.props(vs.wrapText, vs.soft, typeStep.uiMd)}>{message}</div>
        <div {...stylex.props(vs.subCaptionMuted, typeStep.uiSm)}>
          {plugin} · {formatRelative(timestamp)}
        </div>
      </div>
      {!dismissed && (
        <IconButton icon="x" iconSize="xs" title={t("notifications.dismiss")} onClick={onDismiss} />
      )}
    </div>
  );
}
