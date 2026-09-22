import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import { useModels } from "@/plugins/builtin/settings/providers/public/queries";
import { Button, DropdownMenu, Icon, SelectTrigger, providerDisplayName, vocab } from "@/ui";
import type { ScheduleModelSelection } from "../application/scheduleConfig";
import { settingStyles as ss } from "../../kit/settingStyles";
import { type as typeStep } from "@/styles/tokens.stylex";

const styles = stylex.create({ trigger: { maxWidth: "100%" } });

export function ScheduleModelFields({
  selection,
  onChange,
}: {
  selection: ScheduleModelSelection | null;
  onChange: (selection: ScheduleModelSelection | null) => void;
}) {
  const t = useT();
  const { data: models = [], isLoading, isError, refetch } = useModels();
  const selected = models.find(
    (model) => model.provider === selection?.provider && model.id === selection.model,
  );
  const label = selection
    ? `${providerDisplayName(selection.provider)} / ${selected?.label ?? selection.model}`
    : t("schedules.model.default");
  const levels = selected?.reasoningLevels ?? [];

  return (
    <div {...stylex.props(ss.stack, vocab.min)}>
      <div {...stylex.props(ss.lineWrap)}>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger
            render={
              <SelectTrigger
                label={label}
                title={label}
                aria-label={t("composer.switchModel")}
                className={stylex.props(styles.trigger).className}
              />
            }
          />
          <DropdownMenu.Content align="start">
            <DropdownMenu.Item onClick={() => onChange(null)} layout="pickPlain">
              <span>{t("schedules.model.default")}</span>
              {!selection && <Icon name="check" size="xs" />}
            </DropdownMenu.Item>
            {models
              .filter((model) => !model.deprecated)
              .map((model) => (
                <DropdownMenu.Item
                  key={JSON.stringify([model.provider, model.id])}
                  onClick={() => {
                    if (selected !== model) onChange({ provider: model.provider, model: model.id });
                  }}
                  layout="pickPlain"
                >
                  <span {...stylex.props(vocab.truncate)}>
                    {providerDisplayName(model.provider)} / {model.label}
                  </span>
                  {selected === model && <Icon name="check" size="xs" />}
                </DropdownMenu.Item>
              ))}
          </DropdownMenu.Content>
        </DropdownMenu.Root>
        {selection && (levels.length > 0 || selection.reasoningEffort) && (
          <DropdownMenu.Root>
            <DropdownMenu.Trigger
              render={
                <SelectTrigger
                  label={selection.reasoningEffort ?? t("schedules.reasoning.default")}
                  aria-label={t("composer.switchReasoningEffort")}
                  className={stylex.props(styles.trigger).className}
                />
              }
            />
            <DropdownMenu.Content align="start">
              <DropdownMenu.Item
                onClick={() => onChange({ provider: selection.provider, model: selection.model })}
                layout="pickPlain"
              >
                <span>{t("schedules.reasoning.default")}</span>
                {!selection.reasoningEffort && <Icon name="check" size="xs" />}
              </DropdownMenu.Item>
              {levels.map((effort) => (
                <DropdownMenu.Item
                  key={effort}
                  onClick={() => onChange({ ...selection, reasoningEffort: effort })}
                  layout="pickPlain"
                >
                  <span>{effort}</span>
                  {selection.reasoningEffort === effort && <Icon name="check" size="xs" />}
                </DropdownMenu.Item>
              ))}
            </DropdownMenu.Content>
          </DropdownMenu.Root>
        )}
      </div>
      <p {...stylex.props(ss.hint, typeStep.uiSm)}>{t("schedules.model.hint")}</p>
      {isLoading && <p {...stylex.props(ss.hint, typeStep.uiSm)}>{t("common.loading")}</p>}
      {isError ? (
        <div {...stylex.props(ss.lineWrap)}>
          <span {...stylex.props(vocab.negative, typeStep.uiSm)}>
            {t("providers.models.error")}
          </span>
          <Button variant="outline" size="xs" onClick={() => void refetch()}>
            {t("common.retry")}
          </Button>
        </div>
      ) : selection && !selected && !isLoading ? (
        <p {...stylex.props(ss.hint, typeStep.uiSm)}>{t("schedules.model.unavailable")}</p>
      ) : null}
    </div>
  );
}
