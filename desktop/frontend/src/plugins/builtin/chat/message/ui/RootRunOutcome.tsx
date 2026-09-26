import * as stylex from "@stylexjs/stylex";
import { Icon, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import type { CurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import { color, space, type as typeStep } from "@/styles/tokens.stylex";
import { UnresolvedRunEffects } from "./UnresolvedRunEffects";

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
  if (!outcome) return null;

  return (
    <>
      {outcome.type === "canceled" && (
        <div {...stylex.props(ro.line, typeStep.uiSm)}>
          <Icon name="stop" size="xs" className={stylex.props(vocab.hold).className} />
          <span {...stylex.props(vocab.hold)}>{t("agent.runOutcome.canceled")}</span>
          {outcome.detail && (
            <span {...stylex.props(vocab.min, vocab.truncate)}>· {outcome.detail}</span>
          )}
        </div>
      )}
      <UnresolvedRunEffects effects={outcome.unresolvedEffects} />
    </>
  );
}
