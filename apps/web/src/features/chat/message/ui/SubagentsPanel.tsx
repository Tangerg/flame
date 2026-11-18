import type { ReadingListHandle } from "@flame/ui";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import {
  Activity,
  Component,
  useId,
  useState,
  useLayoutEffect,
  useMemo,
  useRef,
  type ReactNode,
  type RefObject,
} from "react";
import { useT } from "@/lib/i18n/index";
import { useActiveConversationRows } from "@/features/agent/public/conversation";
import { useActiveSessionId } from "@/features/agent/public/session";
import { delegatedTranscriptRows } from "@/features/agent/public/conversation";
import { agentRunIsFinished, agentRunIsRunning } from "@/features/agent/public/runPresentation";
import {
  openWorkspaceSubagentRun,
  useWorkspaceSubagentRunId,
  useExpandedWorkspaceToolIds,
  useSetWorkspaceToolExpanded,
  useWorkspaceInspection,
} from "@/features/workspace/public/navigation";
import { ScrollArea } from "@/ui/index";
import { SectionLabel } from "@/ui/index";
import { StatusDot } from "@/ui/index";
import { ContentVisibility, useContentVisible } from "@/ui/index";
import { EmptyState, IconButton, RecordBody, toneInk, vocab } from "@flame/ui";
import { AgentJumpToBottom } from "@/ui/agent/index";
import { AgentWorkspaceView } from "@/ui/agent/index";
import { face, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
import { subagentEntries, type SubagentEntry } from "@/features/chat/message/application/subagents";
import {
  delegatedRunSummary,
  type DelegatedRunSummary,
} from "@/features/chat/message/application/delegatedRunSummary";
import { DelegatedRunLink } from "@/features/chat/message/ui/DelegatedRunLink";
import { type BlockCtx } from "@/features/chat/message/ui/BlockRenderer";
import { MessageTranscript } from "@/features/chat/message/ui/MessageTranscript";
import { readingColumn as rc } from "./readingColumn";
import { UnresolvedRunEffects } from "@/features/chat/message/ui/UnresolvedRunEffects";
import { SubagentStopButton } from "@/features/chat/message/ui/SubagentStopButton";
const styles = stylex.create({
  header: {
    display: "flex",
    alignItems: "flex-start",
    gap: space.s2,
    flexShrink: 0,
    minWidth: 0,
    minHeight: "var(--surface-header-height)",
    // Status and controls must still leave one complete line for the scrollable title.
    maxHeight: "max(calc(100% / 3), calc(var(--surface-header-height) + 1lh))",
    paddingInline: "var(--layout-compact-gutter)",
    paddingBlock: space.s2,
  },
  headLine: { minWidth: 0, flex: 1, alignSelf: "center" },
  overview: { paddingBlock: space.s3, paddingInline: "var(--layout-compact-gutter)" },
  group: { marginBottom: space.s4 },
  title: { marginBottom: space.s2 },
  readerSlot: {
    position: "relative",
    display: "flex",
    flexDirection: "column",
    minHeight: 0,
    flex: 1,
  },
  reader: {
    position: "absolute",
    inset: 0,
    display: "flex",
    flexDirection: "column",
    minHeight: 0,
    containerType: "inline-size",
    containerName: "conversation",
    "--transcript-jump-gap": space.s4,
  },
  // Keeping the layout box preserves WebKit's retained scroll position.
  inactiveReader: { visibility: "hidden" },
  evidence: { marginBottom: space.s6 },
  summary: {
    display: "flex",
    flexDirection: "column",
    alignSelf: "stretch",
    gap: space.s0_5,
    minWidth: 0,
    minHeight: 0,
    flex: 1,
  },
  identity: {
    flex: "0 1 auto",
    minWidth: 0,
    // Keep the scrollport's focus perimeter outside the name/status reading edge.
    marginInline: `calc(-1 * ${space.s0_5})`,
  },
  name: {
    display: "block",
    overflowWrap: "anywhere",
    textWrap: "balance",
    fontWeight: weight.medium,
  },
});

function subagentOpener(container: HTMLElement, runId: string) {
  const row = [...container.querySelectorAll<HTMLElement>('[data-slot="delegated-run-link"]')].find(
    (node) => node.dataset.runId === runId && !node.closest("[hidden], [inert]"),
  );
  return row?.querySelector<HTMLButtonElement>("button") ?? null;
}

interface SubagentPanelLayoutProps {
  panel: RefObject<HTMLDivElement | null>;
  back: RefObject<HTMLButtonElement | null>;
  sessionId: string;
  selectedId: string | null;
  visible: boolean;
  label: string;
  children: ReactNode;
}

interface RetiredSubagentControl {
  source: HTMLElement;
  container: HTMLElement;
  runId: string | null;
}

class SubagentPanelLayout extends Component<
  SubagentPanelLayoutProps,
  unknown,
  RetiredSubagentControl | null
> {
  // Completion can move a task between groups before layout effects can observe its focused node.
  override getSnapshotBeforeUpdate(
    previous: SubagentPanelLayoutProps,
  ): RetiredSubagentControl | null {
    if (
      !this.props.visible ||
      !previous.visible ||
      previous.sessionId !== this.props.sessionId ||
      previous.selectedId !== this.props.selectedId
    )
      return null;
    const panel = this.props.panel.current;
    const source = panel?.ownerDocument.activeElement;
    if (
      !panel ||
      !(source instanceof HTMLElement) ||
      !panel.contains(source) ||
      source.closest("[hidden], [inert]")
    )
      return null;
    const row = source.closest<HTMLElement>('[data-slot="delegated-run-link"]');
    if (row?.dataset.runId)
      return {
        source,
        container: source.closest<HTMLElement>('[data-slot="subagent-transcript"]') ?? panel,
        runId: row.dataset.runId,
      };
    return source.matches("[data-subagent-stop]")
      ? { source, container: panel, runId: null }
      : null;
  }

  override componentDidUpdate(
    _previous: SubagentPanelLayoutProps,
    _state: unknown,
    retired: RetiredSubagentControl | null,
  ) {
    const panel = this.props.panel.current;
    if (
      !retired ||
      !panel ||
      panel.closest("[hidden], [inert]") ||
      retired.source.isConnected ||
      panel.ownerDocument.activeElement !== panel.ownerDocument.body
    )
      return;
    if (retired.runId) {
      const container = retired.container.isConnected ? retired.container : panel;
      (subagentOpener(container, retired.runId) ?? panel).focus();
    } else this.props.back.current?.focus({ preventScroll: true });
  }

  override render() {
    return (
      <AgentWorkspaceView ref={this.props.panel} tabIndex={-1} ariaLabel={this.props.label}>
        {this.props.children}
      </AgentWorkspaceView>
    );
  }
}

export function SubagentsPanel() {
  const workspaceAccess = useWorkspace();

  const t = useT();
  const sessionId = useActiveSessionId();
  const visible = useContentVisible();
  const summaryId = useId();
  const rows = useActiveConversationRows();
  const entries = useMemo(() => subagentEntries(rows), [rows]);
  const selectedId = useWorkspaceSubagentRunId();
  const inspection = useWorkspaceInspection();
  const selected = entries.find(({ narrative }) => narrative.run.id === selectedId);
  const [visitedRunIds, setVisitedRunIds] = useState<ReadonlySet<string>>(() => new Set());
  if (visible && selected && !visitedRunIds.has(selected.narrative.run.id)) {
    setVisitedRunIds(new Set(visitedRunIds).add(selected.narrative.run.id));
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
    if (
      !visible ||
      fromRun === selectedId ||
      (fromInspection &&
        fromInspection.previous === inspection &&
        fromInspection.returnSubagentRunId === selectedId)
    )
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
      (subagentOpener(panel, fromRun) ?? panel).focus();
    }
  }, [selectedId, inspection, visible]);
  const summary = selected
    ? delegatedRunSummary(
        t,
        selected.narrative.run,
        selected.ordinal,
        selected.siblingCount,
        selected.taskLabel,
      )
    : null;
  const running = entries.filter(({ narrative }) => agentRunIsRunning(narrative.run)).length;
  return (
    <SubagentPanelLayout
      panel={panelRef}
      back={backRef}
      sessionId={sessionId}
      selectedId={selectedId}
      visible={visible}
      label={t("subagents.title")}
    >
      <header data-slot="subagent-header" {...stylex.props(styles.header)}>
        {selectedId && (
          <IconButton
            ref={backRef}
            icon={iconGlyph("chevron-left")}
            size="sm"
            aria-label={t("subagents.all")}
            data-chrome-focus=""
            onClick={() => openWorkspaceSubagentRun(workspaceAccess, null)}
          />
        )}
        {summary ? (
          <div {...stylex.props(styles.summary)}>
            <ScrollArea
              tabIndex={0}
              role="group"
              aria-label={summary.label}
              className={stylex.props(styles.identity).className}
            >
              <span
                id={summaryId}
                data-slot="subagent-identity"
                {...stylex.props(styles.name, typeStep.uiSm)}
              >
                {summary.label}
              </span>
            </ScrollArea>
            <span {...stylex.props(vocab.line, vocab.hold, toneInk[summary.ink], typeStep.uiXs)}>
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
        {selected && summary?.cancelable && (
          <SubagentStopButton run={selected.narrative.run} summaryId={summaryId} />
        )}
      </header>
      <div {...stylex.props(styles.readerSlot)}>
        <Activity
          key={entries[0]?.narrative.run.sessionId}
          mode={selectedId ? "hidden" : "visible"}
        >
          <ScrollArea className={stylex.props(styles.overview).className}>
            {entries.length === 0 ? (
              <EmptyState icon={iconGlyph("bot")} title={t("subagents.empty")} />
            ) : (
              <div role="group" aria-label={t("subagents.title")} inert={Boolean(selectedId)}>
                <SubagentGroup
                  title={t("subagents.active")}
                  entries={entries.filter(({ narrative }) => !agentRunIsFinished(narrative.run))}
                />
                <SubagentGroup
                  title={t("subagents.finished")}
                  entries={entries.filter(({ narrative }) => agentRunIsFinished(narrative.run))}
                />
              </div>
            )}
          </ScrollArea>
        </Activity>
        {entries
          .filter(({ narrative }) => visitedRunIds.has(narrative.run.id))
          .sort((left, right) => left.narrative.run.id.localeCompare(right.narrative.run.id))
          .map((entry) => {
            const run = entry.narrative.run;
            const active = run.id === selectedId;
            return (
              <ContentVisibility visible={active} key={run.id}>
                <SubagentTranscript
                  active={active}
                  entry={entry}
                  summary={delegatedRunSummary(
                    t,
                    run,
                    entry.ordinal,
                    entry.siblingCount,
                    entry.taskLabel,
                  )}
                />
              </ContentVisibility>
            );
          })}
        {!selected && selectedId ? (
          <EmptyState
            icon={iconGlyph("alert")}
            tone="negative"
            title={t("subagents.unavailable")}
          />
        ) : null}
      </div>
    </SubagentPanelLayout>
  );
}

function SubagentGroup({ title, entries }: { title: string; entries: SubagentEntry[] }) {
  const workspaceAccess = useWorkspace();

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
          onOpen={() => openWorkspaceSubagentRun(workspaceAccess, narrative.run.id)}
        />
      ))}
    </div>
  );
}

