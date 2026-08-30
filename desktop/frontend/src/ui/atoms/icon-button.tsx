import type { IconSize } from "@/lib/iconScale";
import { cn } from "@/lib/classNames";
import { Icon, type IconName } from "@/ui/icons";
import { useState } from "react";
import { Button, type ButtonProps } from "./button";
import { Tooltip } from "./tooltip";

interface IconButtonProps extends Omit<ButtonProps, "children" | "variant" | "size"> {
  icon: IconName;
  hoverIcon?: IconName;
  size?: "xs" | "sm" | "md" | "lg";
  iconSize?: IconSize;
  active?: boolean;
  quiet?: boolean;
  title?: string;
  badge?: string | number;
}

const BOX = { xs: "icon-xs", sm: "icon-sm", md: "icon-md", lg: "icon-lg" } as const;
const ICON_SIZE: Record<keyof typeof BOX, IconSize> = { xs: "xs", sm: "sm", md: "md", lg: "md" };

export function IconButton({
  icon,
  hoverIcon,
  size = "md",
  iconSize = ICON_SIZE[size],
  active,
  quiet,
  badge,
  className,
  title,
  onPointerEnter,
  onPointerLeave,
  onFocus,
  onBlur,
  ...props
}: IconButtonProps) {
  const [showHoverIcon, setShowHoverIcon] = useState(false);
  return (
    <Tooltip label={title}>
      <Button
        {...props}
        aria-label={props["aria-label"] ?? title}
        onPointerEnter={(event) => {
          setShowHoverIcon(true);
          onPointerEnter?.(event);
        }}
        onPointerLeave={(event) => {
          setShowHoverIcon(false);
          onPointerLeave?.(event);
        }}
        onFocus={(event) => {
          setShowHoverIcon(true);
          onFocus?.(event);
        }}
        onBlur={(event) => {
          setShowHoverIcon(false);
          onBlur?.(event);
        }}
        variant="ghost"
        size={BOX[size]}
        data-active={active ? "" : undefined}
        className={cn(
          "relative data-[active]:bg-selected data-[active]:text-fg",
          quiet && "text-fg-faint",
          className,
        )}
      >
        {hoverIcon ? (
          <span className="t-icon-swap" data-state={showHoverIcon ? "b" : "a"}>
            <span className="t-icon" data-icon="a">
              <Icon name={icon} size={iconSize} />
            </span>
            <span className="t-icon" data-icon="b">
              <Icon name={hoverIcon} size={iconSize} />
            </span>
          </span>
        ) : (
          <Icon name={icon} size={iconSize} />
        )}
        {badge !== undefined && badge !== "" && badge !== 0 && (
          <span className="absolute -top-0.5 -right-0.5 grid h-3.5 min-w-3.5 place-items-center rounded-full bg-cta px-0.5 font-mono text-ui-2xs font-semibold text-cta-text">
            {badge}
          </span>
        )}
      </Button>
    </Tooltip>
  );
}
