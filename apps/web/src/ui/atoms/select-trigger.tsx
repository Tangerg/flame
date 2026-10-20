import * as stylex from "@stylexjs/stylex";
import { type StyleXArray, type StyleXStyles } from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { FIELD_SIZE, FIELD_SURFACE } from "@flame/ui";
import { cn } from "@/lib/classNames";
import { color, space, type, weight } from "@flame/ui/tokens.stylex";
import { Icon } from "@/ui/icons/index";
import { Pressable, type PressableProps } from "@/ui/atoms/pressable";
const styles = stylex.create({
  trigger: {
    display: "inline-flex",
    width: "fit-content",
    minWidth: "var(--select-min-width)",
    alignItems: "center",
    justifyContent: "space-between",
    gap: space.s2,
    paddingBlock: space.s1_5,
    textAlign: "start",
    fontWeight: weight.medium,
  },
  label: {
    minWidth: 0,
    flex: 1,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
  chevronInk: { flexShrink: 0, color: color.fgFaint },
});

export interface SelectTriggerProps extends Omit<PressableProps, "children"> {
  label: ReactNode;
  leading?: ReactNode;
  styles?: StyleXArray<StyleXStyles | null | false>;
}

export function SelectTrigger({
  label,
  leading: lead,
  styles: extra,
  className,
  ...props
}: SelectTriggerProps) {
  const styled = stylex.props(
    FIELD_SURFACE.face,
    FIELD_SURFACE.trigger,
    FIELD_SIZE.md,
    styles.trigger,
    type.uiMd,
    extra,
  );
  return (
    <Pressable
      {...props}
      type={props.type ?? "button"}
      data-chrome-focus=""
      {...styled}
      className={cn(styled.className, className)}
    >
      {lead}
      <span title={typeof label === "string" ? label : undefined} {...stylex.props(styles.label)}>
        {label}
      </span>
      <Icon name="chevron-down" size="xs" {...stylex.props(styles.chevronInk)} />
    </Pressable>
  );
}
