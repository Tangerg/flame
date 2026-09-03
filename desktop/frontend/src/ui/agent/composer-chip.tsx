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

/** The middle grid track is the only one that may shrink, so a chip bottoms out at its glyph
 *  and chevron instead of a sliver whose contents spill onto the next control. `title` names
 *  the current value because that is where the label survives. */
export function AgentComposerChip({
  leading,
  label,
  className,
  shrink = "holds",
  title,
  ...props
}: Props) {
  return (
    <Button
      variant="ghost"
      size="md"
      press={false}
      title={title ?? label}
      className={cn(
        "grid grid-cols-[auto_minmax(0,auto)_auto] gap-1.5 px-2",
        "text-ui-sm text-fg-soft data-[popup-open]:bg-selected data-[popup-open]:text-fg",
        shrink === "gives" ? "shrink-[12]" : "shrink",
        className,
      )}
      {...props}
    >
      <span className="flex items-center">{leading}</span>
      <span data-slot="composer-chip-label" className="truncate">
        {label}
      </span>
      <Icon name="chevron-down" size="sm" className="text-fg-faint" />
    </Button>
  );
}
