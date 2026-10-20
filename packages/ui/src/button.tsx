import * as stylex from "@stylexjs/stylex";
import type { StyleXArray, StyleXStyles } from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { clsx as cn } from "clsx";
import {
  color,
  corner,
  face,
  leading,
  motion,
  radius,
  space,
  surface,
  type,
  weight,
} from "./tokens.stylex";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "./primitives/index";
import { toneInk } from "./tone-ink";

type ButtonVariant =
  | "ghost"
  | "soft"
  | "outline"
  | "primary"
  | "wash"
  | "tonal"
  | "media"
  | "mediaTray"
  | "raised"
  | "bare"
  | "link";

export type ButtonTone = "negative" | "warning" | "accent" | "success";

const inset = (step: string) => `calc(${step} - var(--control-edge-width))`;
const INSET_XS = inset(space.s2);
const INSET_SM = inset(space.s2);
const INSET_MD = inset(space.s2_5);
const INSET_LG = inset(space.s3);

const styles = stylex.create({
  base: {
    display: "inline-flex",
    justifySelf: "start",
    width: "fit-content",
    "corner-shape": "round",
    maxWidth: "100%",
    flexShrink: 0,
    alignItems: "center",
    justifyContent: "center",
    gap: space.s1_5,
    whiteSpace: "normal",
    overflowWrap: "anywhere",
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: "transparent",
    fontFamily: "var(--font-sans)",
    fontWeight: weight.medium,
    lineHeight: leading.snug,
    transitionProperty:
      "background-color, border-color, color, opacity, scale, translate, text-decoration-color",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
  },

  xs: {
    minHeight: "max(var(--control-height-xs), var(--control-target-min))",
    borderRadius: radius.button,
    paddingBlock: space.s0_5,
    paddingInline: INSET_XS,
  },
  sm: {
    minHeight: "max(var(--control-height-sm), var(--control-target-min))",
    borderRadius: radius.button,
    paddingBlock: space.s0_5,
    paddingInline: INSET_SM,
  },
  md: {
    minHeight: "max(var(--control-height-md), var(--control-target-min))",
    borderRadius: radius.button,
    paddingBlock: space.s0_5,
    paddingInline: INSET_MD,
  },
  lg: {
    minHeight: "max(var(--control-height-lg), var(--control-target-min))",
    borderRadius: radius.button,
    paddingBlock: space.s0_5,
    paddingInline: INSET_LG,
  },
  iconXs: {
    height: "var(--control-height-xs)",
    width: "var(--control-height-xs)",
    borderRadius: radius.button,
    padding: 0,
  },
  iconSm: {
    height: "var(--control-height-sm)",
    width: "var(--control-height-sm)",
    borderRadius: radius.button,
    padding: 0,
  },
  iconMd: {
    height: "var(--control-height-md)",
    width: "var(--control-height-md)",
    borderRadius: radius.button,
    padding: 0,
  },
  iconLg: {
    height: "var(--control-height-lg)",
    width: "var(--control-height-lg)",
    borderRadius: radius.button,
    padding: 0,
  },
  iconXl: {
    height: "var(--control-height-xl)",
    width: "var(--control-height-xl)",
    borderRadius: radius.button,
    padding: 0,
  },

  press: {
    scale: {
      default: null,
      ":active": "var(--press-scale)",
      ':is(:disabled, [aria-disabled="true"], [data-disabled]):active': 1,
    },
  },
  nudge: {
    translate: {
      default: null,
      ":active": "0 0.5px",
      ':is(:disabled, [aria-disabled="true"], [data-disabled]):active': null,
    },
  },

  ghost: {
    backgroundColor: {
      default: "transparent",
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": surface.hover,
      },
      ":is([data-chrome-focus]):focus-visible": surface.hover,
      ':is([data-popup-open][aria-expanded="true"])': surface.selected,
      ':is([data-popup-open][aria-expanded="true"]):hover:not(:disabled, [aria-disabled=true], [data-disabled])':
        {
          "@media (hover: hover)": surface.selectedHover,
        },
    },
    color: {
      default: color.fgMuted,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": color.fg,
      },
      ':is([data-popup-open][aria-expanded="true"])': color.fg,
    },
  },
  soft: {
    backgroundColor: {
      default: surface.surface2,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": surface.surface3,
      },
    },
    color: {
      default: color.fgSoft,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": color.fg,
      },
    },
  },
  outline: {
    borderColor: surface.fieldStrong,
    backgroundColor: {
      default: "transparent",
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": surface.hover,
      },
    },
    color: {
      default: color.fgSoft,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": color.fg,
      },
    },
  },
  primary: {
    backgroundColor: {
      default: surface.ctaFill,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": surface.ctaHover,
      },
      ':is(:disabled, [aria-disabled="true"], [data-disabled])':
        surface.surface2,
    },
    color: {
      default: color.ctaText,
      ':is(:disabled, [aria-disabled="true"], [data-disabled])': color.fgFaint,
    },
    opacity: {
      default: null,
      ':is(:disabled, [aria-disabled="true"], [data-disabled])': 1,
    },
  },
  wash: { backgroundColor: "transparent" },
  washNegative: {
    backgroundColor: {
      default: "transparent",
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": surface.negativeWash,
      },
    },
    color: color.negative,
  },
  washWarning: {
    backgroundColor: {
      default: "transparent",
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": surface.warningWash,
      },
    },
    color: color.warning,
  },
  tonal: { fontWeight: weight.semibold },
  tonalNegative: {
    backgroundColor: {
      default: surface.negativeWash,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": surface.negativeBadge,
      },
    },
    color: color.negative,
  },
  tonalWarning: {
    backgroundColor: {
      default: surface.warningWash,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": surface.warningBadge,
      },
    },
    color: color.warning,
  },
  media: { backgroundColor: surface.mediaScrim, color: color.onMedia },
  mediaTray: {
    backgroundColor: {
      default: "transparent",
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": surface.mediaScrim,
      },
    },
    color: color.onMedia,
  },
  raised: {
    borderWidth: 0,
    backgroundColor: {
      default: surface.canvas,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": surface.surface2,
      },
    },
    color: {
      default: color.fgSoft,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": color.fg,
      },
    },
    boxShadow: "var(--shadow-raised)",
  },
  bare: {
    height: "auto",
    minHeight: "var(--control-target-min)",
    justifyContent: "flex-start",
    borderRadius: 0,
    borderWidth: 0,
    backgroundColor: "transparent",
    paddingInline: 0,
    paddingBlock: 0,
    lineHeight: "inherit",
    fontWeight: weight.regular,
  },
  link: {
    position: "relative",
    display: "inline-block",
    cursor: "pointer",
    whiteSpace: "normal",
    overflowWrap: "break-word",
    color: {
      default: color.fg,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": color.fgSoft,
      },
    },
    textDecorationLine: "underline",
    textDecorationColor: color.fgFaint,
    textDecorationStyle: "dotted",
    textDecorationThickness: "1px",
    textUnderlineOffset: "4px",
    "::after": { content: "", position: "absolute", inset: "-4px -8px" },
  },

  chip: { color: color.fgSoft },
  chipSm: { paddingInline: inset(space.s1_5) },
  chipMd: { paddingInline: inset(space.s2) },

  quiet: { color: color.fgFaint },
  quietAccent: {
    color: {
      default: color.fgFaint,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": color.accentText,
      },
    },
  },
  quietNegative: {
    color: {
      default: color.fgFaint,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": color.negative,
      },
    },
  },
  quietSuccess: {
    color: {
      default: color.fgFaint,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": color.success,
      },
    },
  },
  quietWarning: {
    color: {
      default: color.fgFaint,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": color.warning,
      },
    },
  },

  fill: { flexGrow: 1, flexShrink: 1, flexBasis: 0, minWidth: 0 },

  active: {
    backgroundColor: {
      default: surface.selected,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": surface.selectedHover,
      },
    },
    color: color.fg,
  },

  row: {
    width: "100%",
    justifyContent: "flex-start",
    borderRadius: radius.row,
    fontWeight: weight.regular,
  },

  joined: { alignSelf: "stretch", height: "auto" },
  joinStart: { borderTopRightRadius: 0, borderBottomRightRadius: 0 },
  joinEnd: {
    position: "relative",
    marginLeft: "-1px",
    borderTopLeftRadius: 0,
    borderBottomLeftRadius: 0,
    "::before": {
      content: "",
      pointerEvents: "none",
      position: "absolute",
      insetBlock: space.s1_5,
      left: 0,
      width: "1px",
      backgroundColor: surface.joinSeam,
    },
  },
});

