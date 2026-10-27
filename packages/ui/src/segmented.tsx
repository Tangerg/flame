import { useId } from "react";
import { ItemHighlight } from "./item-highlight";
import * as stylex from "@stylexjs/stylex";
import { clsx as cn } from "clsx";
import {
  color,
  motion,
  radius,
  space,
  surface,
  type as typeStep,
  weight,
} from "./tokens.stylex";
import { Radio } from "@base-ui/react/radio";
import { RadioGroup } from "@base-ui/react/radio-group";

export interface SegmentedOption<T> {
  value: T;
  label: string;
}

const styles = stylex.create({
  root: {
    display: "inline-flex",
    width: "fit-content",
    maxWidth: "100%",
    flexWrap: "wrap",
    alignItems: "center",
    gap: space.s0_5,
    borderRadius: radius.segmented,
    padding: space.s0_5,
    backgroundColor: surface.hover,
    boxShadow: "var(--shadow-well)",
  },
  option: {
    position: "relative",
    isolation: "isolate",
    minHeight: "max(var(--control-height-xs), var(--control-target-min))",
    borderRadius: radius.segment,
    borderWidth: 0,
    backgroundColor: "transparent",
    paddingInline: space.s3,
    paddingBlock: space.s1,
    fontWeight: weight.medium,
    transitionProperty: "color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
    color: {
      default: color.fgMuted,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": color.fg,
      },
      ":is([data-checked])": color.fg,
    },
    opacity: {
      default: null,
      ':is(:disabled, [aria-disabled="true"], [data-disabled])':
        "var(--control-disabled-opacity)",
    },
  },
  label: { position: "relative" },
});

export function Segmented<T extends string | number>({
  value,
  options,
  onChange,
  ariaLabel,
  className,
  disabled,
}: {
  value: T;
  options: readonly SegmentedOption<T>[];
  onChange: (value: T) => void;
  ariaLabel: string;
  className?: string;
  disabled?: boolean;
}) {
  const chipId = useId();
  const root = stylex.props(styles.root);
  return (
    <RadioGroup
      data-slot="segmented"
      aria-label={ariaLabel}
      value={value}
      disabled={disabled}
      onValueChange={(v) => {
        const opt = options.find((o) => o.value === v);
        if (opt) onChange(opt.value);
      }}
      {...root}
      className={cn(root.className, className)}
    >
      {options.map((opt) => (
        <Radio.Root
          key={String(opt.value)}
          value={opt.value}
          nativeButton
          render={<button type="button" />}
          {...stylex.props(styles.option, typeStep.uiMd)}
        >
          {opt.value === value && (
            <ItemHighlight layoutId={chipId} variant="segment" />
          )}
          <span {...stylex.props(styles.label)}>{opt.label}</span>
        </Radio.Root>
      ))}
    </RadioGroup>
  );
}
