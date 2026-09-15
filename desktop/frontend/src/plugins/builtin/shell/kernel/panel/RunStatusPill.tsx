import * as stylex from "@stylexjs/stylex";
import { AgentStatusPill } from "@/ui/agent";
import { durationText } from "@/lib/format";
import { useCurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import { useElapsedMillis } from "./useElapsedMillis";
import { useT } from "@/lib/i18n";
import { vocab } from "@/ui";

/**
 * That the turn is running, and how long it has been running for.
 *
 * Its own component because the duration re-reads once a second: the header around it must not
 * re-render on that tick. Wall clock on purpose — this is the wait as lived, including approval
 * pauses that a runtime-measured step duration deliberately excludes.
 */
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