const SIZE = {
  xs: [styles.xs, type.uiSm],
  sm: [styles.sm, type.uiMd],
  md: [styles.md, type.uiMd],
  lg: [styles.lg, type.uiMd],
  "icon-xs": [styles.iconXs, type.uiSm],
  "icon-sm": [styles.iconSm, type.uiMd],
  "icon-md": [styles.iconMd, type.uiMd],
  "icon-lg": [styles.iconLg, type.uiMd],
  "icon-xl": [styles.iconXl, type.uiMd],
} as const;

const VARIANT = {
  ghost: styles.ghost,
  soft: styles.soft,
  outline: styles.outline,
  primary: styles.primary,
  wash: styles.wash,
  tonal: styles.tonal,
  media: styles.media,
  mediaTray: styles.mediaTray,
  raised: styles.raised,
  bare: styles.bare,
  link: [styles.bare, styles.link],
} as const;

const TONAL = {
  negative: styles.tonalNegative,
  warning: styles.tonalWarning,
} as const;
const WASH = {
  negative: styles.washNegative,
  warning: styles.washWarning,
} as const;

const QUIET_TONE = {
  negative: styles.quietNegative,
  warning: styles.quietWarning,
  accent: styles.quietAccent,
  success: styles.quietSuccess,
} as const;

