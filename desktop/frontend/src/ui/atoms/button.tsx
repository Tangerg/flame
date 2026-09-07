import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";

const BARE = "h-auto rounded-none border-0 bg-transparent p-0 font-normal hover:bg-transparent";

export const buttonStyles = cva(
  [
    "inline-flex shrink-0 items-center justify-center gap-1.5 whitespace-nowrap",
    "border-[length:var(--control-edge-width)] border-transparent font-sans font-medium leading-tight outline-none",
    "transition-[background-color,border-color,color,scale] duration-[var(--dur-fast)] ease-out",
    "disabled:cursor-not-allowed disabled:opacity-64 disabled:active:scale-100",
    "[&_svg:not([class*='opacity-'])]:opacity-80",
  ].join(" "),
  {
    variants: {
      // The ink this control reports, at rest: a copy that just succeeded, a task that failed.
      // `tonal` reads the same word as a fill instead, and `quiet` reads it as what the control
      // will become when pointed at — see the compounds below.
      tone: {
        negative: "text-negative",
        warning: "text-warning",
        accent: "text-accent",
        success: "text-success",
      },
      size: {
        xs: "h-[var(--control-height-xs)] rounded-[var(--button-radius)] px-[7px] text-ui-sm",
        sm: "h-[var(--control-height-sm)] rounded-[var(--button-radius)] px-[9px] text-ui-md",
        md: "h-[var(--control-height-md)] rounded-[var(--button-radius)] px-[11px] text-ui-md",
        // The ladder's top step, which only the icon sizes could reach before: a text button
        // that had to stand beside a field was picking `h-9` off Tailwind's scale instead,
        // and 36px next to a 32px field is a row that does not line up.
        lg: "h-[var(--control-height-lg)] rounded-[var(--button-radius)] px-[13px] text-ui-md",
        "icon-xs":
          "h-[var(--control-height-xs)] w-[var(--control-height-xs)] rounded-[var(--button-radius)] p-0",
        "icon-sm":
          "h-[var(--control-height-sm)] w-[var(--control-height-sm)] rounded-[var(--button-radius)] p-0",
        "icon-md":
          "h-[var(--control-height-md)] w-[var(--control-height-md)] rounded-[var(--button-radius)] p-0",
        "icon-lg":
          "h-[var(--control-height-lg)] w-[var(--control-height-lg)] rounded-[var(--button-radius)] p-0",
        // The ladder's last step, which only a control laid over an image reaches: it is read
        // against a photograph rather than inside a dense row.
        "icon-xl":
          "h-[var(--control-height-xl)] w-[var(--control-height-xl)] rounded-[var(--button-radius)] p-0",
      },
      press: {
        true: "active:scale-[var(--press-scale)]",
        false: "",
      },
      /** A round button, for the places a square one would read as a plate. */
      round: { true: "rounded-full", false: "" },
      // Two buttons acting as one control: the primary action and the menu that qualifies it.
      // The seam is a hairline drawn by the trailing half rather than a border, because a
      // border would land outside the fill and read as an outline around the pair. The 1px
      // pull is what closes the gap the two edges would otherwise leave.
      join: {
        start: "rounded-r-none",
        end: [
          "relative -ml-px rounded-l-none",
          "before:pointer-events-none before:absolute before:inset-y-1.5 before:left-0",
          "before:w-px before:bg-cta-text/20",
        ].join(" "),
      },
      // Declared AFTER `size` so its neutralising classes win: `cn` is tailwind-merge, which
      // resolves a conflict in favour of the later class. Every other variant sets only ink
      // and fill, which `size` never touches, so the order is invisible to them.
      variant: {
        // The open state is Base UI's own attribute on a trigger, so a button that has opened
        // a popup says so here rather than at each of the five call sites that were saying it.
        // A button that never opens one never carries the attribute.
        ghost:
          "bg-transparent text-fg-muted hover:bg-hover hover:text-fg data-[popup-open]:bg-selected data-[popup-open]:text-fg",
        soft: "bg-surface-2 text-fg-soft hover:bg-surface-3 hover:text-fg",
        outline: "border-field bg-transparent text-fg-soft hover:bg-hover hover:text-fg",
        primary: "bg-cta text-cta-text hover:bg-cta-hover",
        danger: "bg-transparent text-negative hover:bg-negative-wash",
        // Laid over an image: the ink is the one that survives any photograph, and the fill is
        // the scrim that makes it legible. `mediaTray` is the same control inside a tray that
        // already carries the scrim, so it only takes one on hover.
        media: "bg-media-scrim text-on-media hover:bg-media-scrim",
        mediaTray: "bg-transparent text-on-media hover:bg-media-scrim",
        // Floating above the stream rather than sitting in it, so it carries a cast the flat
        // variants never do.
        raised:
          "border-0 bg-canvas text-fg-soft shadow-[var(--shadow-raised)] hover:bg-surface-2 hover:text-fg",
        tonal: "font-semibold",
        // No box at all: the button IS its text, so it takes the height of the line and none of
        // the plate. `link` is this plus the underline that says it opens something.
        bare: BARE,
        // A control that reads as prose: it sits inside a sentence, wraps with it, and says
        // it can be opened with a dotted underline rather than a plate. The hit area is a
        // pseudo-element because the text itself is only as tall as its line.
        link: [
          BARE,
          "relative inline-block cursor-pointer whitespace-normal break-words text-fg",
          "underline decoration-fg-faint decoration-dotted decoration-[1px] underline-offset-4",
          "after:absolute after:-inset-x-2 after:-inset-y-1",
          "hover:bg-transparent hover:text-fg-soft",
        ].join(" "),
      },
      // Declared after `variant` for the reason stated there: `cn` is tailwind-merge, so the
      // later class wins, and a chip's ink has to outrank the one its variant would give.
      // A control that reports a current value rather than offering an action: it keeps its
      // height so the row still lines up, but reads one step quieter and sits one step tighter.
      // Not a size step — it is the same rule at two heights, so it adjusts the ladder instead
      // of doubling it.
      chip: { true: "text-fg-soft", false: "" },
      // A control that steps back until it is needed. With a `tone`, the tone moves to hover:
      // the button rests faint and shows what it will do only when the pointer is on it.
      quiet: { true: "text-fg-faint", false: "" },
      // Two strengths of "off". The default says the control is unavailable right now; `faded`
      // says it does not apply at all — attaching an image to a model that cannot read one.
      off: { normal: "", faded: "disabled:opacity-25" },
    },
    compoundVariants: [
      { chip: true, size: "sm", class: "px-1.5 text-ui-sm" },
      { chip: true, size: "md", class: "px-2 text-ui-sm" },
      { quiet: true, tone: "accent", class: "hover:text-accent" },
      { quiet: true, tone: "negative", class: "hover:text-negative" },
      { quiet: true, tone: "success", class: "hover:text-success" },
      { quiet: true, tone: "warning", class: "hover:text-warning" },
      // A filled action that cannot act reads as broken at 64% of its own fill, so it takes a
      // neutral plate instead — the composer's send button had been spelling this out.
      { variant: "primary", class: "disabled:bg-surface-2 disabled:text-fg-faint" },
      {
        variant: "tonal",
        tone: "negative",
        class: "bg-negative-wash text-negative hover:bg-negative-badge",
      },
      {
        variant: "tonal",
        tone: "warning",
        class: "bg-warning-wash text-warning hover:bg-warning-badge",
      },
    ],
    defaultVariants: {
      variant: "ghost",
      size: "md",
      press: true,
    },
  },
);

// `data-slot` / `data-variant` are set after the spread, so a caller's would be dropped
// silently. Omitted from the props type to make that a compile error.
export type ButtonProps = Omit<ButtonPrimitiveProps, "children" | "data-slot" | "data-variant"> &
  VariantProps<typeof buttonStyles> & {
    children?: ReactNode;
  };

export function Button({
  variant,
  size,
  tone,
  press,
  join,
  round,
  chip,
  quiet,
  off,
  className,
  children,
  ref,
  ...props
}: ButtonProps) {
  const resolvedVariant = variant ?? "ghost";
  return (
    <ButtonPrimitive
      {...props}
      ref={ref}
      data-slot="button"
      data-variant={resolvedVariant}
      className={cn(
        buttonStyles({ variant, size, tone, press, join, round, chip, quiet, off }),
        className,
      )}
    >
      {children}
    </ButtonPrimitive>
  );
}
