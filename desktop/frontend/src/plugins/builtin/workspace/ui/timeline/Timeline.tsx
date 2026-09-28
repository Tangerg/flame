import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { useActiveSession, useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { useSessionTrajectory } from "@/plugins/builtin/agent/public/run";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { exportSessionTrajectory } from "@/plugins/builtin/workspace/public/conversationArchive";
import {
  Button,
  DataView,
  DropdownMenu,
  EmptyState,
  IconButton,
  SearchField,
  SelectTrigger,
  vocab,
} from "@/ui";
import { face, space, type as typeStep } from "@/styles/tokens.stylex";
import { type TimelineCategory, timelineViewModel } from "../../application/timelineViewModel";
import { WorkspaceViewLayout } from "../WorkspaceViewLayout";
import { viewStyles as vs } from "../viewStyles";
import { TimelineRecord } from "./TimelineRecord";

const styles = stylex.create({
  toolbar: { display: "grid", gap: space.s2, paddingBlock: space.s2 },
  filters: { display: "flex", flexWrap: "wrap", gap: space.s2, minWidth: 0 },
  filter: { minWidth: 0, maxWidth: "100%" },
  metrics: { display: "flex", flexWrap: "wrap", gap: space.s3 },
  pagination: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: space.s2,
    paddingBlock: space.s3,
  },
});

const CATEGORIES: readonly TimelineCategory[] = [
  "all",
  "model",
  "toolCall",
  "message",
  "run",
  "attention",
];

export function Timeline() {
  const sessionId = useActiveSessionId();
  const includeDescendants = useRuntimeCapability("subagents");
  return (
    <SessionTimeline
      key={`${sessionId}:${includeDescendants}`}
      sessionId={sessionId}
      includeDescendants={includeDescendants}
    />
  );
}