const CHIP_INSET = { sm: styles.chipSm, md: styles.chipMd } as const;

export type ButtonSize = keyof typeof SIZE;

export interface ButtonVariants {
  variant?: ButtonVariant;
  size?: ButtonSize;
  tone?: ButtonTone;
  press?: "scale" | "nudge" | "none";
  join?: "start" | "end";
  round?: boolean;
  chip?: boolean;
  quiet?: boolean;
  shape?: "control" | "row";
  active?: boolean;
  flex?: "none" | "fill";
  face?: "text" | "mono";
}

function pressFor({
  variant,
  chip,
  shape,
  round,
  join,
}: ButtonVariants): "scale" | "nudge" | "none" {
  if (join || variant === "link" || variant === "bare") return "none";
  if (round && variant === "primary") return "nudge";
  return chip || shape === "row" ? "none" : "scale";
}

export function dress({
  variant = "ghost",
  size = "md",
  tone,
  press,
  join,
  round,
  chip,
  quiet,
  shape,
  active,
  flex,
  face: textFace,
}: ButtonVariants) {
  const chipStep = chip && (size === "sm" || size === "md") ? size : null;
  const pressStep = press ?? pressFor({ variant, chip, shape, round, join });
  return [
    styles.base,
    tone && toneInk[tone],
    SIZE[size],
    pressStep === "scale" && styles.press,
    pressStep === "nudge" && styles.nudge,
    round && corner.pill,
    join && styles.joined,
    join === "start" && styles.joinStart,
    join === "end" && styles.joinEnd,
    VARIANT[variant],
    variant === "tonal" &&
      tone &&
      tone in TONAL &&
      TONAL[tone as keyof typeof TONAL],
    variant === "wash" &&
      tone &&
      tone in WASH &&
      WASH[tone as keyof typeof WASH],
    chip && styles.chip,
    chipStep && [
      CHIP_INSET[chipStep],
      chipStep === "md" ? type.uiMd : type.uiSm,
    ],
    quiet && styles.quiet,
    quiet && tone && QUIET_TONE[tone],
    active && styles.active,
    shape === "row" && styles.row,
    flex === "fill" && styles.fill,
    textFace && face[textFace],
  ];
}

export type ButtonProps = Omit<
  ButtonPrimitiveProps,
  "children" | "data-slot" | "data-variant"
> &
  ButtonVariants & {
    children?: ReactNode;
    styles?: StyleXArray<StyleXStyles | null | false>;
  };

export function Button({
  variant = "ghost",
  size,
  tone,
  press,
  join,
  round,
  chip,
  quiet,
  shape,
  active,
  flex,
  face: textFace,
  styles: extra,
  className,
  children,
  ref,
  ...props
}: ButtonProps) {
  const styled = stylex.props(
    dress({
      variant,
      size,
      tone,
      press,
      join,
      round,
      chip,
      quiet,
      shape,
      active,
      flex,
      face: textFace,
    }),
    extra,
  );
  return (
    <ButtonPrimitive
      {...props}
      ref={ref}
      data-slot="button"
      data-variant={variant}
      data-active={active ? "" : undefined}
      {...styled}
      className={cn(styled.className, className)}
    >
      {children}
    </ButtonPrimitive>
  );
}
