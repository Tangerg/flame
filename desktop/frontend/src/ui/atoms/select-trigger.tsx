import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, motion, radius, space, surface, type, weight } from "@/styles/tokens.stylex";
import { Icon } from "@/ui/icons";
import { Pressable, type PressableProps } from "./pressable";

const styles = stylex.create({
  trigger: {
    display: "inline-flex",
    width: "fit-content",
    // Every one of the three call sites was spelling this out, using the variable that is
    // already named for it: a select is wide enough for the values it has to hold, whichever
    // one is showing. That is the control's measure, not each pane's.
    minWidth: "var(--select-min-width)",
    minHeight: "var(--field-height-md)",
    alignItems: "center",
    justifyContent: "space-between",
    gap: space.s2,
    borderRadius: radius.field,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.field,
    backgroundColor: {
      default: surface.surface2,
      ":hover": surface.surface3,
      ":is([data-popup-open])": surface.surface3,
      ":disabled": surface.surface2,
    },
    paddingInline: space.s2_5,
    paddingBlock: space.s1_5,
    textAlign: "left",
    fontWeight: weight.medium,
    color: color.fg,
    transitionProperty: "background-color, border-color, color",
    transitionDuration: motion.color,
    cursor: { default: null, ":disabled": "not-allowed" },
    opacity: { default: null, ":disabled": 0.5 },
  },
  label: {
    minWidth: 0,
    flex: 1,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
  // The chevron is `more` turned: one glyph in the set, pointed at the menu it opens.
  chevron: { flexShrink: 0, rotate: "-90deg", color: color.fgFaint },
});

export interface SelectTriggerProps extends Omit<PressableProps, "children"> {
  label: ReactNode;
  leading?: ReactNode;
}

export function SelectTrigger({ label, leading: lead, className, ...props }: SelectTriggerProps) {
  const styled = stylex.props(styles.trigger, type.uiMd);
  return (
    <Pressable
      {...props}
      type={props.type ?? "button"}
      {...styled}
      className={cn(styled.className, className)}
    >
      {lead}
      <span {...stylex.props(styles.label)}>{label}</span>
      <Icon name="more" size="xs" {...stylex.props(styles.chevron)} />
    </Pressable>
  );
}
