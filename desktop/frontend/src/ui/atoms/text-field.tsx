import type { IconSize } from "@/lib/iconScale";
import type { VariantProps } from "class-variance-authority";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { Icon } from "@/ui/icons";
import { Button } from "./button";
import {
  InputPrimitive,
  TextAreaPrimitive,
  type InputPrimitiveProps,
  type TextAreaPrimitiveProps,
} from "@/ui/primitives";

// `variant` answers one question: who draws the edge? `bare` means the container already
// did, so metrics stay off there — a second height here would fight the container's.
const EDGE = {
  boxed:
    "rounded-[var(--field-radius)] border-[length:var(--control-edge-width)] border-field bg-canvas focus:border-field-strong",
  bare: "border-0 bg-transparent",
} as const;

// Invalid reaches the eye through whichever edge the variant owns: recolour the border
// where there is one, add a ring where there is not.
const INVALID = {
  boxed: "border-negative focus:border-negative",
  bare: "outline outline-1 outline-negative",
} as const;

const BASE =
  "w-full min-w-0 text-ui-md text-fg outline-none transition-colors placeholder:text-fg-faint " +
  "disabled:cursor-not-allowed disabled:opacity-60";

const SHARED_VARIANTS = {
  variant: EDGE,
  font: { mono: "font-mono", sans: "font-sans" },
  invalid: { true: "", false: "" },
} as const;

const INVALID_COMPOUNDS = [
  { variant: "boxed", invalid: true, class: INVALID.boxed },
  { variant: "bare", invalid: true, class: INVALID.bare },
] as const;

const inputStyles = cva(BASE, {
  variants: {
    ...SHARED_VARIANTS,
    size: { sm: "", md: "", lg: "" },
  },
  compoundVariants: [
    { variant: "boxed", size: "sm", class: "h-[var(--field-height-sm)] px-2" },
    { variant: "boxed", size: "md", class: "h-[var(--field-height-md)] px-2.5" },
    { variant: "boxed", size: "lg", class: "h-[var(--field-height-lg)] px-3" },
    ...INVALID_COMPOUNDS,
  ],
  defaultVariants: { variant: "boxed", size: "md", font: "mono", invalid: false },
});

// A textarea's size step is inset alone — `rows` and the resize handle own the height.
// `prose` carries no inset because the composer's own density tokens place the editor.
const textAreaStyles = cva(`${BASE} resize-y leading-body`, {
  variants: {
    ...SHARED_VARIANTS,
    size: {
      sm: "px-2.5 py-1.5",
      md: "px-3 py-2",
      prose: "text-prose leading-prose",
    },
    // Native `field-sizing`, not a measure-and-write effect: the latter forces a reflow
    // per keystroke and its pixel cap stops tracking the type ladder when text size changes.
    autosize: { true: "field-sizing-content resize-none", false: "" },
  },
  compoundVariants: [...INVALID_COMPOUNDS],
  defaultVariants: {
    variant: "boxed",
    size: "md",
    font: "mono",
    invalid: false,
    autosize: false,
  },
});

type FieldVariants = VariantProps<typeof inputStyles>;

export type TextFieldProps = Omit<InputPrimitiveProps, "size" | "className"> &
  FieldVariants & { className?: string };

export function TextField({ variant, size, font, invalid, className, ...props }: TextFieldProps) {
  return (
    <InputPrimitive
      {...props}
      data-slot="text-field"
      data-variant={variant ?? "boxed"}
      className={cn(inputStyles({ variant, size, font, invalid }), className)}
    />
  );
}

// The documented Base-UI-first exemption (CLAUDE.md §4), not an oversight: Base UI ships
// no textarea part and types every handler against `HTMLInputElement`, so routing one
// through it costs a cast and buys nothing — a textarea's focus/keyboard/aria are native.
export type TextAreaProps = Omit<TextAreaPrimitiveProps, "className"> &
  VariantProps<typeof textAreaStyles> & { className?: string };

export function TextArea({
  variant,
  size,
  font,
  invalid,
  autosize,
  className,
  ...props
}: TextAreaProps) {
  return (
    <TextAreaPrimitive
      {...props}
      className={cn(textAreaStyles({ variant, size, font, invalid, autosize }), className)}
    />
  );
}

// One component rather than "wrap TextField in a div": the focus edge belongs to the whole
// composite, and hand-assembled versions each reached for a different edge.
const SEARCH_BOX = {
  sm: "h-[var(--field-height-sm)] gap-1.5 px-2",
  md: "h-[var(--field-height-md)] gap-1.5 px-2.5",
  lg: "h-[var(--field-height-lg)] gap-2 px-3",
} as const;

const SEARCH_GLYPH: Record<keyof typeof SEARCH_BOX, IconSize> = { sm: "xs", md: "sm", lg: "md" };

export type SearchFieldProps = Omit<TextFieldProps, "variant" | "invalid" | "size"> & {
  size?: keyof typeof SEARCH_BOX;
  onClear?: () => void;
  clearLabel?: string;
};

export function SearchField({
  size = "md",
  font = "sans",
  onClear,
  clearLabel,
  className,
  ...props
}: SearchFieldProps) {
  return (
    <label
      className={cn(
        "flex items-center text-fg-muted focus-within:text-fg",
        EDGE.boxed,
        "focus-within:border-field-strong",
        SEARCH_BOX[size],
        className,
      )}
    >
      <Icon name="search" size={SEARCH_GLYPH[size]} className="shrink-0" />
      <TextField {...props} type="search" variant="bare" font={font} size={size} />
      {onClear && props.value !== "" && (
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onClear}
          aria-label={clearLabel}
          className="-mr-1 shrink-0"
        >
          <Icon name="x" size="xs" />
        </Button>
      )}
    </label>
  );
}
