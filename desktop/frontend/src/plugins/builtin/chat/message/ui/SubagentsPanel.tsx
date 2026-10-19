import { iconGlyph } from "@/ui/icons";
import * as stylex from "@stylexjs/stylex";
import { useLayoutEffect, useMemo, useRef } from "react";
import { useStickToBottom } from "use-stick-to-bottom";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import {
  useActiveConversationRows,
  steerInputFeedback,
} from "@/plugins/builtin/agent/public/conversation";
import { MessageContext } from "@/plugins/sdk/messageContext";
import {
  openWorkspaceSubagentRun,
  useWorkspaceSubagentRunId,
  useExpandedWorkspaceToolIds,
  useToggleWorkspaceTool,
} from "@/plugins/builtin/workspace/public/navigation";
import { ScrollArea, SectionLabel, StatusDot } from "@/ui";
import { EmptyState, IconButton, RecordBody, toneInk, vocab } from "@flame/ui";
import { AgentWorkspaceView } from "@/ui/agent";
import { corner, face, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
import { subagentEntries, type SubagentEntry } from "../application/subagents";
import { delegatedRunSummary, type DelegatedRunSummary } from "../application/delegatedRunSummary";
import { DelegatedRunLink } from "./DelegatedRunLink";
import { renderMessageBlocks, type BlockCtx } from "./BlockRenderer";
import { MESSAGE_CONTENT_CLASS } from "./messageContent";
import { messageStyles as ms } from "./messageStyles";
import { UnresolvedRunEffects } from "./UnresolvedRunEffects";
import { SteerInputStatus } from "./SteerInputStatus";
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
  transcript: { display: "flex", flexDirection: "column", gap: space.s5, minWidth: 0 },
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
  const panelRef = useRef<HTMLDivElement>(null);
  const backRef = useRef<HTMLButtonElement>(null);
  const previousRun = useRef<string | null>(null);
  useLayoutEffect(() => {
    const fromRun = previousRun.current;
    previousRun.current = selectedId;
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
  }, [selectedId]);
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
            aria-label={t("subagents.back")}
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
      {selected && summary ? (
        <SubagentTranscript key={selectedId} entry={selected} summary={summary} />
      ) : (
        <ScrollArea className={stylex.props(styles.scroller).className}>
          {selectedId ? (
            <EmptyState icon={iconGlyph("bot")} title={t("subagents.unavailable")} />
          ) : entries.length === 0 ? (
            <EmptyState icon={iconGlyph("bot")} title={t("subagents.empty")} />
          ) : (
            <>
              <SubagentGroup
                title={t("subagents.active")}
                entries={entries.filter(({ narrative }) => narrative.run.status !== "finished")}
              />
              <SubagentGroup
                title={t("subagents.finished")}
                entries={entries.filter(({ narrative }) => narrative.run.status === "finished")}
              />
            </>
          )}
        </ScrollArea>
      )}
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
  const { scrollRef, contentRef } = useStickToBottom({ initial: "instant", resize: "instant" });
  const expandedIds = useExpandedWorkspaceToolIds();
  const onToggleExpand = useToggleWorkspaceTool();
  const ctx: BlockCtx = { expandedIds, onToggleExpand, textReveal: "instant" };
  const { narrative, facts } = entry;
  return (
    <ScrollArea ref={scrollRef} className={stylex.props(styles.scroller).className}>
      <div
        ref={contentRef}
        role="region"
        aria-label={summary.label}
        {...stylex.props(styles.transcript)}
      >
        {summary.detail && (
          <div {...stylex.props(typeStep.uiSm, toneInk[summary.ink])}>
            <RecordBody>{summary.detail}</RecordBody>
          </div>
        )}
        <UnresolvedRunEffects effects={narrative.run.outcome?.unresolvedEffects} />
        {narrative.messages.length === 0 && (
          <p {...stylex.props(vocab.muted, typeStep.uiSm)}>{t("agent.runTree.material.empty")}</p>
        )}
        {narrative.messages.map((message) => {
          const isUser = message.role === "user";
          return (
            <MessageContext.Provider
              key={message.id}
              value={{ sessionId: narrative.run.sessionId, message }}
            >
              <div {...stylex.props(ms.column, isUser && ms.columnUser)}>
                {isUser && (
                  <SteerInputStatus feedback={steerInputFeedback(message)} messageId={message.id} />
                )}
                <div
                  data-user-message-bubble={isUser ? "" : undefined}
                  className={cn(
                    MESSAGE_CONTENT_CLASS,
                    stylex.props(
                      ms.body,
                      typeStep.prose,
                      isUser && ms.bubble,
                      isUser && corner.bubble,
                    ).className,
                  )}
                >
                  {renderMessageBlocks({ message, facts }, ctx)}
                </div>
              </div>
            </MessageContext.Provider>
          );
        })}
      </div>
    </ScrollArea>
  );
}
