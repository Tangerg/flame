import type { ComponentProps } from "react";
import { cn } from "@/lib/classNames";

// No border: the edge is the first layer of `--shadow-popover`. Adding one puts a
// second line on the same boundary (DESIGN.md §5).

const FLOATING_SURFACE_BASE = [
  // `isolate` rather than a z-index: the stacking context exists only to keep the
  // blur's `-z-1` above this element's own background.
  "relative isolate overflow-hidden text-fg",
  "bg-[var(--app-floating-surface)] shadow-[var(--shadow-popover)]",
  // Blur on a `before` layer so it composites UNDER the content instead of blurring it.
  "before:pointer-events-none before:absolute before:inset-0 before:-z-1",
  "before:rounded-[inherit] before:[backdrop-filter:var(--floating-backdrop)]",
].join(" ");

/**
 * Arrival/exit for a surface driven by a Base UI part.
 *
 * A transition, not the `rise-in` keyframe: Base UI holds a closing part mounted under
 * `data-ending-style`, and only a transition animates the exit and stays interruptible
 * (a keyframe restarts from frame one when a panel is reopened mid-dismiss).
 *
 * `scale`/`translate` rather than `transform`: the positioner owns `transform`.
 */
export const FLOATING_MOTION = [
  "transition-[opacity,scale,translate] ease-[var(--ease-out)] duration-[var(--dur-fast)]",
  "data-[starting-style]:scale-[0.97] data-[starting-style]:translate-y-1",
  "data-[ending-style]:scale-[0.97] data-[ending-style]:translate-y-1",
  "data-[starting-style]:opacity-0 data-[ending-style]:opacity-0",
  "data-[ending-style]:duration-[var(--dur-instant)]",
].join(" ");

export const MODAL_SCRIM = [
  "fixed inset-0 z-[var(--layer-modal)] bg-scrim",
  "transition-opacity ease-[var(--ease-out)] duration-[var(--dur-fast)]",
  "data-[starting-style]:opacity-0 data-[ending-style]:opacity-0",
  "data-[ending-style]:duration-[var(--dur-instant)]",
].join(" ");

/**
 * Goes on the POSITIONER, never on the panel inside it. Base UI positions the portaled
 * node with a `transform`, so the panel is already its own stacking context and a
 * z-index there settles nothing outside it — while a positioner left at `auto` loses to
 * any page element holding a layer.
 */
export const FLOATING_LAYER = "z-[var(--layer-floating)]";

export const FLOATING_PANEL = `${FLOATING_SURFACE_BASE} ${FLOATING_MOTION} rounded-[var(--floating-panel-radius)]`;

export const FLOATING_TIP = `${FLOATING_SURFACE_BASE} ${FLOATING_MOTION} rounded-[var(--floating-tip-radius)]`;

/**
 * A floating panel with no behaviour of its own, for surfaces the popover/menu/tooltip
 * models cannot host (the composer's inline-anchored pickers).
 *
 * Keeps the one-shot `rise-in` keyframe rather than `FLOATING_MOTION`: the caller mounts
 * and unmounts directly, so nothing ever sets `data-starting-style`/`data-ending-style`
 * and a transition would have no state to animate from.
 */
export function FloatingSurface({
  className,
  ...props
}: ComponentProps<"div"> & { className?: string }) {
  return (
    <div
      {...props}
      className={cn(
        FLOATING_SURFACE_BASE,
        "animate-rise-in rounded-[var(--floating-panel-radius)]",
        className,
      )}
    />
  );
}
