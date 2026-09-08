import * as stylex from "@stylexjs/stylex";
import type { Tone } from "@/lib/tone";
import type { IconName } from "@/ui";
import type { TimelineEntry, TimelineEntryKind } from "@/plugins/sdk/types/agentSessionView";
import { Badge, EmptyState, Icon, IconButton } from "@/ui";
import { useT, type Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { toolIntent } from "@/plugins/builtin/agent/public/messagePresentation";
import { useActiveSessionToolCalls } from "@/plugins/builtin/agent/public/run";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { type as typeStep } from "@/styles/tokens.stylex";
import { indent, inkByTone, timelineStyles as ts, viewStyles as vs } from "./views/viewStyles";
import {
  cancelSessionRun,
  useActiveSessionRunTree,
  useActiveSessionTimeline,
} from "@/plugins/builtin/agent/public/run";
import {
  locateWorkspaceTool,
  selectWorkspaceChat,
} from "@/plugins/builtin/workspace/public/navigation";
import {
  timelineGroupKey,
  type TimelineRunGroup,
  timelineRunStatusView,
  timelineSubtext,
  timelineTimeOfDay,
  timelineViewModel,
} from "@/plugins/builtin/workspace/application/timelineViewModel";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";

const KIND_ICON: Record<TimelineEntryKind, IconName> = {
  "run-start": "play",
  "run-end": "check",
  "run-error": "bug",
  "tool-start": "tool",
  "tool-end": "tool",
  "approval-request": "shield",
  "approval-result": "shield",
};

const KIND_I18N: Record<TimelineEntryKind, string> = {
  "run-start": "timeline.kind.runStart",
  "run-end": "timeline.kind.runEnd",
  "run-error": "timeline.kind.runError",
  "tool-start": "timeline.kind.toolStart",
  "tool-end": "timeline.kind.toolEnd",
  "approval-request": "timeline.kind.approvalRequest",
  // Settled, not "Approval". Every other row is a statement — "Run finished", "Tool
  // finished" — and the verdict rides the status mark beside it, which is how `tool-end`
  // already reports a failure. As a bare noun it also lied in four languages: 核准, 承認,
  // 승인 and Aprobación all mean GRANTED, so a denial was filed under approved.
  "approval-result": "timeline.kind.approvalResult",
};

// A glyph, not a dot. `tool-start` and `tool-end` carry the same kind icon and the same label,
// so a succeeded call and a failed one used to differ by one 6px circle being green instead of
// red — the pair colour vision fails on, with nothing else in the row to read instead. The kind
// mark on the left already speaks in glyphs; this answers in the same vocabulary, and `ok` beside
// `approved` stays legible because their kind marks differ.
// The mark says the same thing the badge beside a run does, so it speaks the same `Tone`
// rather than carrying an ink of its own.
const STATUS_MARK: Record<NonNullable<TimelineEntry["status"]>, { icon: IconName; tone: Tone }> = {
  ok: { icon: "check", tone: "success" },
  err: { icon: "alert", tone: "negative" },
  approved: { icon: "check", tone: "success" },
  declined: { icon: "x", tone: "warning" },
};

/**
 * What the row is ABOUT.
 *
 * The fold puts the identifying argument here and falls back to the tool's wire NAME when it
 * finds none — deliberately, because a translated string in view state would freeze a
 * language into it. The transcript resolves that fallback through `toolIntent`; this surface
 * printed it, so a run's audit trail read `ask_user`, `read_tool_result` and `sh_01` beside
 * rows that said "Verify package dependencies".
 */
function entrySubject(t: Translate, entry: TimelineEntry, tool: ToolCall | undefined): string {
  if (!tool || entry.summary === undefined || entry.summary !== tool.name)
    return entry.summary ?? "";
  const intent = toolIntent(t, tool);
  return intent.detail?.value ?? intent.label.value;
}

function TimelineRow({ entry, tool }: { entry: TimelineEntry; tool: ToolCall | undefined }) {
  const t = useT();
  const icon = KIND_ICON[entry.kind];
  const subject = entrySubject(t, entry, tool);
  return (
    <div {...stylex.props(vs.rowTop, vs.gutter, vs.groupPad)}>
      <Icon name={icon} size="xs" className={stylex.props(ts.glyph).className} />
      <div {...stylex.props(vs.fill)}>
        <div {...stylex.props(vs.lineBaseline)}>
          <span {...stylex.props(vs.hold, ts.kind, typeStep.uiSm)}>{t(KIND_I18N[entry.kind])}</span>
          {subject && (
            // Named because it is the one place a tool reaches the timeline by name, and a
            // closure test checks that the name is the transcript's rather than the wire's.
            <span
              data-timeline-subject=""
              title={subject}
              {...stylex.props(vs.truncate, vs.mono, vs.muted, typeStep.uiSm)}
            >
              {subject}
            </span>
          )}
        </div>
      </div>
      {entry.status && (
        // `Icon` is `aria-hidden` by design and takes no name, so the label lives on a wrapper
        // that claims the role. Passing `aria-label` to the component compiles — TypeScript does
        // not check hyphenated JSX attributes against a component's props — and is dropped.
        <span
          role="img"
          aria-label={entry.status}
          {...stylex.props(ts.mark, inkByTone[STATUS_MARK[entry.status].tone])}
        >
          <Icon name={STATUS_MARK[entry.status].icon} size="xs" />
        </span>
      )}
      <span {...stylex.props(ts.stamp, typeStep.uiXs)}>{timelineTimeOfDay(entry.ts)}</span>
    </div>
  );
}

function TimelineRunHeader({
  group,
  runtimeAvailable,
}: {
  group: TimelineRunGroup;
  runtimeAvailable: boolean;
}) {
  const t = useT();
  const run = group.run;
  if (!run) {
    return group.runId ? (
      <div {...stylex.props(vs.gutter, vs.sectionPad, vs.mono, vs.caption, typeStep.uiXs)}>
        {t("timeline.unknownRun", { id: group.runId })}
      </div>
    ) : null;
  }

  const status = timelineRunStatusView(run);
  const parentRunId = run.parentRunId;
  const spawnedByItemId = run.spawnedByItemId;
  const child = parentRunId !== null;
  return (
    <div {...stylex.props(ts.runHeader)}>
      <Icon
        name={child ? "bot" : "branch"}
        size="sm"
        className={stylex.props(vs.hold, vs.muted).className}
      />
      <div {...stylex.props(vs.fill, vs.rowPad)}>
        <div {...stylex.props(vs.line)}>
          <span {...stylex.props(vs.hold, vs.title, typeStep.uiSm)}>
            {t(child ? "timeline.delegatedRun" : "timeline.rootRun")}
          </span>
          <span title={run.id} {...stylex.props(vs.truncate, vs.mono, vs.caption, typeStep.uiXs)}>
            {run.id}
          </span>
          <Badge tone={status.tone}>{t(status.labelKey)}</Badge>
        </div>
        <div {...stylex.props(ts.runDetail, typeStep.uiXs)}>
          {status.detail && (
            <span title={status.detail} {...stylex.props(vs.truncate, ts.pretty)}>
              {status.detail}
            </span>
          )}
          {child && (
            <span title={parentRunId} {...stylex.props(vs.truncate, vs.mono, vs.caption)}>
              {t("timeline.parentRun", { id: parentRunId })}
            </span>
          )}
          <span {...stylex.props(vs.pushEnd, vs.hold, vs.mono)}>
            {t("agent.steps", { count: status.stepCount })}
          </span>
        </div>
      </div>
      {spawnedByItemId && (
        <IconButton
          icon="chat"
          size="lg"
          quiet
          disabled={!runtimeAvailable}
          title={t("timeline.locateParent")}
          onClick={() => locateWorkspaceTool(spawnedByItemId)}
        />
      )}
      {status.cancelable && (
        <IconButton
          icon="stop"
          size="lg"
          quiet
          disabled={!runtimeAvailable}
          title={t("agent.runTree.action.cancel")}
          onClick={() => {
            cancelSessionRun({ sessionId: run.sessionId, runId: run.id });
          }}
        />
      )}
    </div>
  );
}

export function TimelineTab() {
  const t = useT();
  const timeline = useActiveSessionTimeline();
  const runTree = useActiveSessionRunTree();
  const toolCalls = useActiveSessionToolCalls();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const view = timelineViewModel(timeline, runTree);

  return (
    <WorkspaceViewLayout
      icon="history"
      titleStrong
      title="timeline.title"
      sub={timelineSubtext(t, view)}
      scrollClassName="py-1"
      actions={
        <IconButton
          icon="chat"
          iconSize="sm"
          title={t("timeline.jumpToChat")}
          onClick={selectWorkspaceChat}
        />
      }
    >
      {view.groups.length === 0 ? (
        <EmptyState
          icon="history"
          title={t("timeline.empty.title")}
          sub={t("timeline.empty.sub")}
        />
      ) : (
        view.groups.map((group, index) => (
          <div
            key={timelineGroupKey(group)}
            {...stylex.props(
              index > 0 && ts.groupGap,
              indent[Math.min(group.depth, indent.length - 1)],
              group.depth > 0 && ts.nested,
            )}
          >
            <TimelineRunHeader group={group} runtimeAvailable={runtimeAvailable} />
            {group.items.length > 0 ? (
              group.items.map((entry) => (
                <TimelineRow
                  key={entry.id}
                  entry={entry}
                  tool={entry.refId === undefined ? undefined : toolCalls[entry.refId]}
                />
              ))
            ) : (
              <p {...stylex.props(vs.gutter, vs.rowPad, ts.pretty, vs.caption, typeStep.uiXs)}>
                {t("timeline.noEvents")}
              </p>
            )}
          </div>
        ))
      )}
    </WorkspaceViewLayout>
  );
}
