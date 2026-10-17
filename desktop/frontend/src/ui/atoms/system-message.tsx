import * as stylex from "@stylexjs/stylex";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, layout, leading, radius, space, surface, type } from "@flame/ui/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons";
import { Button, type ButtonProps } from "@flame/ui/button";

type SystemMessageVariant = "info" | "warning" | "error" | "success";

type SystemMessageShape = "sentence" | "form";

const styles = stylex.create({
  banner: {
    display: "flex",
    flexWrap: "wrap",
    minWidth: 0,
    gap: layout.contentGap,
    borderRadius: radius.lg,
    paddingInline: layout.contentGap,
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
    flex: "1 1 14rem",
    alignItems: "flex-start",
    gap: layout.contentGap,
    lineHeight: leading.body,
  },
  glyph: {
    display: "flex",
    height: "1lh",
    flexShrink: 0,
    alignItems: "center",
    justifyContent: "center",
  },
  copy: { minWidth: 0, flex: 1, overflowWrap: "anywhere", whiteSpace: "pre-wrap" },
  action: { maxWidth: "100%", marginInlineStart: "auto" },
});

const TONE = {
  info: styles.info,
  warning: styles.warning,
  error: styles.error,
  success: styles.success,
} as const;

const DEFAULT_ICON: Record<SystemMessageVariant, IconName> = {
  info: "info",
  warning: "alert",
  error: "circle-x",
  success: "circle-check",
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
        <div data-slot="system-message-copy" {...stylex.props(styles.copy, type.uiMd)}>
          {children}
        </div>
      </div>
      {action && (
        <Button
          variant={action.variant ?? "soft"}
          size="sm"
          styles={styles.action}
          onClick={action.onClick}
        >
          {action.label}
        </Button>
      )}
    </div>
  );
}
