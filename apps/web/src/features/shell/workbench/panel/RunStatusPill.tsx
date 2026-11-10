import * as stylex from "@stylexjs/stylex";
import { AgentStatusPill } from "@/ui/agent/index";
import { durationText } from "@/lib/format";
import { useCurrentRootMaterial } from "@/features/agent/public/run";
import { useElapsedMillis } from "@/lib/useElapsedMillis";
import { useT } from "@/lib/i18n/index";
import { Tooltip, vocab } from "@flame/ui";
const styles = stylex.create({
  trigger: { display: "flex", minWidth: 0, flexShrink: 1 },
});

export function RunStatusPill() {
  const t = useT();
  const { running, startedAt } = useCurrentRootMaterial();
  const elapsed = useElapsedMillis(running ? startedAt : null);
  if (!running) return null;

  const runningLabel = t("session.status.running");
  const label =
    startedAt === null
      ? runningLabel
      : `${runningLabel} · ${durationText(t, startedAt, startedAt + elapsed)}`;
  return (
    <Tooltip label={label}>
      <span data-slot="run-status" {...stylex.props(styles.trigger)}>
        <AgentStatusPill tone="running">
          <span {...stylex.props(vocab.figures, vocab.truncate)}>{label}</span>
        </AgentStatusPill>
      </span>
    </Tooltip>
  );
}
