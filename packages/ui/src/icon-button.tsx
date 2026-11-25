import * as stylex from "@stylexjs/stylex";
import { ChevronLeft } from "@flame/icons/react";
import type { IconSize } from "./icon-scale";
import { color, corner, space, surface, type, weight } from "./tokens.stylex";
import { Icon, type IconGlyph } from "./font-scale";
import { Button, type ButtonProps, type ButtonSize } from "./button";
import { GlyphSwap } from "./glyph-swap";
import { Tooltip } from "./tooltip";

type IconButtonOwnProps = Omit<
  ButtonProps,
  "children" | "size" | "aria-label" | "aria-labelledby"
> & {
  icon: IconGlyph;
  hoverIcon?: IconGlyph;
  size?: "xs" | "sm" | "md" | "lg" | "xl";
  iconSize?: IconSize | number;
  title?: string;
  badge?: string | number;
};

type IconButtonProps = IconButtonOwnProps &
  (
    | { "aria-label": string; "aria-labelledby"?: never }
    | { "aria-label"?: never; "aria-labelledby": string }
  );

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
  // The back chevron's stroke, not its button, starts the header's column edge (as
  // Disclosure's does); its hover fill bleeds like a row's.
  back: {
    marginInlineStart:
      "calc((var(--icon-sm) - var(--control-height-sm)) / 2 - var(--icon-sm) * 0.3125)",
  },
});

export function IconButton({
  icon,
  hoverIcon,
  size = "md",
  iconSize = ICON_SIZE[size],
  badge,
  className,
  styles: extra,
  title,
  ...props
}: IconButtonProps) {
  const hasBadge = badge !== undefined && badge !== "" && badge !== 0;
  return (
    <Tooltip label={title ?? props["aria-label"]}>
      <Button
        {...props}
        size={BOX[size]}
        className={className}
        styles={[hasBadge && styles.host, extra]}
      >
        {hoverIcon ? (
          <GlyphSwap
            rest={<Icon glyph={icon} size={iconSize} />}
            hover={<Icon glyph={hoverIcon} size={iconSize} />}
          />
        ) : (
          <Icon glyph={icon} size={iconSize} />
        )}
        {hasBadge && (
          <span {...stylex.props(styles.badge, corner.pill, type.ui2xs)}>
            {badge}
          </span>
        )}
      </Button>
    </Tooltip>
  );
}

export function BackButton(
  props: Omit<IconButtonOwnProps, "icon" | "size" | "styles"> & {
    "aria-label": string;
  },
) {
  return (
    <IconButton {...props} icon={ChevronLeft} size="sm" styles={styles.back} />
  );
}
