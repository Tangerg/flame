import { useState } from "react";
import { ModelInvocationHistory } from "./views/ModelInvocationHistory";
import * as stylex from "@stylexjs/stylex";
import type { Tone } from "@/lib/tone";
import type { IconName } from "@/ui";
import type { TimelineEntry, TimelineEntryKind } from "@/plugins/sdk/types/agentSessionView";
import { Badge, EmptyState, Icon, IconButton, toneInk, vocab } from "@/ui";
import { ToolText } from "@/ui/agent";
import { useT, type Translate } from "@/lib/i18n";
import { formatClock, formatDateTime } from "@/lib/i18n/relativeTime";
import { fmtDuration } from "@/lib/format";
import { TIMELINE_WINDOW_SIZE } from "@/plugins/sdk/types/agentTimeline";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { toolIntent, type ToolDetail } from "@/plugins/builtin/agent/public/messagePresentation";
import { useActiveSessionToolCalls } from "@/plugins/builtin/agent/public/run";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { face, type as typeStep } from "@/styles/tokens.stylex";
import { indent, timelineStyles as ts, viewStyles as vs } from "./views/viewStyles";
import {
  cancelSessionRun,
  useActiveSessionRunTree,
  useActiveSessionTimeline,
} from "@/plugins/builtin/agent/public/run";
import {
  locateWorkspaceTool,
  openWorkspaceSubagentRun,
} from "@/plugins/builtin/workspace/public/navigation";
import {
  timelineGroupKey,
  type TimelineRunGroup,
  timelineRunStatusView,
  timelineSubtext,
  timelineViewModel,
} from "@/plugins/builtin/workspace/application/timelineViewModel";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";

const KIND_ICON: Record<TimelineEntryKind, IconName> = {
  "run-start": "play",
  "run-end": "check",
  "run-error": "bug",
  tool: "tool",
  "approval-request": "shield",
  "approval-result": "shield",
  compaction: "history",
};

const KIND_I18N: Record<TimelineEntryKind, string> = {
  "run-start": "timeline.kind.runStart",
  "run-end": "timeline.kind.runEnd",
  "run-error": "timeline.kind.runError",
  tool: "timeline.kind.toolStart",
  "approval-request": "timeline.kind.approvalRequest",
  // Settled, not "Approval". Every other row is a statement — "Run finished", "Tool
  // finished" — and the verdict rides the status mark beside it, which is how a completed tool
  // already reports a failure. As a bare noun it also lied in four languages: 核准, 承認,
  // 승인 and Aprobación all mean GRANTED, so a denial was filed under approved.
  "approval-result": "timeline.kind.approvalResult",
  compaction: "timeline.kind.compaction",
};

// Status glyphs keep outcomes distinguishable without relying on color.
const STATUS_MARK: Record<NonNullable<TimelineEntry["status"]>, { icon: IconName; tone: Tone }> = {
  ok: { icon: "check", tone: "success" },
  err: { icon: "alert", tone: "negative" },
  approved: { icon: "check", tone: "success" },
  declined: { icon: "x", tone: "warning" },
};

function entrySubject(t: Translate, entry: TimelineEntry, tool: ToolCall | undefined): ToolDetail {
  if (!tool) return { kind: "prose", value: entry.summary ?? "" };
  if (entry.kind !== "tool" && entry.summary !== tool.name) {
    return { kind: "prose", value: entry.summary ?? "" };
  }
  const intent = toolIntent(t, tool);
  return intent.detail ?? intent.label;
}

