import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { toolActivityStatus } from "@/plugins/builtin/agent/public/messagePresentation";
import { useT } from "@/lib/i18n";
import { StatusDot } from "@/ui";
import { toneInk, vocab } from "@flame/ui";
import { space, type } from "@flame/ui/tokens.stylex";

const styles = stylex.create({
  summary: { display: "grid", minWidth: 0, gap: space.s0_5, paddingBlock: space.s0_5 },
  label: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  statuses: { display: "flex", flexWrap: "wrap", gap: space.s2, whiteSpace: "normal" },
  status: { display: "inline-flex", alignItems: "center", gap: space.s1, whiteSpace: "nowrap" },
});

export function ToolActivitySummary({
  label,
  tools,
}: {
  label: string;
  tools: readonly ToolCall[];
}) {
  const t = useT();
  const status = toolActivityStatus(tools);
  return (
    <span {...stylex.props(styles.summary)}>
      <span {...stylex.props(styles.label)}>{label}</span>
      {(status.running > 0 || status.issues > 0 || status.denied > 0) && (
        <span {...stylex.props(styles.statuses, type.uiXs)}>
          {status.running > 0 && (
            <span {...stylex.props(styles.status, toneInk.accent)}>
              <StatusDot tone="running" />
              {t("tools.group.running", { count: status.running })}
            </span>
          )}
          {status.issues > 0 && (
            <span {...stylex.props(styles.status, toneInk.negative)}>
              {t("tools.group.issues", { count: status.issues })}
            </span>
          )}
          {status.denied > 0 && (
            <span {...stylex.props(styles.status, vocab.muted)}>
              {t("tools.group.denied", { count: status.denied })}
            </span>
          )}
        </span>
      )}
    </span>
  );
}
