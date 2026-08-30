import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";

// Every variant carries a border, transparent where there is nothing to draw: without it
// a bordered variant is 2px larger than a borderless one at the same size and the two never
// share a baseline in a toolbar. Horizontal padding is 1px short of the nominal step to
// compensate.
// The `:not([class*='opacity-'])` guard lets a semantic glyph keep its own opacity.
export const buttonStyles = cva(
  [
    "inline-flex shrink-0 items-center justify-center gap-1.5 whitespace-nowrap",
    // `leading-tight`, not `leading-none`: `truncate` clips vertically too, and at a line
    // box the height of the font size the descenders fall outside it.
    "border-[length:var(--control-edge-width)] border-transparent font-sans font-medium leading-tight outline-none",
    "transition-[background-color,border-color,color,scale] duration-[var(--dur-fast)] ease-out",
    "disabled:cursor-not-allowed disabled:opacity-64 disabled:active:scale-100",
    "[&_svg:not([class*='opacity-'])]:opacity-80",
  ].join(" "),
  {
    variants: {
      variant: {
        ghost: "bg-transparent text-fg-muted hover:bg-hover hover:text-fg",
        soft: "bg-surface-2 text-fg-soft hover:bg-surface-3 hover:text-fg",
        outline: "border-field bg-transparent text-fg-soft hover:bg-hover hover:text-fg",
        primary: "bg-cta text-cta-text hover:bg-cta-hover",
        danger: "bg-transparent text-negative hover:bg-negative-wash",
        tonal: "font-semibold",
      },
      /** Only read by `variant: "tonal"`. */
      tone: {
        negative: "",
        warning: "",
      },
      size: {
        xs: "h-[var(--control-height-xs)] rounded-[var(--button-radius)] px-[7px] text-ui-sm",
        sm: "h-[var(--control-height-sm)] rounded-[var(--button-radius)] px-[9px] text-ui-md",
        md: "h-[var(--control-height-md)] rounded-[var(--button-radius)] px-[11px] text-ui-md",
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

export type ButtonProps = Omit<ButtonPrimitiveProps, "children"> &
  VariantProps<typeof buttonStyles> & {
    children?: ReactNode;
  };

export function Button({
  variant,
  size,
  tone,
  press,
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
      className={cn(buttonStyles({ variant, size, tone, press }), className)}
    >
      {children}
    </ButtonPrimitive>
  );
}
