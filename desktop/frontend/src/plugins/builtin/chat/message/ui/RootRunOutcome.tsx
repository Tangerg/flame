import * as stylex from "@stylexjs/stylex";
import type { IconName } from "@/ui";
import type { AgentRunOutcome } from "@/plugins/sdk/types/agentSessionView";
import { isAgentRunFailure } from "@/plugins/builtin/agent/public/viewState";
import { Icon } from "@/ui";
import { useT } from "@/lib/i18n";
import type { CurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import { color, space, type as typeStep } from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../chatStyles";

const ro = stylex.create({
  // A one-line receipt between turns: it owns the gap on both sides, like a banner.
  line: {
    marginBlock: space.s2,
    display: "flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s2,
    color: color.fgFaint,
  },
});

export function RootRunOutcome({ material }: { material: CurrentRootMaterial }) {
  const t = useT();
  const { outcome } = material;
  if (!outcome || outcome.type === "completed" || isAgentRunFailure(outcome)) return null;

  const face = CLOSE_FACE[outcome.type];
  const detail = outcome.detail;

  return (
    <div {...stylex.props(ro.line, typeStep.uiSm)}>
      <Icon name={face.icon} size="xs" className={stylex.props(ct.hold).className} />
      <span {...stylex.props(ct.hold)}>{t(face.labelKey)}</span>
      {detail && <span {...stylex.props(ct.min, ct.truncate)}>· {detail}</span>}
    </div>
  );
}

const CLOSE_FACE: Record<
  Exclude<AgentRunOutcome["type"], "completed" | "timedOut" | "failed" | "lost">,
  { icon: IconName; labelKey: string }
> = {
  canceled: { icon: "stop", labelKey: "agent.runOutcome.canceled" },
  maxSteps: { icon: "alert", labelKey: "agent.runOutcome.maxSteps" },
  maxBudget: { icon: "alert", labelKey: "agent.runOutcome.maxBudget" },
};
