import * as stylex from "@stylexjs/stylex";
import { isAgentRunFailure } from "@/plugins/builtin/agent/public/viewState";
import { Icon, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import type { CurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import { color, space, type as typeStep } from "@/styles/tokens.stylex";

const ro = stylex.create({
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

  const detail = outcome.detail;

  return (
    <div {...stylex.props(ro.line, typeStep.uiSm)}>
      <Icon name="stop" size="xs" className={stylex.props(vocab.hold).className} />
      <span {...stylex.props(vocab.hold)}>{t("agent.runOutcome.canceled")}</span>
      {detail && <span {...stylex.props(vocab.min, vocab.truncate)}>· {detail}</span>}
    </div>
  );
}
