import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

/**
 * A strip on the composer's top edge, tucked behind it.
 *
 * It owns the SHAPE and not the material or the width. Its two callers look nothing alike — the
 * Goal tray is edged, backdrop-filtered and spans the composer; the project tray is a plain fill
 * inset from both edges. Anything declared here that a caller also declares is merged by `cn`,
 * which leaves precedence to stylesheet order, so only what neither caller disagrees with lives
 * here.
 */
const styles = stylex.create({
  surface: {
    position: "relative",
    minWidth: 0,
    // `clip` and not `hidden`: a scroll container here would swallow the transcript's wheel
    // events at the composer's edge.
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
