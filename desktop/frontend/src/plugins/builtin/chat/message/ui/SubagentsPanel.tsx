import { iconGlyph } from "@/ui/icons";
import * as stylex from "@stylexjs/stylex";
import { useMemo } from "react";
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
import { Icon, ScrollArea, SectionLabel, StatusDot } from "@/ui";
import { EmptyState, IconButton, RecordSummary, toneInk, vocab } from "@flame/ui";
import { AgentSurfaceHeader, AgentWorkspaceView } from "@/ui/agent";
import { cancelSessionRun } from "@/plugins/builtin/agent/public/run";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { corner, face, space, type as typeStep } from "@flame/ui/tokens.stylex";
import { subagentEntries, type SubagentEntry } from "../application/subagents";
import { delegatedRunSummary } from "../application/delegatedRunSummary";
import { DelegatedRunLink } from "./DelegatedRunLink";
import { renderMessageBlocks, type BlockCtx } from "./BlockRenderer";
import { MESSAGE_CONTENT_CLASS } from "./messageContent";
import { messageStyles as ms } from "./messageStyles";
import { UnresolvedRunEffects } from "./UnresolvedRunEffects";
import { SteerInputStatus } from "./SteerInputStatus";

const styles = stylex.create({
  headLine: { minWidth: 0, flex: 1 },
  scroller: {
    paddingInline: "var(--reading-gutter-wide)",
    paddingBlock: space.s3,
  },
  group: { marginBottom: space.s4 },
  title: { marginBottom: space.s2 },
  transcript: { display: "flex", flexDirection: "column", gap: space.s5, minWidth: 0 },
  status: { display: "flex", alignItems: "flex-start", gap: space.s2 },
  summary: { minWidth: 0, flex: 1 },
  statusAction: { marginInlineStart: "auto" },
  detail: { whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
});

export function SubagentsPanel() {
  const t = useT();
  const rows = useActiveConversationRows();
  const entries = useMemo(() => subagentEntries(rows), [rows]);
  const selectedId = useWorkspaceSubagentRunId();
  const selected = entries.find(({ narrative }) => narrative.run.id === selectedId);
  const running = entries.filter(({ narrative }) => narrative.run.status === "running").length;
  return (
    <AgentWorkspaceView ariaLabel={t("subagents.title")}>
      <AgentSurfaceHeader>
        <span
          {...stylex.props(styles.headLine, vocab.truncate, vocab.muted, typeStep.uiSm, face.mono)}
        >
          {t("subagents.count", { count: entries.length })}
          {running > 0 && ` · ${t("subagents.running", { count: running })}`}
        </span>
        {selectedId && (
          <IconButton
            icon={iconGlyph("chevron-left")}
            size="sm"
            aria-label={t("subagents.title")}
            onClick={() => openWorkspaceSubagentRun(null)}
          />
        )}
      </AgentSurfaceHeader>
      {selected ? (
        <SubagentTranscript key={selectedId} entry={selected} />
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
                title={t("subagents.completed")}
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

function SubagentTranscript({ entry }: { entry: SubagentEntry }) {
  const t = useT();
  const available = useRuntimeCommandsAvailable();
  const { scrollRef, contentRef } = useStickToBottom({ initial: "instant", resize: "instant" });
  const expandedIds = useExpandedWorkspaceToolIds();
  const onToggleExpand = useToggleWorkspaceTool();
  const ctx: BlockCtx = { expandedIds, onToggleExpand, textReveal: "instant" };
  const { narrative, facts, ordinal, siblingCount, taskLabel } = entry;
  const model = delegatedRunSummary(t, narrative.run, ordinal, siblingCount, taskLabel);
  return (
    <ScrollArea ref={scrollRef} className={stylex.props(styles.scroller).className}>
      <div
        ref={contentRef}
        role="region"
        aria-label={model.label}
        {...stylex.props(styles.transcript)}
      >
        <div {...stylex.props(styles.status, typeStep.uiMd)}>
          <span {...stylex.props(vocab.firstLine)}>
            <Icon name="bot" size="sm" />
          </span>
          <span {...stylex.props(styles.summary)}>
            <RecordSummary
              title={<span title={model.label}>{model.label}</span>}
              status={
                <span {...stylex.props(vocab.line, vocab.hold, toneInk[model.ink])}>
                  <StatusDot tone={model.dotTone} />
                  {model.statusLabel}
                </span>
              }
            />
          </span>
          {model.cancelable && (
            <IconButton
              icon={iconGlyph("stop")}
              size="sm"
              quiet
              disabled={!available}
              className={stylex.props(styles.statusAction).className}
              aria-label={t("agent.runTree.action.cancel")}
              onClick={() =>
                cancelSessionRun({ sessionId: narrative.run.sessionId, runId: narrative.run.id })
              }
            />
          )}
        </div>
        {model.detail && (
          <p {...stylex.props(styles.detail, typeStep.uiSm, toneInk[model.ink])}>{model.detail}</p>
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