function SessionTimeline({
  sessionId,
  includeDescendants,
}: {
  sessionId: string;
  includeDescendants: boolean;
}) {
  const t = useT();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const canExport = useRuntimeCapability("sessionExport");
  const session = useActiveSession();
  const [cursors, setCursors] = useState<string[]>([]);
  const [category, setCategory] = useState<TimelineCategory>("all");
  const [query, setQuery] = useState("");
  const [runId, setRunId] = useState<string | null>(null);
  const [exporting, setExporting] = useState(false);
  const cursor = cursors.at(-1);
  const history = useSessionTrajectory(sessionId || null, includeDescendants, cursor);
  const view = timelineViewModel(history.data?.data ?? [], { category, query, runId });
  const exportReady = runtimeAvailable && canExport && session?.status === "idle";

  async function exportTrajectory() {
    setExporting(true);
    try {
      await exportSessionTrajectory();
    } catch (error) {
      notifyError(error instanceof Error ? error.message : t("timeline.exportFailed"), {
        source: "session",
      });
    } finally {
      setExporting(false);
    }
  }

  return (
    <WorkspaceViewLayout
      icon="history"
      title="timeline.title"
      sub={t("timeline.pageSummary", { visible: view.records.length, total: view.recordCount })}
      actions={
        <>
          <IconButton
            icon="loop"
            size="sm"
            disabled={!sessionId || !runtimeAvailable || history.isFetching}
            title={t("timeline.refresh")}
            onClick={() => {
              void history.refetch();
            }}
          />
          <IconButton
            icon="download"
            size="sm"
            disabled={!sessionId || !exportReady || exporting}
            title={t(session?.status === "idle" ? "timeline.export" : "timeline.exportWhenIdle")}
            onClick={() => {
              void exportTrajectory();
            }}
          />
        </>
      }
    >
      {!sessionId ? (
        <EmptyState
          icon="history"
          title={t("timeline.empty.title")}
          sub={t("timeline.empty.sub")}
        />
      ) : (
        <>
          <div {...stylex.props(vs.gutter, styles.toolbar)}>
            <SearchField
              size="sm"
              value={query}
              aria-label={t("timeline.search")}
              placeholder={t("timeline.search")}
              onChange={(event) => setQuery(event.target.value)}
              onClear={() => setQuery("")}
              clearLabel={t("timeline.clearSearch")}
            />
            <div {...stylex.props(styles.filters)}>
              <DropdownMenu.Root>
                <DropdownMenu.Trigger
                  render={
                    <SelectTrigger
                      aria-label={t("timeline.filterKind")}
                      label={t(`timeline.filter.${category}`)}
                      className={stylex.props(styles.filter).className}
                    />
                  }
                />
                <DropdownMenu.Content>
                  {CATEGORIES.map((value) => (
                    <DropdownMenu.Item key={value} onClick={() => setCategory(value)}>
                      {t(`timeline.filter.${value}`)}
                    </DropdownMenu.Item>
                  ))}
                </DropdownMenu.Content>
              </DropdownMenu.Root>
              <DropdownMenu.Root>
                <DropdownMenu.Trigger
                  render={
                    <SelectTrigger
                      aria-label={t("timeline.filterRun")}
                      label={runId ?? t("timeline.allRuns")}
                      className={stylex.props(styles.filter).className}
                    />
                  }
                />
                <DropdownMenu.Content>
                  <DropdownMenu.Item onClick={() => setRunId(null)}>
                    {t("timeline.allRuns")}
                  </DropdownMenu.Item>
                  {view.runIds.map((id) => (
                    <DropdownMenu.Item key={id} onClick={() => setRunId(id)}>
                      {id}
                    </DropdownMenu.Item>
                  ))}
                </DropdownMenu.Content>
              </DropdownMenu.Root>
            </div>
            <div {...stylex.props(styles.metrics, vocab.muted, typeStep.uiXs, vocab.figures)}>
              <span>
                {t("timeline.pageModels")}{" "}
                <span {...stylex.props(face.mono)}>{view.modelCount}</span>
              </span>
              <span>
                {t("timeline.pageTools")} <span {...stylex.props(face.mono)}>{view.toolCount}</span>
              </span>
              <span>
                {t("timeline.pageAttention")}{" "}
                <span {...stylex.props(face.mono)}>{view.attentionCount}</span>
              </span>
            </div>
            <p {...stylex.props(vocab.faint, typeStep.uiXs)}>{t("timeline.pageScope")}</p>
          </div>
          <DataView
            items={view.records}
            isLoading={history.isLoading}
            failure={history.error}
            onRetry={() => {
              void history.refetch();
            }}
            skeletonCount={4}
            empty={{
              icon: "history",
              title: t(view.recordCount === 0 ? "timeline.empty.title" : "timeline.noMatches"),
              sub: t(view.recordCount === 0 ? "timeline.empty.sub" : "timeline.filterScope"),
            }}
          >
            {(records) =>
              records.map((record) => (
                <TimelineRecord
                  key={record.key}
                  record={record}
                  runtimeAvailable={runtimeAvailable}
                />
              ))
            }
          </DataView>
          <div {...stylex.props(vs.gutter, styles.pagination)}>
            {cursor && (
              <>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={history.isLoading}
                  onClick={() => setCursors([])}
                >
                  {t("timeline.newest")}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={history.isLoading}
                  onClick={() => setCursors((current) => current.slice(0, -1))}
                >
                  {t("timeline.newer")}
                </Button>
              </>
            )}
            {history.data?.nextCursor && (
              <Button
                variant="ghost"
                size="sm"
                disabled={history.isFetching}
                onClick={() => setCursors((current) => [...current, history.data!.nextCursor!])}
              >
                {t("timeline.older")}
              </Button>
            )}
            {history.data && !history.data.nextCursor && (
              <span {...stylex.props(vocab.faint, typeStep.uiXs)}>{t("timeline.end")}</span>
            )}
          </div>
        </>
      )}
    </WorkspaceViewLayout>
  );
}
