import { iconGlyph } from "@/ui/icons";
import * as stylex from "@stylexjs/stylex";
import { AnimatePresence } from "motion/react";
import { Activity, useLayoutEffect, useMemo, useRef } from "react";
import { useStickToBottom } from "use-stick-to-bottom";
import { useT } from "@/lib/i18n";
import {
  useActiveConversationRows,
  delegatedTranscriptRows,
} from "@/plugins/builtin/agent/public/conversation";
import {
  openWorkspaceSubagentRun,
  useWorkspaceSubagentRunId,
  useExpandedWorkspaceToolIds,
  useToggleWorkspaceTool,
  useWorkspaceInspection,
} from "@/plugins/builtin/workspace/public/navigation";
import { ScrollArea, SectionLabel, StatusDot } from "@/ui";
import { EmptyState, IconButton, RecordBody, toneInk, vocab } from "@flame/ui";
import { AgentJumpToBottom, AgentWorkspaceView } from "@/ui/agent";
import { stopFollowOnReadingFocus } from "@/ui/agent/readingFollow";
import { face, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
import { subagentEntries, type SubagentEntry } from "../application/subagents";
import { delegatedRunSummary, type DelegatedRunSummary } from "../application/delegatedRunSummary";
import { DelegatedRunLink } from "./DelegatedRunLink";
import type { BlockCtx } from "./BlockRenderer";
import { useStreamRevealStore } from "../public/streamReveal";
import { MessageTurn } from "./MessageTurn";
import { UnresolvedRunEffects } from "./UnresolvedRunEffects";
import { SubagentStopButton } from "./SubagentStopButton";

const styles = stylex.create({
  header: {
    display: "flex",
    alignItems: "center",
    gap: space.s2,
    flexShrink: 0,
    minWidth: 0,
    minHeight: "var(--surface-header-height)",
    paddingInline: "var(--reading-gutter-wide)",
    paddingBlock: space.s2,
  },
  headLine: { minWidth: 0, flex: 1 },
  scroller: {
    paddingInline: "var(--reading-gutter-wide)",
    paddingBlock: space.s3,
  },
  group: { marginBottom: space.s4 },
  title: { marginBottom: space.s2 },
  reader: { position: "relative", display: "flex", flexDirection: "column", minHeight: 0, flex: 1 },
  transcript: { minWidth: 0, paddingBottom: space.s12 },
  evidence: { marginBottom: space.s6 },
  jump: {
    position: "absolute",
    left: "50%",
    bottom: space.s4,
    translate: "-50% 0",
    zIndex: 3,
    pointerEvents: "none",
  },
  summary: { display: "grid", gap: space.s0_5, minWidth: 0, flex: 1 },
  name: {
    display: "-webkit-box",
    WebkitBoxOrient: "vertical",
    WebkitLineClamp: 2,
    overflow: "hidden",
    overflowWrap: "anywhere",
    fontWeight: weight.medium,
  },
});

export function SubagentsPanel() {
  const t = useT();
  const rows = useActiveConversationRows();
  const entries = useMemo(() => subagentEntries(rows), [rows]);
  const selectedId = useWorkspaceSubagentRunId();
  const inspection = useWorkspaceInspection();
  const retainedRunIds = new Set<string>();
  for (let source = inspection; source; source = source.previous) {
    if (
      source.returnSubagentRunId &&
      (source.returnDockId === "subagents" || source.returnViewId === "subagents")
    ) {
      retainedRunIds.add(source.returnSubagentRunId);
    }
  }
  const panelRef = useRef<HTMLDivElement>(null);
  const backRef = useRef<HTMLButtonElement>(null);
  const previousRun = useRef<string | null>(null);
  const previousInspection = useRef(inspection);
  useLayoutEffect(() => {
    const fromRun = previousRun.current;
    const fromInspection = previousInspection.current;
    previousRun.current = selectedId;
    previousInspection.current = inspection;
    if (fromRun === selectedId || (fromInspection && fromInspection.previous === inspection))
      return;
    const panel = panelRef.current;
    if (!panel) return;
    const active = panel.ownerDocument.activeElement;
    const source = active?.closest<HTMLElement>('[data-slot="delegated-run-link"]');
    if (
      active !== panel.ownerDocument.body &&
      !panel.contains(active) &&
      !(
        selectedId !== null &&
        source?.dataset.runId === selectedId &&
        source.querySelector("button") === active
      )
    )
      return;
    if (selectedId) {
      backRef.current?.focus({ preventScroll: true });
    } else if (fromRun) {
      const row = [...panel.querySelectorAll<HTMLElement>('[data-slot="delegated-run-link"]')].find(
        (node) => node.dataset.runId === fromRun,
      );
      (row?.querySelector<HTMLButtonElement>("button") ?? panel).focus();
    }
  }, [selectedId, inspection]);
  const selected = entries.find(({ narrative }) => narrative.run.id === selectedId);
  const summary = selected
    ? delegatedRunSummary(
        t,
        selected.narrative.run,
        selected.ordinal,
        selected.siblingCount,
        selected.taskLabel,
      )
    : null;
  const running = entries.filter(({ narrative }) => narrative.run.status === "running").length;
  return (
    <AgentWorkspaceView ref={panelRef} tabIndex={-1} ariaLabel={t("subagents.title")}>
      <header data-slot="subagent-header" {...stylex.props(styles.header)}>
        {selectedId && (
          <IconButton
            ref={backRef}
            icon={iconGlyph("list")}
            size="sm"
            aria-label={t("subagents.all")}
            onClick={() => openWorkspaceSubagentRun(null)}
          />
        )}
        {summary ? (
          <div {...stylex.props(styles.summary)}>
            <span
              data-slot="subagent-identity"
              title={summary.label}
              {...stylex.props(styles.name, typeStep.uiSm)}
            >
              {summary.label}
            </span>
            <span {...stylex.props(vocab.line, toneInk[summary.ink], typeStep.uiXs)}>
              <StatusDot tone={summary.dotTone} />
              {summary.statusLabel}
            </span>
          </div>
        ) : (
          <span
            {...stylex.props(
              styles.headLine,
              vocab.truncate,
              vocab.muted,
              typeStep.uiSm,
              face.mono,
            )}
          >
            {t("subagents.count", { count: entries.length })}
            {running > 0 && ` · ${t("subagents.running", { count: running })}`}
          </span>
        )}
        {selected && summary?.cancelable && <SubagentStopButton run={selected.narrative.run} />}
      </header>
      <Activity key={entries[0]?.narrative.run.sessionId} mode={selectedId ? "hidden" : "visible"}>
        <ScrollArea className={stylex.props(styles.scroller).className}>
          {entries.length === 0 ? (
            <EmptyState icon={iconGlyph("bot")} title={t("subagents.empty")} />
          ) : (
            <div role="group" aria-label={t("subagents.title")}>
              <SubagentGroup
                title={t("subagents.active")}
                entries={entries.filter(({ narrative }) => narrative.run.status !== "finished")}
              />
              <SubagentGroup
                title={t("subagents.finished")}
                entries={entries.filter(({ narrative }) => narrative.run.status === "finished")}
              />
            </div>
          )}
        </ScrollArea>
      </Activity>
      {entries
        .filter(
          ({ narrative }) =>
            narrative.run.id === selectedId || retainedRunIds.has(narrative.run.id),
        )
        .map((entry) => (
          <Activity
            key={entry.narrative.run.id}
            mode={entry.narrative.run.id === selectedId ? "visible" : "hidden"}
          >
            <SubagentTranscript
              entry={entry}
              summary={delegatedRunSummary(
                t,
                entry.narrative.run,
                entry.ordinal,
                entry.siblingCount,
                entry.taskLabel,
              )}
            />
          </Activity>
        ))}
      {!selected && selectedId ? (
        <EmptyState icon={iconGlyph("bot")} title={t("subagents.unavailable")} />
      ) : null}
    </AgentWorkspaceView>
  );
}

function SubagentGroup({ title, entries }: { title: string; entries: SubagentEntry[] }) {
  if (entries.length === 0) return null;
  return (
    <div {...stylex.props(styles.group)}>
      <SectionLabel className={stylex.props(styles.title).className}>
        {title} · {entries.length}
      </SectionLabel>
      {entries.map(({ narrative, ordinal, siblingCount, taskLabel }) => (
        <DelegatedRunLink
          key={narrative.run.id}
          run={narrative.run}
          taskLabel={taskLabel}
          ordinal={ordinal}
          siblingCount={siblingCount}
          onOpen={() => openWorkspaceSubagentRun(narrative.run.id)}
        />
      ))}
    </div>
  );
}

function SubagentTranscript({
  entry,
  summary,
}: {
  entry: SubagentEntry;
  summary: DelegatedRunSummary;
}) {
  const t = useT();
  const { scrollRef, contentRef, isAtBottom, scrollToBottom, state, stopScroll } = useStickToBottom(
    {
      initial: "instant",
      resize: "instant",
    },
  );
  const expandedIds = useExpandedWorkspaceToolIds();
  const onToggleExpand = useToggleWorkspaceTool();
  const textReveal = useStreamRevealStore((state) => state.streamReveal);
  const ctx: BlockCtx = { expandedIds, onToggleExpand, textReveal };
  const { narrative, facts } = entry;
  const rows = useMemo(() => delegatedTranscriptRows(narrative, facts), [narrative, facts]);
  return (
    <div {...stylex.props(styles.reader)}>
      <ScrollArea ref={scrollRef} className={stylex.props(styles.scroller).className}>
        <div
          ref={contentRef}
          role="region"
          aria-label={summary.label}
          tabIndex={-1}
          onFocusCapture={(event) =>
            stopFollowOnReadingFocus(event, { scrollRef, state, stopScroll })
          }
          {...stylex.props(styles.transcript)}
        >
          {(summary.detail || narrative.run.outcome?.unresolvedEffects?.length) && (
            <div {...stylex.props(styles.evidence)}>
              {summary.detail && (
                <div {...stylex.props(typeStep.uiSm, toneInk[summary.ink])}>
                  <RecordBody>{summary.detail}</RecordBody>
                </div>
              )}
              <UnresolvedRunEffects effects={narrative.run.outcome?.unresolvedEffects} />
            </div>
          )}
          {rows.length === 0 && (
            <p {...stylex.props(vocab.muted, typeStep.uiSm)}>{t("agent.runTree.material.empty")}</p>
          )}
          <AnimatePresence initial={false}>
            {rows.map((row, index) => (
              <MessageTurn
                key={row.message.id}
                row={row}
                ctx={ctx}
                sessionId={narrative.run.sessionId}
                isLast={index === rows.length - 1}
                isRunning={narrative.run.status === "running"}
                previous={rows[index - 1]?.message}
                next={rows[index + 1]?.message}
              />
            ))}
          </AnimatePresence>
        </div>
      </ScrollArea>
      <div {...stylex.props(styles.jump)}>
        <AgentJumpToBottom
          visible={!isAtBottom}
          onClick={() => {
            contentRef.current?.focus({ preventScroll: true });
            void scrollToBottom();
          }}
        />
      </div>
    </div>
  );
}
