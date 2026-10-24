import { tupleKey } from "@/lib/tupleKey";
import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n/index";
import { useCodeActionLabels } from "@/ui/index";
import { type AgentUnresolvedEffect } from "@/features/agent/public/sessionView";
import {
  CodeBlock,
  Disclosure,
  Facts,
  RecordBody,
  RecordGroup,
  RecordSummary,
  vocab,
} from "@flame/ui";
import { space, type as typeStep } from "@flame/ui/tokens.stylex";
const styles = stylex.create({
  root: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    minWidth: 0,
    gap: space.s2,
  },
  explanation: { margin: 0 },
});

export function UnresolvedRunEffects({ effects }: { effects?: AgentUnresolvedEffect[] }) {
  const t = useT();
  const codeLabels = useCodeActionLabels();
  if (!effects?.length) return null;
  return (
    <div {...stylex.props(styles.root)}>
      <p {...stylex.props(styles.explanation, vocab.muted, typeStep.uiSm)}>
        {t("agent.unresolved.summary")}
      </p>
      <RecordGroup variant="list">
        {effects.map((effect, index) => (
          <Disclosure
            key={tupleKey(effect.processId, effect.effectId)}
            variant="row"
            title={
              <RecordSummary
                title={t("agent.unresolved.operation", { index: index + 1 })}
                description={effect.effectId}
              />
            }
          >
            <Facts
              values={[
                [t("agent.unresolved.processId"), effect.processId],
                [t("agent.unresolved.effectId"), effect.effectId],
                [t("agent.unresolved.cause"), effect.cause],
                [t("agent.unresolved.reason"), effect.reason],
              ]}
            />
            {effect.detail && <RecordBody>{effect.detail}</RecordBody>}
            {effect.output && (
              <>
                <p {...stylex.props(styles.explanation, vocab.muted, typeStep.uiSm)}>
                  {t("agent.unresolved.output")}
                </p>
                <CodeBlock label={t("agent.unresolved.output")} labels={codeLabels}>
                  {effect.output}
                </CodeBlock>
              </>
            )}
          </Disclosure>
        ))}
      </RecordGroup>
    </div>
  );
}
