import * as stylex from "@stylexjs/stylex";
import { type ToolActivityCounts } from "@/features/agent/public/messagePresentation";
import { useT } from "@/lib/i18n/index";
import { StatusDot } from "@/ui/index";
import { toneInk, vocab } from "@flame/ui";
import { space, type } from "@flame/ui/tokens.stylex";
const styles = stylex.create({
  statuses: { display: "flex", flexWrap: "wrap", gap: space.s2, whiteSpace: "normal" },
  status: { display: "inline-flex", alignItems: "center", gap: space.s1, whiteSpace: "nowrap" },
});

export function ToolActivityStatus({ status }: { status: ToolActivityCounts }) {
  const t = useT();
  return (
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
  );
}
