import * as stylex from "@stylexjs/stylex";
import type { IconSize } from "@/lib/iconScale";
import { cn } from "@/lib/classNames";
import { color, corner, space, surface, type, weight } from "@/styles/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons";
import { Button, type ButtonProps, type ButtonSize } from "./button";
import { GlyphSwap } from "./glyph-swap";
import { Tooltip } from "./tooltip";

interface IconButtonProps extends Omit<ButtonProps, "children" | "size"> {
  icon: IconName;
  hoverIcon?: IconName;
  size?: "xs" | "sm" | "md" | "lg" | "xl";
  iconSize?: IconSize;
  title?: string;
  badge?: string | number;
}

const BOX: Record<NonNullable<IconButtonProps["size"]>, ButtonSize> = {
  xs: "icon-xs",
  sm: "icon-sm",
  md: "icon-md",
  lg: "icon-lg",
  xl: "icon-xl",
};

const ICON_SIZE: Record<keyof typeof BOX, IconSize> = {
  xs: "xs",
  sm: "sm",
  md: "md",
  lg: "md",
  xl: "md",
};

const styles = stylex.create({
  // The badge hangs off the corner, so the button is its containing block.
  host: { position: "relative" },
  badge: {
    position: "absolute",
    top: "-2px",
    right: "-2px",
    display: "grid",
    height: "14px",
    minWidth: "14px",
    placeItems: "center",
    backgroundColor: surface.ctaFill,
    paddingInline: space.s0_5,
    fontFamily: "var(--font-mono)",
    fontWeight: weight.semibold,
    color: color.ctaText,
  },
});

export function IconButton({
  icon,
  hoverIcon,
  size = "md",
  iconSize = ICON_SIZE[size],
  badge,
  className,
  title,
  ...props
}: IconButtonProps) {
  const hasBadge = badge !== undefined && badge !== "" && badge !== 0;
  // Only a badge needs the button to be a containing block. Declaring it always would pin every
  // icon button to `relative`, and the one that has to float could not say otherwise.
  const host = stylex.props(hasBadge && styles.host);
  return (
    <Tooltip label={title}>
      <Button
        {...props}
        aria-label={props["aria-label"] ?? title}
        size={BOX[size]}
        className={cn(host.className, className)}
      >
        {hoverIcon ? (
          <GlyphSwap
            rest={<Icon name={icon} size={iconSize} />}
            hover={<Icon name={hoverIcon} size={iconSize} />}
          />
        ) : (
          <Icon name={icon} size={iconSize} />
        )}
        {hasBadge && <span {...stylex.props(styles.badge, corner.pill, type.ui2xs)}>{badge}</span>}
      </Button>
    </Tooltip>
  );
}
