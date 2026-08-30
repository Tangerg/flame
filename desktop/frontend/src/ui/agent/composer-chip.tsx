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
 * `Button` is `shrink-0`, which is right for a button and wrong for these — they sit in one
 * no-wrap row inside a clipped, rounded composer, so a row wider than the composer pushed the
 * last chip through the edge, sliced. It could be seen and not clicked. They shrink here and
 * the LABEL is what gives way, because the glyph and the chevron are what say the control is
 * a control. One rule for all of them, instead of the three different hand-picked maximum
 * widths that were here before and still let the row overflow.
 *
 * They do NOT give way equally. Shrinking every chip the same amount spends the shortfall on
 * whichever labels happen to be longest and truncates all of them to initials. The model name
 * is the one to spend: it is the longest, and its picker shows the full name anyway. The
 * other two are single short words that stop meaning anything the moment they are cut.
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
