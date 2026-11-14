import * as stylex from "@stylexjs/stylex";
import { type StyleXArray, type StyleXStyles } from "@stylexjs/stylex";
import { type ReactNode } from "react";
import {
  FIELD_SIZE,
  FIELD_SURFACE,
  vocab,
  ButtonPrimitive,
  type ButtonPrimitiveProps,
} from "@flame/ui";
import { cn } from "@/lib/classNames";
import { color, space, type, weight } from "@flame/ui/tokens.stylex";
import { Icon } from "@/ui/icons/index";

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
  },
  chevronInk: { flexShrink: 0, color: color.fgFaint },
});

export interface SelectTriggerProps extends Omit<
  ButtonPrimitiveProps,
  "children" | "aria-label" | "aria-labelledby"
> {
  label: string;
  purpose: string;
  leading?: ReactNode;
  styles?: StyleXArray<StyleXStyles | null | false>;
}

export function SelectTrigger({
  label,
  purpose,
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
    <ButtonPrimitive
      {...props}
      type={props.type ?? "button"}
      aria-label={`${label}, ${purpose}`}
      {...styled}
      className={cn(styled.className, className)}
    >
      {lead}
      <span title={label} {...stylex.props(styles.label, vocab.truncate)}>
        {label}
      </span>
      <Icon name="chevron-down" size="xs" {...stylex.props(styles.chevronInk)} />
    </ButtonPrimitive>
  );
}
