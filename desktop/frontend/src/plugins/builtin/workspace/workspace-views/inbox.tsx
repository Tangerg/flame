import type { PendingWorkItem } from "@/plugins/builtin/agent/public/hitl";
import { usePendingWork } from "@/plugins/builtin/agent/public/hitl";
import { selectAgentSession, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { Badge, DataView, Icon, Pressable } from "@/ui";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";

export function InboxTab() {
  const t = useT();
  const query = usePendingWork();
  const sessions = useAgentSessions();
  const items = query.data ?? [];
  const titleOf = (sessionId: string) =>
    sessions.data?.find((session) => session.id === sessionId)?.title ?? sessionId;

  return (
    <WorkspaceViewLayout
      icon="bell"
      titleStrong
      title="inbox.title"
      sub={items.length > 0 ? t("inbox.waiting", { count: items.length }) : undefined}
    >
      <DataView
        items={items}
        isLoading={query.isLoading}
        isError={query.isError}
        skeletonVariant="stacked"
        empty={{ icon: "bell", title: t("inbox.empty.title"), sub: t("inbox.empty.sub") }}
      >
        {(pending) =>
          pending.map((item) => (
            <PendingRow
              key={item.id}
              item={item}
              sessionTitle={titleOf(item.sessionId)}
              onOpen={() => selectAgentSession(item.sessionId)}
            />
          ))
        }
      </DataView>
    </WorkspaceViewLayout>
  );
}

function PendingRow({
  item,
  sessionTitle,
  onOpen,
}: {
  item: PendingWorkItem;
  sessionTitle: string;
  onOpen: () => void;
}) {
  const t = useT();
  const ask = item.kind === "question" ? t("inbox.ask.question") : t("inbox.ask.approval");

  return (
    <Pressable
      type="button"
      data-chrome-focus=""
      onClick={onOpen}
      className="flex w-full min-w-0 items-start gap-2.5 px-[var(--density-column-gutter-wide)] py-2 text-left hover:bg-hover"
    >
      <Icon
        name={item.kind === "question" ? "question" : "shield"}
        size="sm"
        className="mt-0.5 shrink-0 text-warning"
      />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-baseline gap-2">
          <span className="min-w-0 flex-1 truncate text-ui-md text-fg">{sessionTitle}</span>
          <span className="shrink-0 text-ui-sm text-fg-muted">
            {formatRelative(item.waitingSince)}
          </span>
        </div>
        <div className="mt-0.5 flex min-w-0 items-center gap-1.5">
          <span className="text-ui-sm text-fg-muted">{ask}</span>
          {item.subject && (
            <span className="min-w-0 flex-1 truncate text-ui-sm text-fg-soft">{item.subject}</span>
          )}
          {item.more > 0 && <Badge tone="neutral">{`+${item.more}`}</Badge>}
        </div>
      </div>
    </Pressable>
  );
}
