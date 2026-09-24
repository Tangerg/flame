import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import { reasoningEffortLabel } from "@/plugins/builtin/settings/providers/public/reasoningEffort";
import { DropdownMenu, Icon, vocab } from "@/ui";
import { AgentComposerChip } from "@/ui/agent";
import { corner, space, type as typeStep } from "@/styles/tokens.stylex";
import { useSetComposerModelPreference } from "../public/modelPreference";
import { useSelectedModelSelection } from "../public/selectedModel";
import { toolbarStyles } from "../toolbarStyles";

const styles = stylex.create({
  meter: {
    display: "inline-flex",
    height: "var(--icon-sm)",
    width: "var(--icon-sm)",
    flexShrink: 0,
    alignItems: "flex-end",
    justifyContent: "center",
    gap: "1.5px",
  },
  bar: { width: "2px", backgroundColor: "currentColor" },
  unlit: { opacity: 0.28 },
  row: { gap: space.s2 },
});

function EffortMeter({ index, count }: { index: number; count: number }) {
  const bars = Math.max(1, count - 1);
  return (
    <span aria-hidden {...stylex.props(styles.meter)}>
      {Array.from({ length: bars }, (_, bar) => (
        <span
          key={bar}
          {...stylex.props(styles.bar, corner.pill, bar >= index && styles.unlit)}
          style={{ height: `${Math.round(((bar + 1) / bars) * 100)}%` }}
        />
      ))}
    </span>
  );
}

export function ReasoningEffortPill() {
  const t = useT();
  const selection = useSelectedModelSelection();
  const setModel = useSetComposerModelPreference();
  if (!selection) return null;
  const { model } = selection;
  const levels = model.reasoningLevels;
  const current = model.reasoningLevelOrDefault(selection.reasoningEffort);
  if (current === undefined || levels.length === 0) return null;
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        render={
          <AgentComposerChip
            type="button"
            aria-label={t("composer.switchReasoningEffort")}
            variant="ghost"
            leading={<EffortMeter index={levels.indexOf(current)} count={levels.length} />}
            label={reasoningEffortLabel(current, t)}
            labelVisibility="wide"
          />
        }
      />
      <DropdownMenu.Content align="start" sideOffset={6}>
        <div aria-hidden {...stylex.props(toolbarStyles.menuHeading, typeStep.uiSm)}>
          {t("composer.reasoningEffort")}
        </div>
        {levels.map((effort, index) => (
          <DropdownMenu.Item
            key={effort}
            layout="pick"
            styles={styles.row}
            onClick={() =>
              setModel({
                kind: "explicit",
                provider: model.provider,
                model: model.id,
                reasoningEffort: effort,
              })
            }
          >
            <EffortMeter index={index} count={levels.length} />
            <span {...stylex.props(vocab.fill, vocab.truncate, typeStep.uiMd)}>
              {reasoningEffortLabel(effort, t)}
            </span>
            {effort === current && (
              <Icon name="check" size="xs" className={stylex.props(vocab.accent).className} />
            )}
          </DropdownMenu.Item>
        ))}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}
