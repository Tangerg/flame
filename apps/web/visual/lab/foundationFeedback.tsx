import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { Bell } from "@flame/icons/react";
import { Badge, Button, EmptyState, ProgressBar, Segmented, Slider } from "@flame/ui";
import { foundationMessages } from "./foundationMessages";
import { foundationStyles as styles } from "./foundationStyles";
import { foundationTones, foundationViews as views } from "./foundationVariants";
import type { LabExampleProps } from "./story";

const INITIAL_PROGRESS = 40;
const STEPPED_INCREMENT = 10;
const PROGRESS_WEIGHTS = ["bar", "row", "seam"] as const;

export function SegmentedExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const options = [
    { value: "trace", label: copy.trace },
    { value: "metrics", label: copy.metrics },
    { value: "logs", label: copy.logs },
  ] as const;
  const [value, setValue] = useState<(typeof options)[number]["value"]>("trace");
  return (
    <Segmented
      value={value}
      options={options}
      ariaLabel={copy.diagnostics}
      disabled={variant === views.disabled.id}
      onChange={(next) => {
        setValue(next);
        observe("valueChange", next);
      }}
    />
  );
}

export function SliderExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [value, setValue] = useState(INITIAL_PROGRESS);
  return (
    <div {...stylex.props(styles.stack)}>
      <Slider
        value={value}
        step={variant === views.stepped.id ? STEPPED_INCREMENT : 1}
        ariaLabel={copy.volume}
        onValueChange={(next) => {
          setValue(next);
          observe("valueChange", next);
        }}
      />
      <output {...stylex.props(styles.value)} aria-label={copy.value}>
        {value}
      </output>
    </div>
  );
}

export function BadgeExample({ locale }: LabExampleProps) {
  const copy = foundationMessages[locale];
  return (
    <div {...stylex.props(styles.row)}>
      {foundationTones.map((tone) => (
        <Badge key={tone} tone={tone}>
          {copy[tone]}
        </Badge>
      ))}
    </div>
  );
}

export function ProgressExample({ locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [value, setValue] = useState(INITIAL_PROGRESS);
  return (
    <div {...stylex.props(styles.stack)}>
      <Slider
        value={value}
        ariaLabel={copy.value}
        onValueChange={(next) => {
          setValue(next);
          observe("valueChange", next);
        }}
      />
      {PROGRESS_WEIGHTS.map((weight) => (
        <div key={weight} {...stylex.props(styles.stack)}>
          <span {...stylex.props(styles.label)}>{weight}</span>
          <ProgressBar
            value={value}
            label={copy.progress}
            labelVisibility="visible"
            locale={locale}
            weight={weight}
          />
        </div>
      ))}
    </div>
  );
}

export function EmptyStateExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [created, setCreated] = useState(false);
  if (created)
    return (
      <div {...stylex.props(styles.stack)}>
        <p role="status">{copy.exampleAdded}</p>
        <Button
          variant="outline"
          onClick={() => {
            setCreated(false);
            observe("reset");
          }}
        >
          {copy.reset}
        </Button>
      </div>
    );
  return (
    <EmptyState
      icon={Bell}
      title={copy.nothingHere}
      sub={copy.emptyDescription}
      size={variant === views.compact.id ? "compact" : "comfortable"}
      action={
        <Button
          variant="primary"
          onClick={() => {
            setCreated(true);
            observe("create");
          }}
        >
          {copy.addExample}
        </Button>
      }
    />
  );
}
