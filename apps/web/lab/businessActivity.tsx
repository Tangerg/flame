import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { Slider } from "@flame/ui";
import { AgentActivityDisclosure } from "@/ui/agent/index";
import { businessMessages as messages } from "./businessMessages";
import { businessStyles as styles } from "./businessStyles";
import { activityVariants, businessVariant } from "./businessVariants";
import { localize, type LabExampleProps } from "./story";

const INITIAL_PROGRESS = 65;

export function ActivityExample({ variant, locale, observe }: LabExampleProps) {
  const scenario = businessVariant(activityVariants, variant);
  const [open, setOpen] = useState(false);
  const [progress, setProgress] = useState(INITIAL_PROGRESS);
  const progressLabel = localize(messages.samples.progress, locale);
  return (
    <div {...stylex.props(styles.stack)}>
      <AgentActivityDisclosure
        icon="folder"
        shell={scenario.shell}
        tone={scenario.tone}
        label={localize(messages.samples.activity, locale)}
        detail={localize(messages.samples.activityDetail, locale)}
        open={open}
        onToggle={() => {
          const next = !open;
          setOpen(next);
          observe("activity.toggle", next);
        }}
        progress={scenario.progress ? { value: progress, label: progressLabel } : undefined}
      >
        <div {...stylex.props(styles.reading)}>
          {localize(
            scenario.failed ? messages.samples.activityFailure : messages.samples.activityBody,
            locale,
          )}
        </div>
      </AgentActivityDisclosure>
      {scenario.progress && (
        <div {...stylex.props(styles.row)}>
          <Slider
            ariaLabel={progressLabel}
            value={progress}
            onValueChange={(value) => {
              setProgress(value);
              observe("activity.progress", value);
            }}
          />
          <output aria-label={progressLabel} {...stylex.props(styles.caption)}>
            {progress}%
          </output>
        </div>
      )}
    </div>
  );
}
