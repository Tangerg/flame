import * as stylex from "@stylexjs/stylex";
import type { StyleXArray, StyleXStyles } from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
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
} from "@/styles/tokens.stylex";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";
import { toneInk } from "./tone-ink";

/**
 * The product's one button, in the shapes and inks its call sites proved it needed.
 *
 * ORDER IS LOAD-BEARING. `stylex.props` resolves a property to whichever style declares it last,
 * so the sequence in `dress()` below is the design's precedence, spelled once: size states a
 * corner, `round` replaces it, `join` flattens one side of it, a boxless variant removes it
 * entirely, and `shape="row"` states the row's own. The same for ink: a variant sets it, `chip`
 * softens it, `quiet` withdraws it, and a `tone` beside `quiet` moves it to hover.
 *
 * A caller's `className` composes after all of it, but it cannot outrank any property declared
 * here — a generated selector carries `:not(#\#)` specificity. That is the point: an override
 * that the design has an answer for is a design gap, and the steps below are the ones that
 * turned into a step rather than staying spelled out at a call site.
 *
 * Two rules are NOT here. Every glyph inside a button sits a step back from its label, which is
 * a DESCENDANT rule that no atomic class can express; and the browser's own button chrome is
 * turned off a ring below. Both live in `globals.css`, the second under `@layer base` so that
 * this ring can simply state a border or a fill instead of out-specifying one.
 */
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

type ButtonTone = "negative" | "warning" | "accent" | "success";

