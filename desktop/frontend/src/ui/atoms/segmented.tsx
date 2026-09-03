import { useId } from "react";
import { motion } from "motion/react";
import { cn } from "@/lib/classNames";
import { selectionTransition } from "@/lib/motion";
import { TabsPrimitive } from "@/ui/primitives";

export interface SegmentedOption<T> {
  value: T;
  label: string;
}

interface SegmentedProps<T extends string | number> {
  value: T;
  options: SegmentedOption<T>[];
  onChange: (value: T) => void;
  ariaLabel: string;
  className?: string;
}

export function Segmented<T extends string | number>({
  value,
  options,
  onChange,
  ariaLabel,
  className,
}: SegmentedProps<T>) {
  const chipId = useId();
  return (
    <TabsPrimitive.Root
      value={String(value)}
      onValueChange={(v) => {
        const opt = options.find((o) => String(o.value) === v);
        if (opt) onChange(opt.value);
      }}
      className={cn(
        "inline-flex w-fit items-center gap-0.5 rounded-[var(--segmented-radius)] p-0.5",
        "border-[length:var(--control-edge-width)] border-field bg-sunken shadow-[var(--shadow-well)]",
        className,
      )}
    >
      <TabsPrimitive.List aria-label={ariaLabel} className="contents" activateOnFocus>
        {options.map((opt) => (
          <TabsPrimitive.Tab
            key={String(opt.value)}
            value={String(opt.value)}
            className={cn(
              "relative h-[var(--control-height-xs)] rounded-[var(--segment-radius)] border-0 bg-transparent px-2 text-ui-sm font-medium",
              "text-fg-muted transition-colors duration-[var(--dur-color)] ease-out",
              "hover:text-fg",
              "data-[active]:text-fg",
              "focus-visible:outline-none",
            )}
          >
            {String(opt.value) === String(value) && (
              <motion.span
                aria-hidden
                layoutId={chipId}
                transition={selectionTransition}
                className={cn(
                  "absolute inset-0 rounded-[var(--segment-radius)]",
                  "border-[length:var(--control-edge-width)] border-field bg-canvas",
                  "shadow-[var(--shadow-raised-chip)]",
                )}
              />
            )}
            <span className="relative">{opt.label}</span>
          </TabsPrimitive.Tab>
        ))}
      </TabsPrimitive.List>
    </TabsPrimitive.Root>
  );
}
