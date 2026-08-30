import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon } from "@/ui/icons";
import { Pressable, type PressableProps } from "./pressable";

export interface SelectTriggerProps extends Omit<PressableProps, "children"> {
  label: ReactNode;
  leading?: ReactNode;
}

export function SelectTrigger({ label, leading, className, ...props }: SelectTriggerProps) {
  return (
    <Pressable
      {...props}
      type={props.type ?? "button"}
      className={cn(
        "inline-flex w-fit min-h-[var(--field-height-md)] items-center justify-between gap-2",
        "rounded-[var(--field-radius)] border-[length:var(--control-edge-width)] border-field",
        "bg-surface-2 px-2.5 py-1.5 text-left text-ui-md font-medium text-fg transition-colors",
        "hover:bg-surface-3 data-[popup-open]:bg-surface-3",
        "disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-surface-2",
        className,
      )}
    >
      {leading}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      <Icon name="more" size="xs" className="shrink-0 -rotate-90 text-fg-faint" />
    </Pressable>
  );
}
