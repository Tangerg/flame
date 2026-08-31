import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Button, type ButtonProps } from "@/ui/atoms";
import { Icon } from "@/ui/icons";

interface Props extends Omit<ButtonProps, "children" | "variant" | "size"> {
  leading: ReactNode;
  label: string;
  /** `gives` yields its label first when the row is short. Default holds as long as it can. */
  shrink?: "holds" | "gives";
}

/**
 * A composer footer chip: leading glyph, label, disclosure chevron.
 *
 * Shrinks, unlike `Button`: these share one no-wrap row inside a clipped, rounded composer,
 * where a row wider than the composer leaves the last chip sliced by the edge — visible and
 * unclickable. The label gives way, never the glyph or chevron. Not equally, either: shrinking
 * each the same truncates every label to initials, so `gives` spends the model name first.
 */
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