const styles = stylex.create({
  base: {
    display: "inline-flex",
    flexShrink: 0,
    alignItems: "center",
    justifyContent: "center",
    gap: space.s1_5,
    whiteSpace: "nowrap",
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: "transparent",
    fontFamily: "var(--font-sans)",
    fontWeight: weight.medium,
    lineHeight: leading.tight,
    outline: "none",
    // One list, and it names every property a button animates: a press scales it, a reveal
    // fades it, a floating one slides in. A transition-property declaration is the whole list,
    // so a call site cannot add to it — it can only replace it.
    transitionProperty: "background-color, border-color, color, opacity, scale, translate",
    transitionDuration: motion.fast,
    transitionTimingFunction: "var(--ease-out)",
  },

  xs: { height: "var(--control-height-xs)", borderRadius: radius.button, paddingInline: "7px" },
  sm: { height: "var(--control-height-sm)", borderRadius: radius.button, paddingInline: "9px" },
  md: { height: "var(--control-height-md)", borderRadius: radius.button, paddingInline: "11px" },
  // The ladder's top text step: a button that stands beside a field has to match its height.
  lg: { height: "var(--control-height-lg)", borderRadius: radius.button, paddingInline: "13px" },
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
  // The ladder's last step, which only a control laid over an image reaches: it is read against
  // a photograph rather than inside a dense row.
  iconXl: {
    height: "var(--control-height-xl)",
    width: "var(--control-height-xl)",
    borderRadius: radius.button,
    padding: 0,
  },

  // How a press is answered, and neither step answers one a disabled control cannot accept.
  press: {
    scale: { default: null, ":active": "var(--press-scale)", ":is(:disabled):active": 1 },
  },
  // The filled circle's step: a plate that shrinks reads as a bug rather than a press, and a
  // half-pixel drop reads as one. It had been a string constant in the composer, which made
  // "how a press is answered" two mechanisms — one here and one in a plugin.
  nudge: {
    translate: { default: null, ":active": "0 0.5px", ":is(:disabled):active": null },
  },

  ghost: {
    backgroundColor: {
      default: "transparent",
      ":hover": surface.hover,
      // `data-chrome-focus` is the design system's way of saying "a row state stands in for the
      // focus ring here". A control that opts out of the ring has to show something else, so
      // the button honours the marker rather than leaving each call site to remember.
      ":is([data-chrome-focus]):focus-visible": surface.hover,
      // Base UI's own attribute on a trigger, so a button that has opened a popup says so here.
      ":is([data-popup-open])": surface.selected,
    },
    color: { default: color.fgMuted, ":hover": color.fg, ":is([data-popup-open])": color.fg },
  },
  soft: {
    backgroundColor: { default: surface.surface2, ":hover": surface.surface3 },
    color: { default: color.fgSoft, ":hover": color.fg },
  },
  outline: {
    borderColor: surface.field,
    backgroundColor: { default: "transparent", ":hover": surface.hover },
    color: { default: color.fgSoft, ":hover": color.fg },
  },
  primary: {
    backgroundColor: {
      default: surface.ctaFill,
      ":hover": surface.ctaHover,
      // A filled action that cannot act reads as broken at 64% of its own fill, so it takes a
      // neutral plate instead — the composer's send button had been spelling this out.
      ":disabled": surface.surface2,
    },
    color: { default: color.ctaText, ":disabled": color.fgFaint },
    // …and having answered, it does not answer twice. The fade a ring below applies is the
    // GENERIC way to say "cannot be used", for a control with no answer of its own. Stacked on
    // this plate it took the glyph from 4.9:1 to 1.9:1 — a disabled control still has to be
    // readable, because reading it is how you work out what would enable it.
    opacity: { default: null, ":disabled": 1 },
  },
  // The action wears its consequence: no plate at rest, the tone's own wash under the pointer.
  // The tone decides which, so this is one rule rather than a variant named after a colour.
  wash: { backgroundColor: "transparent" },
  washNegative: {
    backgroundColor: { default: "transparent", ":hover": surface.negativeWash },
    color: color.negative,
  },
  washWarning: {
    backgroundColor: { default: "transparent", ":hover": surface.warningWash },
    color: color.warning,
  },
  tonal: { fontWeight: weight.semibold },
  tonalNegative: {
    backgroundColor: { default: surface.negativeWash, ":hover": surface.negativeBadge },
    color: color.negative,
  },
  tonalWarning: {
    backgroundColor: { default: surface.warningWash, ":hover": surface.warningBadge },
    color: color.warning,
  },
  // Laid over an image: the ink is the one that survives any photograph, and the fill is the
  // scrim that makes it legible. `mediaTray` sits in a tray that already carries the scrim.
  media: { backgroundColor: surface.mediaScrim, color: color.onMedia },
  mediaTray: {
    backgroundColor: { default: "transparent", ":hover": surface.mediaScrim },
    color: color.onMedia,
  },
  // Floating above the stream rather than sitting in it, so it carries a cast the flat variants
  // never do.
  raised: {
    borderWidth: 0,
    backgroundColor: { default: surface.canvas, ":hover": surface.surface2 },
    color: { default: color.fgSoft, ":hover": color.fg },
    boxShadow: "var(--shadow-raised)",
  },
  // No box at all: the button IS its text, so it takes the height of the line, none of the
  // plate, and starts where the text starts. `link` is this plus the underline.
  bare: {
    height: "auto",
    justifyContent: "flex-start",
    borderRadius: 0,
    borderWidth: 0,
    backgroundColor: { default: "transparent", ":hover": "transparent" },
    // On the SAME axis the sizes use. A physical `padding: 0` and a logical `padding-inline`
    // expand to different longhands, so the shorthand would not have replaced the step's inset
    // — it would have sat beside it and lost.
    paddingInline: 0,
    paddingBlock: 0,
    // A button that IS its text takes the line height of the sentence it sits in, not the
    // tight one a control's box needs.
    lineHeight: "inherit",
    fontWeight: weight.regular,
  },
  // A control that reads as prose: it sits inside a sentence, wraps with it, and says it can be
  // opened with a dotted underline rather than a plate. The hit area is a pseudo-element because
  // the text itself is only as tall as its line.
  link: {
    position: "relative",
    display: "inline-block",
    cursor: "pointer",
    whiteSpace: "normal",
    overflowWrap: "break-word",
    color: { default: color.fg, ":hover": color.fgSoft },
    textDecorationLine: "underline",
    textDecorationColor: color.fgFaint,
    textDecorationStyle: "dotted",
    textDecorationThickness: "1px",
    textUnderlineOffset: "4px",
    "::after": { content: "", position: "absolute", inset: "-4px -8px" },
  },

  // A control that reports a current value rather than offering an action: it keeps its height
  // so the row still lines up, but reads one step quieter and sits one step tighter.
  chip: { color: color.fgSoft },
  chipSm: { paddingInline: space.s1_5 },
  chipMd: { paddingInline: space.s2 },

  // A control that steps back until it is needed. With a `tone`, the tone moves to hover.
  quiet: { color: color.fgFaint },
  quietAccent: { color: { default: color.fgFaint, ":hover": color.accent } },
  quietNegative: { color: { default: color.fgFaint, ":hover": color.negative } },
  quietSuccess: { color: { default: color.fgFaint, ":hover": color.success } },
  quietWarning: { color: { default: color.fgFaint, ":hover": color.warning } },

  // Two strengths of "off": unavailable right now, versus does not apply here at all.
  faded: { opacity: { default: null, ":disabled": 0.25 } },

  // Most buttons hold their width in a row; one that stands in for a field takes the space
  // instead. The parent decides that, so it is a step rather than something a class adds.
  fill: { flexGrow: 1, flexShrink: 1, flexBasis: 0, minWidth: 0 },

  // A toggle that is on. Declared after the variants so it outranks whichever fill they gave.
  active: { backgroundColor: surface.selected, color: color.fg },

  // A control that is a row in a list rather than a button in a bar.
  row: {
    width: "100%",
    justifyContent: "flex-start",
    borderRadius: radius.row,
    fontWeight: weight.regular,
  },

  // Two buttons acting as one control. The seam is a hairline drawn by the trailing half rather
  // than a border, because a border would land outside the fill and read as an outline around
  // the pair. The 1px pull is what closes the gap the two edges would otherwise leave.
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

const TONAL = { negative: styles.tonalNegative, warning: styles.tonalWarning } as const;
const WASH = { negative: styles.washNegative, warning: styles.washWarning } as const;

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
  /** Left unset this follows the box, which is what decides it — see `pressFor`. */
  press?: "scale" | "nudge" | "none";
  join?: "start" | "end";
  round?: boolean;
  chip?: boolean;
  quiet?: boolean;
  off?: "normal" | "faded";
  shape?: "control" | "row";
  /** A toggle that is on: the wrap control on a code block, the open panel in a bar. */
  active?: boolean;
  /** `fill` lets the button take the row's spare space instead of holding its own width. */
  flex?: "none" | "fill";
  /** Machine text takes the mono face and the tracking that belongs with it. */
  face?: "text" | "mono";
}

// A press is answered by the BOX, so the box decides whether there is an answer to give.
// `link` and `bare` have no box — they are a run of text, and scaling text reads as a glitch.
// A row's box is the whole row, where two percent moves each edge five pixels and looks like
// the layout breathing. A chip's box is too small for two percent to read at all. Every one of
// these had been turning the press off at the call site, all four of them unanimously.
// The exception is the saturated disc — the composer's send and stop: a solid accent plate that
// shrinks reads as a bug rather than a press, and a half-pixel drop reads as one. That argument
// had been made in a comment beside a plugin-level string constant, which left "how a press is
// answered" with two mechanisms. A `raised` circle is not this case: its fill is the canvas, so
// it takes the scale like any other box.
function pressFor({ variant, chip, shape, round }: ButtonVariants): "scale" | "nudge" | "none" {
  if (variant === "link" || variant === "bare") return "none";
  if (round && variant === "primary") return "nudge";
  return chip || shape === "row" ? "none" : "scale";
}

/** The precedence, in one place. Read it top to bottom: later decides. */
export function dress({
  variant = "ghost",
  size = "md",
  tone,
  press,
  join,
  round,
  chip,
  quiet,
  off,
  shape,
  active,
  flex,
  face: textFace,
}: ButtonVariants) {
  const chipStep = chip && (size === "sm" || size === "md") ? size : null;
  const pressStep = press ?? pressFor({ variant, chip, shape, round });
  return [
    styles.base,
    tone && toneInk[tone],
    SIZE[size],
    pressStep === "scale" && styles.press,
    pressStep === "nudge" && styles.nudge,
    round && corner.pill,
    join === "start" && styles.joinStart,
    join === "end" && styles.joinEnd,
    VARIANT[variant],
    variant === "tonal" && tone && tone in TONAL && TONAL[tone as keyof typeof TONAL],
    variant === "wash" && tone && tone in WASH && WASH[tone as keyof typeof WASH],
    chip && styles.chip,
    chipStep && [CHIP_INSET[chipStep], type.uiSm],
    quiet && styles.quiet,
    quiet && tone && QUIET_TONE[tone],
    off === "faded" && styles.faded,
    active && styles.active,
    shape === "row" && styles.row,
    flex === "fill" && styles.fill,
    textFace && face[textFace],
  ];
}

// `data-slot` / `data-variant` are set after the spread, so a caller's would be dropped
// silently. Omitted from the props type to make that a compile error.
export type ButtonProps = Omit<ButtonPrimitiveProps, "children" | "data-slot" | "data-variant"> &
  ButtonVariants & {
    children?: ReactNode;
    /**
     * StyleX styles composed INTO this button's own, for a shell shape the design system builds
     * on top of it — the agent row's density height and inset, say.
     *
     * Not an escape hatch: it is composed in the same `stylex.props()` call, so a property it
     * declares replaces this component's rather than losing to it, which is exactly what a
     * `className` cannot do. A business call site that needs a shape asks for a step instead.
     */
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
  off,
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
      off,
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
