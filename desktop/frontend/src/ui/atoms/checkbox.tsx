import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon } from "@/ui/icons";
import { CheckboxPrimitive } from "@/ui/primitives";

interface CheckboxProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  label: ReactNode;
  disabled?: boolean;
  className?: string;
}

export function Checkbox({ checked, onCheckedChange, label, disabled, className }: CheckboxProps) {
  return (
    <label
      className={cn(
        "inline-flex items-center gap-2 text-ui-md text-fg-muted select-none",
        disabled ? "cursor-not-allowed opacity-60" : "cursor-default",
        className,
      )}
    >
      <CheckboxPrimitive.Root
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
        className={cn(
          "grid h-[18px] w-[18px] shrink-0 place-items-center rounded-2xs border-[length:var(--control-edge-width)] border-field bg-canvas transition-colors duration-[var(--dur-color)]",
          "data-[checked]:border-accent data-[checked]:bg-accent",
        )}
      >
        <CheckboxPrimitive.Indicator>
          <Icon name="check" size="xs" className="text-on-accent" />
        </CheckboxPrimitive.Indicator>
      </CheckboxPrimitive.Root>
      <span>{label}</span>
    </label>
  );
}