function TimelineRow({ entry, tool }: { entry: TimelineEntry; tool: ToolCall | undefined }) {
  const t = useT();
  const icon = KIND_ICON[entry.kind];
  const subject = entrySubject(t, entry, tool);
  return (
    <div {...stylex.props(vs.rowTop, vs.gutter, vs.groupPad)}>
      <Icon name={icon} size="xs" className={stylex.props(ts.glyph).className} />
      <div {...stylex.props(vocab.fill)}>
        <div {...stylex.props(vs.lineBaseline)}>
          <span {...stylex.props(vocab.hold, ts.kind, typeStep.uiSm)}>
            {t(
              entry.kind === "tool" && entry.status !== undefined
                ? "timeline.kind.toolEnd"
                : KIND_I18N[entry.kind],
            )}
          </span>
          {subject.value && (
            // Named because it is the one place a tool reaches the timeline by name, and a
            // closure test checks that the name is the transcript's rather than the wire's.
            <span data-timeline-subject="" {...stylex.props(vs.subject)}>
              <ToolText value={subject} styles={[vocab.muted, typeStep.uiSm]} />
            </span>
          )}
        </div>
        {entry.kind === "tool" && entry.status !== undefined && tool?.exitCode !== undefined && (
          <div {...stylex.props(vocab.faint, face.mono, typeStep.uiXs)}>
            {t("tool.meta.exit", { code: tool.exitCode })}
          </div>
        )}
        {entry.kind === "tool" && entry.status !== undefined && tool?.error && (
          <div {...stylex.props(vs.body, typeStep.uiXs)}>{tool.error}</div>
        )}
      </div>
      {entry.status && (
        // `Icon` is `aria-hidden` by design and takes no name, so the label lives on a wrapper
        // that claims the role. Passing `aria-label` to the component compiles — TypeScript does
        // not check hyphenated JSX attributes against a component's props — and is dropped.
        <span
          role="img"
          aria-label={entry.status}
          {...stylex.props(ts.mark, toneInk[STATUS_MARK[entry.status].tone])}
        >
          <Icon name={STATUS_MARK[entry.status].icon} size="xs" />
        </span>
      )}
      {/* Held open for any row that carries a mark, so the mark itself keeps one edge: a run
          boundary has an outcome and no duration, and letting its slot collapse moved the
          glyph half an inch away from the column of glyphs above it. */}
      {entry.status !== undefined && (
        <span
          title={entry.kind === "tool" ? t("timeline.executionDuration") : undefined}
          {...stylex.props(ts.stamp, ts.duration, typeStep.uiXs)}
        >
          {entry.kind !== "tool"
            ? ""
            : tool?.durationMillis === undefined
              ? "—"
              : fmtDuration(tool.durationMillis)}
        </span>
      )}
      <time
        dateTime={new Date(entry.ts).toISOString()}
        title={`${t(KIND_I18N[entry.kind])}: ${formatDateTime(entry.ts)}`}
        {...stylex.props(ts.stamp, typeStep.uiXs)}
      >
        {formatClock(entry.ts, "second")}
      </time>
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
  const [showModels, setShowModels] = useState(false);
  if (!run) {
    return group.runId ? (
      <div {...stylex.props(vs.gutter, vs.sectionPad, vocab.faint, typeStep.uiXs, face.mono)}>
        {t("timeline.unknownRun", { id: group.runId })}
      </div>
    ) : null;
  }

  const status = timelineRunStatusView(run);
  const parentRunId = run.parentRunId;
  const spawnedByItemId = run.spawnedByItemId;
  const child = parentRunId !== null;
  return (
    <>
      <div {...stylex.props(ts.runHeader)}>
        <span {...stylex.props(vocab.firstLine, typeStep.uiSm)}>
          <Icon
            name={child ? "bot" : "branch"}
            size="sm"
            className={stylex.props(vocab.muted).className}
          />
        </span>
        <div {...stylex.props(vocab.fill)}>
          <div {...stylex.props(vs.titleLine)}>
            <span {...stylex.props(vocab.hold, vs.title, typeStep.uiSm)}>
              {t(child ? "timeline.delegatedRun" : "timeline.rootRun")}
            </span>
            <span
              title={run.id}
              {...stylex.props(vocab.truncate, vocab.faint, typeStep.uiXs, face.mono)}
            >
              {run.id}
            </span>
            <Badge tone={status.tone}>{t(status.labelKey)}</Badge>
            <span {...stylex.props(vocab.hold, vocab.faint, typeStep.uiXs, face.mono)}>
              {t("agent.steps", { count: status.stepCount })}
            </span>
          </div>
          {(status.detail || child) && (
            <div {...stylex.props(ts.runDetail, typeStep.uiXs)}>
              {status.detail && (
                <span title={status.detail} {...stylex.props(vocab.truncate)}>
                  {status.detail}
                </span>
              )}
              {child && (
                <span title={parentRunId} {...stylex.props(vocab.truncate, vocab.faint, face.mono)}>
                  {t("timeline.parentRun", { id: parentRunId })}
                </span>
              )}
            </div>
          )}
        </div>
        <span {...stylex.props(vocab.firstLine, typeStep.uiSm)}>
          <IconButton
            icon="bot"
            size="sm"
            quiet
            title={t("timeline.modelCalls")}
            aria-expanded={showModels}
            onClick={() => setShowModels(!showModels)}
          />
          {spawnedByItemId && (
            <IconButton
              icon="chat"
              size="sm"
              quiet
              title={t("timeline.locateParent")}
              onClick={() => {
                if (parentRunId && parentRunId !== run.rootRunId) {
                  openWorkspaceSubagentRun(parentRunId);
                } else {
                  locateWorkspaceTool(spawnedByItemId);
                }
              }}
            />
          )}
          {status.cancelable && (
            <IconButton
              icon="stop"
              size="sm"
              quiet
              disabled={!runtimeAvailable}
              title={t("agent.runTree.action.cancel")}
              onClick={() => {
                cancelSessionRun({ sessionId: run.sessionId, runId: run.id });
              }}
            />
          )}
        </span>
      </div>
      {showModels && <ModelInvocationHistory key={run.id} run={run} />}
    </>
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
      title="timeline.title"
      sub={
        timeline.length === TIMELINE_WINDOW_SIZE
          ? t("timeline.recentWindow", { count: TIMELINE_WINDOW_SIZE })
          : timelineSubtext(t, view)
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
              <p {...stylex.props(vs.gutter, vs.rowPad, vocab.pretty, vocab.faint, typeStep.uiXs)}>
                {t("timeline.noEvents")}
              </p>
            )}
          </div>
        ))
      )}
    </WorkspaceViewLayout>
  );
}
