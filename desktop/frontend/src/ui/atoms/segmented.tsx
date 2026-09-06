import { useId } from "react";
import * as stylex from "@stylexjs/stylex";
import { motion as anim } from "motion/react";
import { cn } from "@/lib/classNames";
import { selectionTransition } from "@/lib/motion";
import { color, motion, radius, space, surface, type as typeStep } from "@/styles/tokens.stylex";
import { TabsPrimitive } from "@/ui/primitives";

export interface SegmentedOption<T> {
  value: T;
  label: string;
}

const styles = stylex.create({
  root: {
    display: "inline-flex",
    width: "fit-content",
    alignItems: "center",
    gap: space.s0_5,
    borderRadius: radius.segmented,
    padding: space.s0_5,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.field,
    backgroundColor: surface.sunken,
    boxShadow: "var(--shadow-well)",
  },
  // The list adds no box of its own: the root already IS the track, and a second box between
  // them would put the tabs one nesting step away from the padding that positions them.
  list: { display: "contents" },
  tab: {
    position: "relative",
    height: "var(--control-height-xs)",
    borderRadius: radius.segment,
    borderWidth: 0,
    backgroundColor: "transparent",
    paddingInline: space.s2,
    fontWeight: 500,
    transitionProperty: "color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeOut,
    color: {
      default: color.fgMuted,
      ":hover": color.fg,
      ":is([data-active])": color.fg,
    },
    outline: { default: null, ":focus-visible": "none" },
  },
  // The moving chip is a sibling behind the label rather than the tab's own background, so one
  // element can travel between tabs — a background cannot animate from one box to another.
  chip: {
    position: "absolute",
    inset: 0,
    borderRadius: radius.segment,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.field,
    backgroundColor: surface.canvas,
    boxShadow: "var(--shadow-raised-chip)",
  },
  label: { position: "relative" },
});

export function Segmented<T extends string | number>({
  value,
  options,
  onChange,
  ariaLabel,
  className,
}: {
  value: T;
  options: SegmentedOption<T>[];
  onChange: (value: T) => void;
  ariaLabel: string;
  className?: string;
}) {
  const chipId = useId();
  const root = stylex.props(styles.root);
  return (
    <TabsPrimitive.Root
      value={String(value)}
      onValueChange={(v) => {
        const opt = options.find((o) => String(o.value) === v);
        if (opt) onChange(opt.value);
      }}
      {...root}
      className={cn(root.className, className)}
    >
      <TabsPrimitive.List aria-label={ariaLabel} {...stylex.props(styles.list)} activateOnFocus>
        {options.map((opt) => (
          <TabsPrimitive.Tab
            key={String(opt.value)}
            value={String(opt.value)}
            {...stylex.props(styles.tab, typeStep.uiSm)}
          >
            {String(opt.value) === String(value) && (
              <anim.span
                aria-hidden
                layoutId={chipId}
                transition={selectionTransition}
                {...stylex.props(styles.chip)}
              />
            )}
            <span {...stylex.props(styles.label)}>{opt.label}</span>
          </TabsPrimitive.Tab>
        ))}
      </TabsPrimitive.List>
    </TabsPrimitive.Root>
  );
}
