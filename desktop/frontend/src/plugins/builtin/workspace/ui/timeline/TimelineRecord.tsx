import { useState, type ReactNode } from "react";
import * as stylex from "@stylexjs/stylex";
import type { AgentItem, AgentRunFact } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { formatClock, formatDateTime } from "@/lib/i18n/relativeTime";
import { fmtCost, fmtDuration, fmtTokens } from "@/lib/format";
import {
  Badge,
  DataView,
  IconButton,
  ShikiCodeBlock,
  TextPreview,
  vocab,
  type IconName,
} from "@/ui";
import { AgentActivityDisclosure, ToolText } from "@/ui/agent";
import { face, space, type as typeStep } from "@/styles/tokens.stylex";
import {
  cancelSessionRun,
  useTrajectoryRun,
  type ModelInvocation,
} from "@/plugins/builtin/agent/public/run";
import { locateWorkspaceTool, openWorkspaceSubagentRun } from "../../public/navigation";
import {
  type TimelineKind,
  type TimelineRecord as RecordView,
} from "../../application/timelineViewModel";
import { projectToolCall, toolIntent } from "@/plugins/builtin/agent/public/messagePresentation";
import { toolCallIconFor } from "@/plugins/builtin/agent/public/toolIcon";
import { viewStyles as vs } from "../viewStyles";

const styles = stylex.create({
  row: { paddingBlock: space.s1_5 },
  context: { display: "flex", gap: space.s2, minWidth: 0, paddingLeft: space.s6 },
  details: { paddingLeft: space.s6, paddingBlock: space.s2 },
  metadata: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) minmax(0, 1.5fr)",
    gap: space.s2,
    margin: 0,
  },
  value: { margin: 0, minWidth: 0, overflowWrap: "anywhere" },
  evidence: { marginTop: space.s3 },
  figure: { marginInline: 0, marginBottom: 0, marginTop: space.s3 },
  image: {
    display: "block",
    maxWidth: "100%",
    maxHeight: "calc(var(--spacing) * 64)",
    objectFit: "contain",
  },
});

const KIND: Record<TimelineKind, { icon: IconName; labelKey: string }> = {
  run: { icon: "branch", labelKey: "timeline.kind.run" },
  model: { icon: "bot", labelKey: "timeline.kind.model" },
  toolCall: { icon: "tool", labelKey: "timeline.kind.tool" },
  userMessage: { icon: "user", labelKey: "timeline.kind.input" },
  agentMessage: { icon: "chat", labelKey: "timeline.kind.output" },
  reasoning: { icon: "brain", labelKey: "timeline.kind.reasoning" },
  question: { icon: "chat", labelKey: "timeline.kind.question" },
  compaction: { icon: "history", labelKey: "timeline.kind.compaction" },
};

