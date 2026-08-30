import type { ComponentProps } from "react";
import { cn } from "@/lib/classNames";

const FLOATING_SURFACE_BASE = [
  "relative isolate overflow-hidden text-fg",
  "bg-[var(--app-floating-surface)] shadow-[var(--shadow-popover)]",
  "before:pointer-events-none before:absolute before:inset-0 before:-z-1",
  "before:rounded-[inherit] before:[backdrop-filter:var(--floating-backdrop)]",
].join(" ");

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

export const FLOATING_LAYER = "z-[var(--layer-floating)]";

export const FLOATING_PANEL = `${FLOATING_SURFACE_BASE} ${FLOATING_MOTION} rounded-[var(--floating-panel-radius)]`;

export const FLOATING_TIP = `${FLOATING_SURFACE_BASE} ${FLOATING_MOTION} rounded-[var(--floating-tip-radius)]`;

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
