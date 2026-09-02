import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Button, type ButtonProps, Icon, type IconName } from "@/ui";
import { Tooltip } from "@/ui/atoms/tooltip";
import { AgentOverflowLabel } from "./overflow-label";

const ROW_GROUP = "group/row";

const RESTING_GLYPH =
  "transition-opacity group-hover/row:pointer-events-none group-hover/row:opacity-0 group-focus-visible/row-trigger:pointer-events-none group-focus-visible/row-trigger:opacity-0";

// The action is the caller's node in a sibling span, so only the SPAN can react to it
// having focus — and `:has(:focus-visible)` is not a working way to say that (Chromium
// matches it but does not invalidate on the focus change). `:focus-within` therefore
// stays here: it reveals the action a moment longer than it should after a click, which
// is the lesser of the two, because the alternative hides it from the keyboard.
const HOVER_ACTION =
  "opacity-0 transition-opacity group-hover/row:opacity-100 group-focus-within/row:opacity-100";

interface AgentRowProps extends Omit<ButtonProps, "children" | "variant" | "size" | "press"> {
  active?: boolean;
  icon?: IconName;
  detail?: ReactNode;
  trailing?: ReactNode;
  action?: ReactNode;
  indent?: "none" | "nested";
  revealOverflow?: boolean;
  children?: ReactNode;
}

export function AgentRow({
  active,
  icon,
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
        "group/row-trigger agent-row w-full justify-start rounded-[var(--row-radius)] text-left text-ui-md font-normal",
        "gap-[var(--density-row-gap)]",
        "text-fg transition-[background-color,color] duration-[var(--dur-color)]",
        "hover:bg-hover hover:text-fg focus-visible:bg-hover",
        "data-[active]:bg-selected data-[active]:text-fg",
        detail
          ? "h-auto min-h-[var(--density-row-height)] items-start py-2"
          : "h-[var(--density-row-height)]",
        indent === "nested"
          ? "px-2 pl-[calc(0.5rem+var(--icon-sm)+var(--density-row-gap))]"
          : "px-2",
        action && "pr-8",
        className,
      )}
    >
      {icon && <Icon name={icon} size="sm" className={cn("shrink-0 text-fg", detail && "mt-px")} />}
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
          {trailing && (
            <span
              data-reveal={action ? "rest" : undefined}
              className={cn("shrink-0", action && RESTING_GLYPH)}
            >
              {trailing}
            </span>
          )}
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
      <span
        data-reveal="hover"
        className={cn("absolute inset-y-0 right-1 grid place-items-center", HOVER_ACTION)}
      >
        {action}
      </span>
    </div>
  );
}