function SubagentTranscript({
  entry,
  summary,
  active,
}: {
  entry: SubagentEntry;
  summary: DelegatedRunSummary;
  active: boolean;
}) {
  const t = useT();
  const reader = useRef<ReadingListHandle>(null);
  const [atEnd, setAtEnd] = useState(true);
  const expandedIds = useExpandedWorkspaceToolIds();
  const onExpandedChange = useSetWorkspaceToolExpanded();
  const ctx = useMemo<BlockCtx>(
    () => ({ sessionId: entry.narrative.run.sessionId, expandedIds, onExpandedChange }),
    [entry.narrative.run.sessionId, expandedIds, onExpandedChange],
  );
  const { narrative, facts } = entry;
  const rows = useMemo(() => delegatedTranscriptRows(narrative, facts), [narrative, facts]);
  const hasRunDetails = Boolean(summary.detail || narrative.run.outcome?.unresolvedEffects?.length);
  return (
    <div
      role="region"
      aria-label={summary.label}
      tabIndex={-1}
      data-slot="subagent-transcript"
      inert={!active}
      aria-hidden={!active || undefined}
      {...stylex.props(styles.reader, !active && styles.inactiveReader)}
    >
      <MessageTranscript
        listRef={reader}
        onFollowChange={(_list, value) => setAtEnd(value)}
        header={
          <>
            {hasRunDetails && (
              <div {...stylex.props(rc.box, rc.gutter, styles.evidence)}>
                {summary.detail && (
                  <div {...stylex.props(typeStep.uiSm, toneInk[summary.ink])}>
                    <RecordBody>{summary.detail}</RecordBody>
                  </div>
                )}
                <UnresolvedRunEffects effects={narrative.run.outcome?.unresolvedEffects} />
              </div>
            )}
            {rows.length === 0 && !hasRunDetails && (
              <p {...stylex.props(rc.box, rc.gutter, vocab.muted, typeStep.uiSm)}>
                {t("agent.runTree.material.empty")}
              </p>
            )}
          </>
        }
        rows={rows}
        ctx={ctx}
        sessionId={narrative.run.sessionId}
        isRunning={agentRunIsRunning(narrative.run)}
      />
      <AgentJumpToBottom
        visible={!atEnd}
        onClick={() => {
          reader.current?.getScrollableNode()?.focus({ preventScroll: true });
          void reader.current?.scrollToEnd({ animated: false });
        }}
        placement="reader-footer"
      />
    </div>
  );
}
