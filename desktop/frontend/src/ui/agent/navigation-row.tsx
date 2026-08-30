import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Button, type ButtonProps, Icon, type IconName } from "@/ui";
import { Tooltip } from "@/ui/atoms/tooltip";
import { AgentOverflowLabel } from "./overflow-label";

// NAMED group because rows nest (a project header wraps a button that is itself a group).
// The variants below are spelled out, not interpolated — Tailwind only emits utilities it
// can read as complete literals.
const ROW_GROUP = "group/row";

// The resting glyph stops intercepting clicks so the action REPLACES it rather than
// stacking on top.
const RESTING_GLYPH =
  "transition-opacity group-hover/row:pointer-events-none group-hover/row:opacity-0 group-focus-within/row:pointer-events-none group-focus-within/row:opacity-0";

const HOVER_ACTION =
  "opacity-0 transition-opacity group-hover/row:opacity-100 group-focus-within/row:opacity-100";

interface AgentRowProps extends Omit<ButtonProps, "children" | "variant" | "size" | "press"> {
  active?: boolean;
  icon?: IconName;
  iconClassName?: string;
  /** A prop, not `children`: opting in swaps the row's fixed height for a minimum, and
   *  row height is part of the index's rhythm. */
  detail?: ReactNode;
  trailing?: ReactNode;
  /** Rendered as a SIBLING of the row, because a button cannot nest inside one. */
  action?: ReactNode;
  indent?: "none" | "nested";
  revealOverflow?: boolean;
  children?: ReactNode;
}

export function AgentRow({
  active,
  icon,
  iconClassName,
  detail,
  trailing,
  action,
  indent = "none",
  revealOverflow = false,
  className,
  children,
  type = "button",
  ...props
}: AgentRowProps) {
  const overflowText = revealOverflow && typeof children === "string" ? children : undefined;
  const button = (
    <Button
      {...props}
      type={type}
      variant="ghost"
      size="sm"
      press={false}
      data-active={active ? "" : undefined}
      className={cn(
        // `font-normal` spelled out because `Button` defaults to medium: at 500 every row
        // is emphasised, so nothing is, and selection is left with only its fill to say so.
        "agent-row w-full justify-start rounded-[var(--row-radius)] text-left text-ui-md font-normal",
        "gap-[var(--density-row-gap)]",
        "text-fg transition-[background-color,color] duration-[var(--dur-color)]",
        "hover:bg-hover hover:text-fg focus-visible:bg-hover",
        "data-[active]:bg-selected data-[active]:text-fg",
        // `h-auto` is load-bearing: the size variant ships a fixed `h-`, and without
        // replacing it the second line renders outside the row's fill.
        detail
          ? "h-auto min-h-[var(--density-row-height)] items-start py-2"
          : "h-[var(--density-row-height)]",
        // Spelled as the sum of inset + glyph + gap so a density or icon-ladder change
        // moves parent and child together.
        indent === "nested"
          ? "px-2 pl-[calc(0.5rem+var(--icon-sm)+var(--density-row-gap))]"
          : "px-2",
        action && "pr-8",
        className,
      )}
    >
      {icon && (
        <Icon
          name={icon}
          size="sm"
          className={cn("shrink-0 text-fg", detail && "mt-px", iconClassName)}
        />
      )}
      <span className="flex min-w-0 flex-1 flex-col gap-px">
        <span
          className={cn(
            "flex min-w-0 items-center gap-2",
            detail ? "leading-body" : "leading-snug",
          )}
        >
          {overflowText ? (
            <AgentOverflowLabel text={overflowText} />
          ) : (
            <span className="min-w-0 flex-1 truncate-fade">{children}</span>
          )}
          {trailing && <span className={cn("shrink-0", action && RESTING_GLYPH)}>{trailing}</span>}
        </span>
        {detail != null && (
          <span className="min-w-0 truncate-fade text-ui-2xs leading-body text-fg-faint">
            {detail}
          </span>
        )}
      </span>
    </Button>
  );
  const row = overflowText ? (
    <Tooltip label={overflowText} side="right" sideOffset={8} delayDuration={500}>
      {button}
    </Tooltip>
  ) : (
    button
  );

  if (!action) return row;
  return (
    <div className={cn("relative select-none", ROW_GROUP)}>
      {row}
      <span className={cn("absolute inset-y-0 right-1 grid place-items-center", HOVER_ACTION)}>
        {action}
      </span>
    </div>
  );
}