export function TimelineRecord({
  record,
  runtimeAvailable,
}: {
  record: RecordView;
  runtimeAvailable: boolean;
}) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const kind = KIND[record.kind];
  const tool =
    record.source.type === "item" && record.source.item.type === "toolCall"
      ? projectToolCall(record.source.item)
      : undefined;
  const intent = tool ? toolIntent(t, tool) : undefined;
  const run = record.source.type === "run" ? record.source.run : undefined;
  return (
    <div
      data-trajectory-kind={record.kind}
      data-trajectory-record={record.key}
      {...stylex.props(vs.gutter, styles.row)}
    >
      <AgentActivityDisclosure
        shell="line"
        icon={tool ? toolCallIconFor(tool) : run?.parentRunId ? "bot" : kind.icon}
        label={intent ? <ToolText value={intent.label} /> : t(kind.labelKey)}
        detail={
          intent ? (
            intent.detail ? (
              <span data-timeline-subject="" {...stylex.props(vocab.truncate)}>
                <ToolText value={intent.detail} />
              </span>
            ) : undefined
          ) : (
            <span title={record.summary} {...stylex.props(vocab.truncate)}>
              {record.summary}
            </span>
          )
        }
        trailing={
          <>
            {(record.kind === "model" || record.kind === "toolCall" || record.kind === "run") && (
              <span
                title={t(
                  record.kind === "toolCall"
                    ? "timeline.executionDuration"
                    : "timeline.wallDuration",
                )}
              >
                {record.durationMillis === undefined ? "—" : fmtDuration(record.durationMillis)}
              </span>
            )}
            <time dateTime={record.occurredAt} title={formatDateTime(record.occurredAt)}>
              {formatClock(Date.parse(record.occurredAt), "second")}
            </time>
          </>
        }
        actions={run && <RunActions run={run} runtimeAvailable={runtimeAvailable} />}
        toggleLabel={t("timeline.inspect", { id: record.id })}
        open={open}
        onToggle={() => setOpen(!open)}
      >
        <div {...stylex.props(styles.details)}>
          <dl {...stylex.props(styles.metadata, typeStep.uiXs)}>
            <Datum label={t("timeline.identity")}>{record.id}</Datum>
            <Datum label={t("timeline.runIdentity")}>{record.runId}</Datum>
            <Datum label={t("timeline.startedAt")}>{record.occurredAt}</Datum>
            <RecordMetrics record={record} />
          </dl>
          {record.source.type !== "run" && <RunContext runId={record.runId} />}
          <RecordEvidence record={record} />
        </div>
      </AgentActivityDisclosure>
      <div {...stylex.props(styles.context, typeStep.uiXs)}>
        <Badge tone={record.tone}>{t(record.statusKey)}</Badge>
        <span title={record.runId} {...stylex.props(vocab.truncate, vocab.faint, face.mono)}>
          {record.runId}
        </span>
      </div>
    </div>
  );
}

