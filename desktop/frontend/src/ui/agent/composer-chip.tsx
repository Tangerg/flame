import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Button, type ButtonProps } from "@/ui/atoms";
import { Icon } from "@/ui/icons";

interface Props extends Omit<ButtonProps, "children" | "variant" | "size"> {
  leading: ReactNode;
  label: string;
  /** `gives` yields its label first when the row is short; `holds` keeps it as long as it can.
   *  Shrinking every chip equally truncates all of them to initials. */
  shrink?: "holds" | "gives";
}

export function AgentComposerChip({
  leading,
  label,
  className,
  shrink = "holds",
  ...props
}: Props) {
  return (
    <Button
      variant="ghost"
      size="md"
      press={false}
      className={cn(
        "min-w-0 gap-1.5 px-2 text-ui-sm text-fg-soft data-[popup-open]:bg-selected data-[popup-open]:text-fg",
        shrink === "gives" ? "shrink-[12]" : "shrink",
        className,
      )}
      {...props}
    >
      {leading}
      <span className="truncate">{label}</span>
      <Icon name="chevron-down" size="sm" className="shrink-0 text-fg-faint" />
    </Button>
  );
}
