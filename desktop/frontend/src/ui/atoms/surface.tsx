import type { ComponentPropsWithRef } from "react";
import { cva } from "class-variance-authority";
import type { VariantProps } from "class-variance-authority";
import { cn } from "@/lib/classNames";

const styles = cva(
  "rounded-[var(--surface-card-radius)] bg-[var(--app-card-surface)] shadow-[var(--shadow-surface-card)]",
  {
    variants: {
      // `none` is for a card hosting rows that already pad themselves.
      inset: { none: "", sm: "p-3", md: "p-4" },
    },
    defaultVariants: { inset: "md" },
  },
);

export type SurfaceProps = ComponentPropsWithRef<"div"> & VariantProps<typeof styles>;

export function Surface({ inset, className, children, ...props }: SurfaceProps) {
  return (
    <div {...props} className={cn(styles({ inset }), className)}>
      {children}
    </div>
  );
}
