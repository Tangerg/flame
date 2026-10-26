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
import { Tabs as TabsPrimitive } from "@base-ui/react/tabs";

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
  list: { display: "contents" },
  tab: {
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
      ":is([data-active])": color.fg,
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
    <TabsPrimitive.Root
      data-slot="segmented"
      value={String(value)}
      onValueChange={(v) => {
        const opt = options.find((o) => String(o.value) === v);
        if (opt) onChange(opt.value);
      }}
      {...root}
      className={cn(root.className, className)}
    >
      <TabsPrimitive.List
        aria-label={ariaLabel}
        {...stylex.props(styles.list)}
        activateOnFocus
      >
        {options.map((opt) => (
          <TabsPrimitive.Tab
            key={String(opt.value)}
            value={String(opt.value)}
            disabled={disabled}
            {...stylex.props(styles.tab, typeStep.uiMd)}
          >
            {String(opt.value) === String(value) && (
              <ItemHighlight layoutId={chipId} variant="segment" />
            )}
            <span {...stylex.props(styles.label)}>{opt.label}</span>
          </TabsPrimitive.Tab>
        ))}
      </TabsPrimitive.List>
    </TabsPrimitive.Root>
  );
}
