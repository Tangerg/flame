import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

/**
 * A strip on the composer's top edge, tucked behind it.
 *
 * It owns the SHAPE and not the material. Its two callers look nothing alike — the Goal tray
 * is edged and backdrop-filtered, the project tray is a plain fill with no edge at all — and
 * the surface used to declare the Goal tray's material as if it were shared, which the project
 * tray then cancelled: `border-width: 0` and a different `background`, at the call site.
 *
 * That cancellation was invisible for as long as the material was Tailwind and the override was
 * StyleX, because a generated class outranks any utility. Migrating the material to StyleX put
 * the two on equal footing and the borders came back — through `cn`, which concatenates two
 * separately generated class lists and leaves precedence to stylesheet order. There is no right
 * answer to that race; the answer is not to hold one, so what is left here is what neither
 * caller has ever disagreed with.
 */
const styles = stylex.create({
  surface: {
    position: "relative",
    width: "100%",
    minWidth: 0,
    // `clip` and not `hidden`: a scroll container here would swallow the transcript's wheel
    // events at the composer's edge.
    overflow: "clip",
    // The composer's own corner, so the two read as one surface where they meet.
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
