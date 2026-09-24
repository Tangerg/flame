import * as stylex from "@stylexjs/stylex";
import { motion, space } from "@/styles/tokens.stylex";
import type { ComponentPropsWithoutRef, ReactNode, Ref } from "react";
import { cn } from "@/lib/classNames";

const styles = stylex.create({
  surface: {
    position: "relative",
    containerType: "inline-size",
    containerName: "composer",
    overflow: "hidden",
    borderRadius: "var(--shape-composer)",
    transitionProperty: "box-shadow",
    transitionDuration: motion.med,
    transitionTimingFunction: motion.easeState,
  },
  footer: {
    display: "flex",
    flexWrap: "nowrap",
    alignItems: "center",
    gap: space.s1_5,
    paddingRight: "var(--composer-footer-end)",
    paddingBottom: "var(--composer-footer)",
    paddingLeft: "var(--composer-footer)",
  },
});

export function AgentComposerSurface({
  className,
  children,
  ref,
  ...props
}: ComponentPropsWithoutRef<"div"> & { ref?: Ref<HTMLDivElement> }) {
  return (
    <div
      {...props}
      ref={ref}
      className={cn("agent-composer-glass", stylex.props(styles.surface).className, className)}
    >
      {children}
    </div>
  );
}

export function AgentComposerFooter({ children }: { children: ReactNode }) {
  return (
    <div
      data-slot="composer-footer"
      className={cn("agent-composer-footer", stylex.props(styles.footer).className)}
    >
      {children}
    </div>
  );
}
