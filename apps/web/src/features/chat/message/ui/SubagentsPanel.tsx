import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { AnimatePresence } from "motion/react";
import { Activity, useLayoutEffect, useMemo, useRef } from "react";
import { useStickToBottom } from "use-stick-to-bottom";
import { useT } from "@/lib/i18n/index";
import { useActiveConversationRows } from "@/features/agent/public/conversation";
import { delegatedTranscriptRows } from "@/features/agent/public/conversation";
import {
  openWorkspaceSubagentRun,
  useWorkspaceSubagentRunId,
  useExpandedWorkspaceToolIds,
  useToggleWorkspaceTool,
  useWorkspaceInspection,
} from "@/features/workspace/public/navigation";
import { ScrollArea } from "@/ui/index";
import { SectionLabel } from "@/ui/index";
import { StatusDot } from "@/ui/index";
import { useContentVisible } from "@/ui/index";
import { EmptyState, IconButton, RecordBody, toneInk, vocab } from "@flame/ui";
import { AgentJumpToBottom } from "@/ui/agent/index";
import { AgentWorkspaceView } from "@/ui/agent/index";
import { stopFollowOnReadingFocus } from "@/ui/agent/readingFollow";
import { face, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
import { subagentEntries, type SubagentEntry } from "@/features/chat/message/application/subagents";
import {
  delegatedRunSummary,
  type DelegatedRunSummary,
} from "@/features/chat/message/application/delegatedRunSummary";
import { DelegatedRunLink } from "@/features/chat/message/ui/DelegatedRunLink";
import { type BlockCtx } from "@/features/chat/message/ui/BlockRenderer";
import { useStreamRevealStore } from "@/features/chat/public/message/streamReveal";
import { MessageTurn } from "@/features/chat/message/ui/MessageTurn";
import { readingColumn as rc } from "./readingColumn";
import { UnresolvedRunEffects } from "@/features/chat/message/ui/UnresolvedRunEffects";
import { SubagentStopButton } from "@/features/chat/message/ui/SubagentStopButton";
const styles = stylex.create({
  header: {
    display: "flex",
    alignItems: "center",
    gap: space.s2,
    flexShrink: 0,
    minWidth: 0,
    minHeight: "var(--surface-header-height)",
    paddingInline: "var(--layout-compact-gutter)",
    paddingBlock: space.s2,
  },
  headLine: { minWidth: 0, flex: 1 },
  scroller: {
    paddingInline: "var(--layout-compact-gutter)",
    paddingBlock: space.s3,
    scrollPaddingBlockEnd:
      "calc(var(--control-height-md) + var(--transcript-jump-gap) + var(--layout-control-gap))",
  },
  group: { marginBottom: space.s4 },
  title: { marginBottom: space.s2 },
  reader: {
    position: "relative",
    display: "flex",
    flexDirection: "column",
    minHeight: 0,
    flex: 1,
    containerType: "inline-size",
    containerName: "conversation",
    "--transcript-jump-gap": space.s4,
  },
  transcript: { minWidth: 0, paddingBottom: space.s12 },
  evidence: { marginBottom: space.s6 },
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
            icon={iconGlyph("chevron-left")}
            size="sm"
            aria-label={t("subagents.all")}
            data-chrome-focus=""
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
          <SubagentTranscript
            key={entry.narrative.run.id}
            hidden={entry.narrative.run.id !== selectedId}
            entry={entry}
            summary={delegatedRunSummary(
              t,
              entry.narrative.run,
              entry.ordinal,
              entry.siblingCount,
              entry.taskLabel,
            )}
          />
        ))}
      {!selected && selectedId ? (
        <EmptyState icon={iconGlyph("alert")} tone="negative" title={t("subagents.unavailable")} />
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
  hidden,
}: {
  entry: SubagentEntry;
  summary: DelegatedRunSummary;
  hidden: boolean;
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
  const ctx = useMemo<BlockCtx>(
    () => ({ sessionId: entry.narrative.run.sessionId, expandedIds, onToggleExpand, textReveal }),
    [entry.narrative.run.sessionId, expandedIds, onToggleExpand, textReveal],
  );
  const measured = useContentVisible() && !hidden;
  const { narrative, facts } = entry;
  const rows = useMemo(() => delegatedTranscriptRows(narrative, facts), [narrative, facts]);
  return (
    <div hidden={hidden} {...stylex.props(styles.reader)}>
      <ScrollArea
        ref={measured ? scrollRef : undefined}
        className={stylex.props(styles.scroller).className}
      >
        <div
          ref={measured ? contentRef : undefined}
          role="region"
          aria-label={summary.label}
          tabIndex={-1}
          onFocusCapture={(event) =>
            stopFollowOnReadingFocus(event, { scrollRef, state, stopScroll })
          }
          {...stylex.props(rc.box, styles.transcript)}
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
      <AgentJumpToBottom
        visible={!isAtBottom}
        onClick={() => {
          contentRef.current?.focus({ preventScroll: true });
          void scrollToBottom();
        }}
        placement="inside-reader"
      />
    </div>
  );
}
