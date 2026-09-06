import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";

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
      tone: {
        negative: "",
        warning: "",
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
      },
      press: {
        true: "active:scale-[var(--press-scale)]",
        false: "",
      },
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
        ghost: "bg-transparent text-fg-muted hover:bg-hover hover:text-fg",
        soft: "bg-surface-2 text-fg-soft hover:bg-surface-3 hover:text-fg",
        outline: "border-field bg-transparent text-fg-soft hover:bg-hover hover:text-fg",
        primary: "bg-cta text-cta-text hover:bg-cta-hover",
        danger: "bg-transparent text-negative hover:bg-negative-wash",
        tonal: "font-semibold",
        // A control that reads as prose: it sits inside a sentence, wraps with it, and says
        // it can be opened with a dotted underline rather than a plate. The hit area is a
        // pseudo-element because the text itself is only as tall as its line.
        link: [
          "relative inline-block h-auto rounded-none border-0 bg-transparent p-0",
          "cursor-pointer font-normal whitespace-normal break-words text-fg",
          "underline decoration-fg-faint decoration-dotted decoration-[1px] underline-offset-4",
          "after:absolute after:-inset-x-2 after:-inset-y-1",
          "hover:bg-transparent hover:text-fg-soft",
        ].join(" "),
      },
    },
    compoundVariants: [
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
      className={cn(buttonStyles({ variant, size, tone, press, join }), className)}
    >
      {children}
    </ButtonPrimitive>
  );
}
