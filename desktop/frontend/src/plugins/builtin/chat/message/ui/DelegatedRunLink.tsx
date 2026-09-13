import * as stylex from "@stylexjs/stylex";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { Icon, IconButton, Pressable, StatusDot, toneInk, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { cancelSessionRun } from "@/plugins/builtin/agent/public/run";
import { openWorkspaceSubagentRun } from "@/plugins/builtin/workspace/public/navigation";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { radius, space, type as typeStep } from "@/styles/tokens.stylex";
import { delegatedRunSummary } from "../application/delegatedRunSummary";

const styles = stylex.create({
  row: { display: "flex", alignItems: "center", gap: space.s1, minWidth: 0 },
  link: {
    display: "flex",
    alignItems: "center",
    gap: space.s2,
    minWidth: 0,
    flex: 1,
    textAlign: "start",
    paddingBlock: space.s2,
    paddingInline: space.s2,
    borderRadius: radius.row,
  },
  detail: { display: "flex", flexDirection: "column", gap: space.s0_5, minWidth: 0, flex: 1 },
});

export function DelegatedRunLink({
  run,
  ordinal,
  siblingCount,
}: {
  run: AgentRunView;
  ordinal: number;
  siblingCount: number;
}) {
  const t = useT();
  const available = useRuntimeCommandsAvailable();
  const model = delegatedRunSummary(t, run, ordinal, siblingCount);
  return (
    <div data-slot="delegated-run-link" data-run-id={run.id} {...stylex.props(styles.row)}>
      <Pressable
        onClick={() => openWorkspaceSubagentRun(run.id)}
        className={stylex.props(styles.link).className}
      >
        <Icon name="bot" size="sm" />
        <span {...stylex.props(styles.detail)}>
          <span {...stylex.props(vocab.line, typeStep.uiSm)}>
            <span>{model.label}</span>
            <StatusDot tone={model.dotTone} />
            <span {...stylex.props(toneInk[model.ink])}>{model.statusLabel}</span>
          </span>
          {model.detail && (
            <span {...stylex.props(vocab.muted, vocab.truncate, typeStep.uiXs)}>
              {model.detail}
            </span>
          )}
        </span>
        <Icon name="panel-r" size="xs" />
      </Pressable>
      {model.cancelable && (
        <IconButton
          icon="stop"
          size="sm"
          quiet
          disabled={!available}
          title={t("agent.runTree.action.cancel")}
          onClick={() => cancelSessionRun({ sessionId: run.sessionId, runId: run.id })}
        />
      )}
    </div>
  );
}
