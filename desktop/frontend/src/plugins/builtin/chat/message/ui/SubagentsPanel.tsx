import * as stylex from "@stylexjs/stylex";
import { useMemo } from "react";
import { useStickToBottom } from "use-stick-to-bottom";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { useActiveConversationRows } from "@/plugins/builtin/agent/public/conversation";
import { MessageContext } from "@/plugins/sdk/messageContext";
import {
  openWorkspaceSubagentRun,
  useWorkspaceSubagentRunId,
  useExpandedWorkspaceToolIds,
  useToggleWorkspaceTool,
} from "@/plugins/builtin/workspace/public/navigation";
import {
  EmptyState,
  Icon,
  IconButton,
  SectionLabel,
  StatusDot,
  TextButton,
  toneInk,
  vocab,
} from "@/ui";
import { cancelSessionRun } from "@/plugins/builtin/agent/public/run";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { radius, space, type as typeStep } from "@/styles/tokens.stylex";
import { subagentEntries, type SubagentEntry } from "../application/subagents";
import { delegatedRunSummary } from "../application/delegatedRunSummary";
import { DelegatedRunLink } from "./DelegatedRunLink";
import { renderMessageBlocks, type BlockCtx } from "./BlockRenderer";
import { MESSAGE_CONTENT_CLASS } from "./messageContent";
import { messageStyles as ms } from "./messageStyles";

const styles = stylex.create({
  panel: { display: "flex", flexDirection: "column", height: "100%", minHeight: 0 },
  header: {
    display: "flex",
    alignItems: "center",
    gap: space.s2,
    padding: space.s3,
    flexShrink: 0,
  },
  scroller: {
    flex: 1,
    minHeight: 0,
    overflowY: "auto",
    overscrollBehavior: "contain",
    padding: space.s3,
  },
  group: { marginBottom: space.s4 },
  title: { marginBottom: space.s2 },
  transcript: { display: "flex", flexDirection: "column", gap: space.s5, minWidth: 0 },
  status: { display: "flex", alignItems: "center", gap: space.s2 },
  summary: { flex: 1, minWidth: 0 },
  user: { borderRadius: radius.lg, padding: space.s3 },
});

export function SubagentsPanel() {
  const t = useT();
  const rows = useActiveConversationRows();
  const entries = useMemo(() => subagentEntries(rows), [rows]);
  const selectedId = useWorkspaceSubagentRunId();
  const selected = entries.find(({ narrative }) => narrative.run.id === selectedId);
  return (
    <section aria-label={t("subagents.title")} {...stylex.props(styles.panel)}>
      <header {...stylex.props(styles.header)}>
        {selectedId ? (
          <TextButton onClick={() => openWorkspaceSubagentRun(null)}>
            <Icon name="chevron-left" size="sm" />
            {t("subagents.title")}
          </TextButton>
        ) : (
          <SectionLabel>{t("subagents.title")}</SectionLabel>
        )}
      </header>
      {selected ? (
        <SubagentTranscript key={selectedId} entry={selected} />
      ) : (
        <div {...stylex.props(styles.scroller)}>
          {selectedId ? (
            <EmptyState icon="bot" title={t("subagents.unavailable")} />
          ) : entries.length === 0 ? (
            <EmptyState icon="bot" title={t("subagents.empty")} />
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
        </div>
      )}
    </section>
  );
}

function SubagentGroup({ title, entries }: { title: string; entries: SubagentEntry[] }) {
  if (entries.length === 0) return null;
  return (
    <div {...stylex.props(styles.group)}>
      <SectionLabel className={stylex.props(styles.title).className}>
        {title} · {entries.length}
      </SectionLabel>
      {entries.map(({ narrative, ordinal, siblingCount }) => (
        <DelegatedRunLink
          key={narrative.run.id}
          run={narrative.run}
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
  const { narrative, facts, ordinal, siblingCount } = entry;
  const model = delegatedRunSummary(t, narrative.run, ordinal, siblingCount);
  return (
    <div ref={scrollRef} {...stylex.props(styles.scroller)}>
      <div
        ref={contentRef}
        role="region"
        aria-label={model.label}
        {...stylex.props(styles.transcript)}
      >
        <div {...stylex.props(styles.status, typeStep.uiSm)}>
          <Icon name="bot" size="sm" />
          <span {...stylex.props(styles.summary)}>{model.label}</span>
          <StatusDot tone={model.dotTone} />
          <span {...stylex.props(toneInk[model.ink])}>{model.statusLabel}</span>
          {model.cancelable && (
            <IconButton
              icon="stop"
              size="sm"
              quiet
              disabled={!available}
              title={t("agent.runTree.action.cancel")}
              onClick={() =>
                cancelSessionRun({ sessionId: narrative.run.sessionId, runId: narrative.run.id })
              }
            />
          )}
        </div>
        {narrative.messages.length === 0 && (
          <p {...stylex.props(vocab.muted, typeStep.uiSm)}>{t("agent.runTree.material.empty")}</p>
        )}
        {narrative.messages.map((message) => (
          <MessageContext.Provider
            key={message.id}
            value={{ sessionId: narrative.run.sessionId, message }}
          >
            <div
              className={cn(
                MESSAGE_CONTENT_CLASS,
                stylex.props(
                  typeStep.prose,
                  message.role === "user" && ms.delegatedBubble,
                  message.role === "user" && styles.user,
                ).className,
              )}
            >
              {renderMessageBlocks({ message, facts }, ctx)}
            </div>
          </MessageContext.Provider>
        ))}
      </div>
    </div>
  );
}