function RunActions({ run, runtimeAvailable }: { run: AgentRunFact; runtimeAvailable: boolean }) {
  const t = useT();
  return (
    <>
      {run.spawnedByItemId && (
        <IconButton
          icon="chat"
          size="sm"
          quiet
          title={t("timeline.locateParent")}
          onClick={() => {
            if (run.parentRunId && run.parentRunId !== run.rootRunId)
              openWorkspaceSubagentRun(run.parentRunId);
            else locateWorkspaceTool(run.spawnedByItemId!);
          }}
        />
      )}
      {run.status !== "finished" && (
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
    </>
  );
}

function Datum({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt {...stylex.props(vocab.faint)}>{label}</dt>
      <dd {...stylex.props(styles.value, face.mono, vocab.figures)}>{children}</dd>
    </>
  );
}

function RecordMetrics({ record }: { record: RecordView }) {
  const t = useT();
  const source = record.source;
  if (source.type === "model")
    return <ModelMetrics call={source.model} durationMillis={record.durationMillis} />;
  if (source.type === "run") {
    const run = source.run;
    return (
      <>
        {run.modelSelection && (
          <Datum label={t("timeline.kind.model")}>
            {run.modelSelection.provider}/{run.modelSelection.model}
          </Datum>
        )}
        {run.modelSelection?.reasoningEffort && (
          <Datum label={t("composer.model.reasoning")}>{run.modelSelection.reasoningEffort}</Datum>
        )}
        {run.parentRunId && <Datum label={t("timeline.parentIdentity")}>{run.parentRunId}</Datum>}
        {run.spawnedByItemId && (
          <Datum label={t("timeline.spawnedBy")}>{run.spawnedByItemId}</Datum>
        )}
        {run.activeSegmentId && (
          <Datum label={t("timeline.segmentIdentity")}>{run.activeSegmentId}</Datum>
        )}
        {run.finishedAt && <Datum label={t("timeline.finishedAt")}>{run.finishedAt}</Datum>}
        <Datum label={t("timeline.activeDuration")}>
          {fmtDuration(run.metrics.activeDurationMillis)}
        </Datum>
        <Datum label={t("timeline.wallDuration")}>
          {record.durationMillis === undefined ? "—" : fmtDuration(record.durationMillis)}
        </Datum>
        <Datum label={t("timeline.steps")}>{run.metrics.steps}</Datum>
        <Datum label={t("timeline.inputTokens")}>{fmtTokens(run.metrics.usage.inputTokens)}</Datum>
        <Datum label={t("timeline.outputTokens")}>
          {fmtTokens(run.metrics.usage.outputTokens)}
        </Datum>
        <Datum label={t("usage.cache")}>{fmtTokens(run.metrics.usage.cacheReadTokens)}</Datum>
        {run.metrics.usage.costUsd !== undefined && (
          <Datum label={t("timeline.cost")}>{fmtCost(run.metrics.usage.costUsd)}</Datum>
        )}
      </>
    );
  }
  const item = source.item;
  if (item.type === "toolCall")
    return (
      <>
        {item.finishedAt && <Datum label={t("timeline.finishedAt")}>{item.finishedAt}</Datum>}
        <Datum label={t("timeline.executionDuration")}>
          {item.durationMillis === undefined ? "—" : fmtDuration(item.durationMillis)}
        </Datum>
        {item.approvalDecision && (
          <Datum label={t("timeline.approval")}>
            {t(`timeline.state.${item.approvalDecision}`)}
          </Datum>
        )}
      </>
    );
  if (item.type === "agentMessage" && item.phase)
    return <Datum label={t("timeline.phase")}>{t(`timeline.phase.${item.phase}`)}</Datum>;
  if (item.type === "compaction" && item.droppedMessages !== undefined)
    return <Datum label={t("timeline.droppedMessages")}>{item.droppedMessages}</Datum>;
  return null;
}

function ModelMetrics({
  call,
  durationMillis,
}: {
  call: ModelInvocation;
  durationMillis: number | undefined;
}) {
  const t = useT();
  return (
    <>
      <Datum label={t("timeline.segmentIdentity")}>{call.segmentId}</Datum>
      {call.settledAt && <Datum label={t("timeline.settledAt")}>{call.settledAt}</Datum>}
      <Datum label={t("timeline.wallDuration")}>
        {durationMillis === undefined ? "—" : fmtDuration(durationMillis)}
      </Datum>
      <Datum label={t("timeline.firstOutput")}>
        {call.firstOutputLatencyMillis === undefined
          ? "—"
          : fmtDuration(call.firstOutputLatencyMillis)}
      </Datum>
      <Datum label={t("timeline.inputTokens")}>
        {call.usage === undefined ? (
          "—"
        ) : (
          <span title={String(call.usage.inputTokens)}>{fmtTokens(call.usage.inputTokens)}</span>
        )}
      </Datum>
      <Datum label={t("timeline.outputTokens")}>
        {call.usage === undefined ? (
          "—"
        ) : (
          <span title={String(call.usage.outputTokens)}>{fmtTokens(call.usage.outputTokens)}</span>
        )}
      </Datum>
      {call.usage?.cacheReadTokens !== undefined && (
        <Datum label={t("usage.cache")}>{fmtTokens(call.usage.cacheReadTokens)}</Datum>
      )}
      {call.usage?.cacheWriteTokens !== undefined && (
        <Datum label={t("usage.cacheWrite")}>{fmtTokens(call.usage.cacheWriteTokens)}</Datum>
      )}
      {call.usage?.reasoningTokens !== undefined && (
        <Datum label={t("usage.reasoning")}>{fmtTokens(call.usage.reasoningTokens)}</Datum>
      )}
      {call.state === "unknown" && (
        <Datum label={t("timeline.observation")}>
          <span {...stylex.props(face.text)}>{t("timeline.unknownObservation")}</span>
        </Datum>
      )}
    </>
  );
}

function Evidence({
  title,
  value,
  prose = false,
}: {
  title: string;
  value: unknown;
  prose?: boolean;
}) {
  return (
    <section {...stylex.props(styles.evidence)}>
      <div {...stylex.props(vocab.muted, typeStep.uiXs)}>{title}</div>
      {typeof value === "string" ? (
        prose ? (
          <p {...stylex.props(vs.body, vocab.wrapText, typeStep.uiMd)}>{value}</p>
        ) : (
          <TextPreview wrap="words">{value}</TextPreview>
        )
      ) : (
        <ShikiCodeBlock lang="json" code={JSON.stringify(value, null, 2)} />
      )}
    </section>
  );
}

function RecordEvidence({ record }: { record: RecordView }) {
  const t = useT();
  const source = record.source;
  if (source.type === "model") return null;
  if (source.type === "run")
    return source.run.outcome ? (
      <Evidence title={t("timeline.outcome")} value={source.run.outcome} />
    ) : null;
  const item = source.item;
  if (item.type === "toolCall")
    return (
      <>
        <Evidence
          title={t("timeline.arguments")}
          value={item.tool.argumentsText ?? item.tool.arguments}
        />
        {item.tool.result !== undefined && (
          <Evidence title={t("timeline.result")} value={item.tool.result} />
        )}
        {item.error && <Evidence title={t("timeline.error")} value={item.error} />}
      </>
    );
  if (item.type === "reasoning" && item.redacted)
    return <Evidence title={t("timeline.kind.reasoning")} value={t("timeline.redacted")} prose />;
  if (item.type === "question")
    return <Evidence title={t("timeline.kind.question")} value={item.question} />;
  if (item.type === "compaction")
    return <Evidence title={t("timeline.kind.compaction")} value={item.summary} prose />;
  if (item.type === "reasoning")
    return (
      <Evidence
        title={t("timeline.kind.reasoning")}
        value={item.text ?? t("timeline.unrecorded")}
        prose
      />
    );
  return <MessageEvidence item={item} />;
}

function MessageEvidence({
  item,
}: {
  item: Extract<AgentItem, { type: "userMessage" | "agentMessage" }>;
}) {
  const t = useT();
  if (!item.content?.length)
    return <Evidence title={t("timeline.content")} value={t("timeline.unrecorded")} prose />;
  return item.content.map((part, index) =>
    part.type === "text" ? (
      <Evidence key={index} title={t("timeline.content")} value={part.text} prose />
    ) : (
      <figure key={index} {...stylex.props(styles.figure)}>
        <figcaption {...stylex.props(vocab.muted, typeStep.uiXs)}>
          {t("timeline.image")} <span {...stylex.props(face.mono)}>{part.mime}</span>
        </figcaption>
        <img
          src={`data:${part.mime};base64,${part.data}`}
          alt={part.mime}
          {...stylex.props(styles.image)}
        />
      </figure>
    ),
  );
}

function RunContext({ runId }: { runId: string }) {
  const t = useT();
  const query = useTrajectoryRun(runId);
  return (
    <div {...stylex.props(styles.evidence)}>
      <DataView
        items={query.data ? [query.data] : []}
        isLoading={query.isLoading}
        failure={query.error}
        onRetry={() => {
          void query.refetch();
        }}
        skeletonCount={1}
      >
        {(runs) =>
          runs.map((run) => (
            <dl key={run.id} {...stylex.props(styles.metadata, typeStep.uiXs)}>
              {run.modelSelection && (
                <Datum label={t("timeline.kind.model")}>
                  {run.modelSelection.provider}/{run.modelSelection.model}
                </Datum>
              )}
              {run.modelSelection?.reasoningEffort && (
                <Datum label={t("composer.model.reasoning")}>
                  {run.modelSelection.reasoningEffort}
                </Datum>
              )}
              <Datum label={t("timeline.rootIdentity")}>{run.rootRunId}</Datum>
              {run.parentRunId && (
                <Datum label={t("timeline.parentIdentity")}>{run.parentRunId}</Datum>
              )}
              {run.spawnedByItemId && (
                <Datum label={t("timeline.spawnedBy")}>{run.spawnedByItemId}</Datum>
              )}
            </dl>
          ))
        }
      </DataView>
    </div>
  );
}
