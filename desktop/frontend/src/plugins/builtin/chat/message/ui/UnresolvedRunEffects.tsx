import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import type { AgentUnresolvedEffect } from "@/plugins/sdk/types/agentSessionView";
import {
  CodeBlock,
  Disclosure,
  Facts,
  RecordBody,
  RecordGroup,
  RecordTitle,
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
  if (!effects?.length) return null;
  return (
    <div {...stylex.props(styles.root)}>
      <p {...stylex.props(styles.explanation, vocab.muted, typeStep.uiSm)}>
        {t("agent.unresolved.summary")}
      </p>
      <Disclosure title={t("agent.runOutcome.unresolvedEffects")}>
        {effects.map((effect, index) => (
          <RecordGroup key={JSON.stringify([effect.processId, effect.effectId])}>
            <RecordTitle>{t("agent.unresolved.operation", { index: index + 1 })}</RecordTitle>
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
                <CodeBlock label={t("agent.unresolved.output")}>{effect.output}</CodeBlock>
              </>
            )}
          </RecordGroup>
        ))}
      </Disclosure>
    </div>
  );
}
