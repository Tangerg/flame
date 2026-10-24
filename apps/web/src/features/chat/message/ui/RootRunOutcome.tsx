import * as stylex from "@stylexjs/stylex";
import { Icon } from "@/ui/index";
import { vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { type CurrentRootMaterial } from "@/features/agent/public/run";
import { color, space, type as typeStep } from "@flame/ui/tokens.stylex";
import { UnresolvedRunEffects } from "@/features/chat/message/ui/UnresolvedRunEffects";
const ro = stylex.create({
  line: {
    marginBlock: space.s2,
    display: "flex",
    minWidth: 0,
    alignItems: "flex-start",
    gap: space.s2,
    color: color.fgFaint,
  },
  text: {
    minWidth: 0,
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
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
          <span {...stylex.props(vocab.hold, vocab.firstLine)}>
            <Icon name="stop" size="xs" />
          </span>
          <span {...stylex.props(ro.text)}>
            <span>{t("agent.runOutcome.canceled")}</span>
            {outcome.detail && <span>{` · ${outcome.detail}`}</span>}
          </span>
        </div>
      )}
      <UnresolvedRunEffects effects={outcome.unresolvedEffects} />
    </>
  );
}
