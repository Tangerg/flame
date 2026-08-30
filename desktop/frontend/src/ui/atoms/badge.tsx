import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import type { Tone } from "@/lib/tone";
import { cn } from "@/lib/classNames";

const styles = cva("inline-flex shrink-0 items-center gap-1 rounded-pill font-medium", {
  variants: {
    tone: {
      neutral: "bg-surface-2 text-fg-muted",
      accent: "bg-accent-badge text-fg-soft",
      success: "bg-success-badge text-fg-soft",
      warning: "bg-warning-badge text-fg-soft",
      negative: "bg-negative-badge text-fg-soft",
      info: "bg-info-badge text-fg-soft",
    },
    size: {
      sm: "px-2 py-px text-ui-xs",
      md: "px-2.5 py-0.5 text-ui-sm",
    },
  },
  defaultVariants: { tone: "neutral", size: "sm" },
});

export type BadgeProps = Omit<VariantProps<typeof styles>, "tone"> & {
  tone?: Tone;
  children: ReactNode;
  className?: string;
  title?: string;
};

export function Badge({ tone, size, className, children, title }: BadgeProps) {
  return (
    <span title={title} className={cn(styles({ tone, size }), className)}>
      {children}
    </span>
  );
}
