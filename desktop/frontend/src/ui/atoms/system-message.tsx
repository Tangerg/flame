import * as stylex from "@stylexjs/stylex";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, leading, radius, space, surface, type } from "@/styles/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons";
import { Button, type ButtonProps } from "./button";

type SystemMessageVariant = "info" | "warning" | "error" | "success";

/**
 * What holds the message. A `sentence` centres against its one line; `form` holds a field and
 * the buttons that answer it, so its content starts at the top and it takes a deeper inset —
 * the banner's alignment follows what it is carrying, which is why it is not a caller's class.
 */
type SystemMessageShape = "sentence" | "form";

const styles = stylex.create({
  banner: {
    display: "flex",
    flexDirection: "row",
    gap: space.s3,
    borderRadius: radius.lg,
    paddingInline: space.s3,
  },
  sentence: { alignItems: "center", paddingBlock: space.s2 },
  form: { alignItems: "flex-start", paddingBlock: space.s2_5 },
  info: { backgroundColor: surface.infoWash, color: color.info },
  warning: { backgroundColor: surface.warningWash, color: color.warning },
  error: { backgroundColor: surface.negativeWash, color: color.negative },
  success: { backgroundColor: surface.successWash, color: color.success },
  body: {
    display: "flex",
    minWidth: 0,
    flex: 1,
    flexDirection: "row",
    alignItems: "flex-start",
    gap: space.s2_5,
    lineHeight: leading.body,
  },
  // One line-box tall and centred in it, so the glyph rides the first line of the copy rather
  // than the middle of a paragraph.
  glyph: {
    display: "flex",
    height: "1lh",
    flexShrink: 0,
    alignItems: "center",
    justifyContent: "center",
  },
  copy: { minWidth: 0, flex: 1 },
});

const TONE = {
  info: styles.info,
  warning: styles.warning,
  error: styles.error,
  success: styles.success,
} as const;

const DEFAULT_ICON: Record<SystemMessageVariant, IconName> = {
  info: "question",
  warning: "alert",
  error: "x",
  success: "check",
};

export type SystemMessageProps = ComponentProps<"div"> & {
  variant?: SystemMessageVariant;
  shape?: SystemMessageShape;
  icon?: IconName;
  hideIcon?: boolean;
  action?: { label: string; onClick?: () => void; variant?: ButtonProps["variant"] };
  children: ReactNode;
};

export function SystemMessage({
  variant = "info",
  shape = "sentence",
  icon,
  hideIcon = false,
  action,
  className,
  children,
  ...props
}: SystemMessageProps) {
  const iconName = icon ?? DEFAULT_ICON[variant];
  // An error or a warning interrupts; the other two report when the reader gets there.
  const role = variant === "error" || variant === "warning" ? "alert" : "status";
  const styled = stylex.props(styles.banner, styles[shape], TONE[variant]);

  return (
    <div role={role} {...styled} className={cn(styled.className, className)} {...props}>
      <div {...stylex.props(styles.body)}>
        {!hideIcon && (
          <span {...stylex.props(styles.glyph)}>
            <Icon name={iconName} size="md" />
          </span>
        )}
        <div {...stylex.props(styles.copy, type.uiMd)}>{children}</div>
      </div>
      {action && (
        <Button variant={action.variant ?? "soft"} size="sm" onClick={action.onClick}>
          {action.label}
        </Button>
      )}
    </div>
  );
}
