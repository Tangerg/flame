import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { Popover } from "@flame/ui";
import { AgentComposerChip, AgentRow } from "@/ui/agent/index";
import { ProviderIcon } from "@/ui/atoms/provider-icon";
import { businessMessages as messages } from "./businessMessages";
import { businessStyles as styles } from "./businessStyles";
import { businessVariant, composerVariants } from "./businessVariants";
import { localize, type LabExampleProps } from "./story";

const SAMPLE_MODELS = [
  { id: "local-preview", provider: "openai", label: messages.samples.localModel },
  { id: "reasoning-preview", provider: "deepseek", label: messages.samples.reasoningModel },
] as const;

export function ComposerChipExample({ variant, locale, observe }: LabExampleProps) {
  const scenario = businessVariant(composerVariants, variant);
  const [selected, setSelected] = useState<(typeof SAMPLE_MODELS)[number]>(SAMPLE_MODELS[0]);
  const [open, setOpen] = useState(false);
  const label = localize(scenario.longLabel ? messages.samples.longModel : selected.label, locale);
  return (
    <div {...stylex.props(styles.stack)}>
      <div {...stylex.props(styles.composer, scenario.compact && styles.compactComposer)}>
        <Popover.Root
          open={open}
          onOpenChange={(next) => {
            setOpen(next);
            observe("composer.popover", next);
          }}
        >
          <Popover.Trigger
            render={
              <AgentComposerChip
                leading={<ProviderIcon provider={selected.provider} />}
                label={label}
                aria-label={label}
                disabled={scenario.disabled}
                shrink="gives"
                labelVisibility={scenario.compact ? "wide" : "always"}
              />
            }
          />
          <Popover.Content
            aria-label={localize(messages.samples.modelChoice, locale)}
            align="start"
          >
            <div {...stylex.props(styles.popup)}>
              {SAMPLE_MODELS.map((model) => (
                <AgentRow
                  key={model.id}
                  active={selected.id === model.id}
                  aria-pressed={selected.id === model.id}
                  onClick={() => {
                    setSelected(model);
                    setOpen(false);
                    observe("composer.model.select", model.id);
                  }}
                >
                  {localize(model.label, locale)}
                </AgentRow>
              ))}
            </div>
          </Popover.Content>
        </Popover.Root>
      </div>
      <output
        aria-label={localize(messages.samples.selected, locale)}
        {...stylex.props(styles.caption)}
      >
        {selected.id}
      </output>
    </div>
  );
}
