import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

const styles = stylex.create({
  surface: {
    position: "relative",
    minWidth: 0,
    overflow: "clip",
    borderTopLeftRadius: "var(--shape-composer)",
    borderTopRightRadius: "var(--shape-composer)",
  },
});

export function AgentComposerTopTraySurface({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"div">) {
  const styled = stylex.props(styles.surface);
  return (
    <div
      {...props}
      data-slot="composer-top-tray-surface"
      className={cn(styled.className, className)}
    >
      {children}
    </div>
  );
}
