import type { VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { Icon, type IconName } from "@/ui/icons";
import { Button, type ButtonProps } from "./button";

const banner = cva("flex flex-row items-center gap-3 rounded-lg px-3 py-2", {
  variants: {
    variant: {
      info: "bg-info-wash text-info",
      warning: "bg-warning-wash text-warning",
      error: "bg-negative-wash text-negative",
      success: "bg-success-wash text-success",
    },
  },
  defaultVariants: { variant: "info" },
});

const DEFAULT_ICON: Record<NonNullable<VariantProps<typeof banner>["variant"]>, IconName> = {
  info: "question",
  warning: "alert",
  error: "x",
  success: "check",
};

export type SystemMessageProps = ComponentProps<"div"> &
  VariantProps<typeof banner> & {
    icon?: IconName;
    hideIcon?: boolean;
    action?: { label: string; onClick?: () => void; variant?: ButtonProps["variant"] };
    children: ReactNode;
  };

export function SystemMessage({
  variant = "info",
  icon,
  hideIcon = false,
  action,
  className,
  children,
  ...props
}: SystemMessageProps) {
  const iconName = icon ?? DEFAULT_ICON[variant ?? "info"];
  const role = variant === "error" || variant === "warning" ? "alert" : "status";

  return (
    <div role={role} className={cn(banner({ variant }), className)} {...props}>
      <div className="flex min-w-0 flex-1 flex-row items-start gap-2.5 leading-normal">
        {!hideIcon && (
          <span className="flex h-[1lh] shrink-0 items-center justify-center">
            <Icon name={iconName} size="md" />
          </span>
        )}
        <div className="min-w-0 flex-1 text-ui-md">{children}</div>
      </div>
      {action && (
        <Button variant={action.variant ?? "soft"} size="sm" onClick={action.onClick}>
          {action.label}
        </Button>
      )}
    </div>
  );
}
