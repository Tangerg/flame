import * as stylex from "@stylexjs/stylex";
import { AgentStatusPill } from "@/ui/agent/index";
import { durationText } from "@/lib/format";
import { useCurrentRootMaterial } from "@/features/agent/public/run";
import { useElapsedMillis } from "@/features/shell/workbench/panel/useElapsedMillis";
import { useT } from "@/lib/i18n/index";
import { vocab } from "@flame/ui";
export function RunStatusPill() {
  const t = useT();
  const { running, startedAt } = useCurrentRootMaterial();
  const elapsed = useElapsedMillis(running ? startedAt : null);
  if (!running) return null;

  const label = t("session.status.running");
  return (
    <AgentStatusPill tone="running">
      <span {...stylex.props(vocab.figures)}>
        {startedAt === null
          ? label
          : `${label} · ${durationText(t, startedAt, startedAt + elapsed)}`}
      </span>
    </AgentStatusPill>
  );
}
