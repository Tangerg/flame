import * as stylex from "@stylexjs/stylex";
import { AgentStatusPill } from "@/ui/agent";
import { durationText } from "@/lib/format";
import { useCurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import { useElapsedMillis } from "./useElapsedMillis";
import { useT } from "@/lib/i18n";
import { vocab } from "@/ui";

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
