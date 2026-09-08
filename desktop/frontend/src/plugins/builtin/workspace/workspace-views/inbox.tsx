import * as stylex from "@stylexjs/stylex";
import type { PendingWorkItem } from "@/plugins/builtin/agent/public/hitl";
import { usePendingWork } from "@/plugins/builtin/agent/public/hitl";
import { selectAgentSession, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { Badge, DataView, Icon, Pressable } from "@/ui";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n";
import { type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./views/viewStyles";
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
        onRetry={() => void query.refetch()}
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
      className={stylex.props(vs.pressRow, vs.gutter, vs.rowPad, vs.wash).className}
    >
      <Icon
        name={item.kind === "question" ? "question" : "shield"}
        size="sm"
        className={stylex.props(vs.glyphTop, vs.hold, vs.warning).className}
      />
      <div {...stylex.props(vs.fill)}>
        <div {...stylex.props(vs.lineBaseline)}>
          <span {...stylex.props(vs.fill, vs.truncate, typeStep.uiMd)}>{sessionTitle}</span>
          <span {...stylex.props(vs.hold, vs.muted, typeStep.uiSm)}>
            {formatRelative(item.waitingSince)}
          </span>
        </div>
        <div {...stylex.props(vs.subLine)}>
          <span {...stylex.props(vs.muted, typeStep.uiSm)}>{ask}</span>
          {item.subject && (
            <span {...stylex.props(vs.fill, vs.truncate, vs.soft, typeStep.uiSm)}>
              {item.subject}
            </span>
          )}
          {item.more > 0 && <Badge tone="neutral">{`+${item.more}`}</Badge>}
        </div>
      </div>
    </Pressable>
  );
}
