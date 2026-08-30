import type { VariantProps } from "class-variance-authority";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/classNames";

const dotStyles = cva("inline-block h-1.5 w-1.5 shrink-0 rounded-full", {
  variants: {
    tone: {
      idle: "bg-fg-faint",
      running: "bg-accent shadow-[var(--shadow-live-glow)] animate-pulse-dot",
      waiting: "bg-warning",
      ok: "bg-success",
      err: "bg-negative",
    },
  },
  defaultVariants: { tone: "idle" },
});

type Props = VariantProps<typeof dotStyles> & { className?: string };

export function StatusDot({ tone, className }: Props) {
  return <span aria-hidden="true" className={cn(dotStyles({ tone }), className)} />;
}
