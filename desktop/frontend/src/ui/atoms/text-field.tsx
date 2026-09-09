import * as stylex from "@stylexjs/stylex";
import type { IconSize } from "@/lib/iconScale";
import { cn } from "@/lib/classNames";
import { color, leading, motion, radius, space, surface, type } from "@/styles/tokens.stylex";
import { Icon } from "@/ui/icons";
import { Button } from "./button";
import { WELL_SURFACE } from "./well";
import {
  InputPrimitive,
  TextAreaPrimitive,
  type InputPrimitiveProps,
  type TextAreaPrimitiveProps,
} from "@/ui/primitives";

/**
 * How much chrome a field carries.
 *
 * `boxed` is the standing form. `bare` is a field inside something that already has an edge —
 * a search box, a composer, a card that IS the input. `inline` edits a value in place inside a
 * row: it takes no chrome either, but must still read as editable, which is why it has a fill
 * the row does not. The sidebar's title editor was spelling that out as `bg-surface-3
 * rounded-xs px-1` on top of `bare`, whose `background: transparent` StyleX would have won.
 */
type FieldEdge = "boxed" | "bare" | "inline";

/** Ink, in the vocabulary `Well` already uses — a textarea can BE a well's editable face. */
type FieldInk = "default" | "soft";

const styles = stylex.create({
  base: {
    width: "100%",
    minWidth: 0,
    transitionProperty: "color, background-color, border-color, outline-color",
    transitionDuration: motion.color,
    transitionTimingFunction: "var(--ease-out)",
    "::placeholder": { color: color.fgFaint },
    cursor: { default: null, ":disabled": "not-allowed" },
    opacity: { default: null, ":disabled": "var(--control-disabled-opacity)" },
  },
  boxed: {
    borderRadius: radius.field,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: { default: surface.field, ":focus": surface.fieldStrong },
    backgroundColor: surface.canvas,
  },
  bare: { borderWidth: 0, backgroundColor: "transparent" },
  inline: {
    borderWidth: 0,
    borderRadius: radius.xs,
    backgroundColor: surface.surface3,
    paddingInline: space.s1,
    lineHeight: leading.body,
  },
  inkDefault: { color: color.fg },
  inkSoft: { color: color.fgSoft },
  // A field the caller flagged invalid. `boxed` recolours its own edge; `bare` has none to
  // recolour, so it borrows an outline — the one place a field draws a ring of its own.
  invalidBoxed: { borderColor: { default: color.negative, ":focus": color.negative } },
  invalidBare: { outline: `1px solid ${color.negative}` },
  // Figures in a column have to line up, and a numeric field is always a column of one.
  numeric: { fontVariantNumeric: "tabular-nums" },
  faceMono: { fontFamily: "var(--font-mono)" },
  faceSans: { fontFamily: "var(--font-sans)" },
  inputSm: { height: "var(--control-height-sm)", paddingInline: space.s2 },
  inputMd: { height: "var(--control-height-md)", paddingInline: space.s2_5 },
  inputLg: { height: "var(--control-height-lg)", paddingInline: space.s3 },
  area: { resize: "vertical", lineHeight: leading.body },
  // The prose step brings prose tracking, which is right for what is typed and wrong for the
  // placeholder: a placeholder is UI text, not prose. The composer had reset this at the call
  // site, where the next prose textarea would have had to rediscover it.
  areaProse: { lineHeight: leading.prose, "::placeholder": { letterSpacing: "normal" } },
  areaSm: { paddingInline: space.s2_5, paddingBlock: space.s1_5 },
  areaMd: { paddingInline: space.s3, paddingBlock: space.s2 },
  autosize: { fieldSizing: "content", resize: "none" },
  searchBox: {
    display: "flex",
    alignItems: "center",
    color: { default: color.fgMuted, ":focus-within": color.fg },
    borderColor: { default: surface.field, ":focus-within": surface.fieldStrong },
  },
  searchSm: { height: "var(--control-height-sm)", gap: space.s1_5, paddingInline: space.s2 },
  searchMd: { height: "var(--control-height-md)", gap: space.s1_5, paddingInline: space.s2_5 },
  searchLg: { height: "var(--control-height-lg)", gap: space.s2, paddingInline: space.s3 },
  glyph: { flexShrink: 0 },
  clear: { marginRight: "calc(var(--spacing) * -1)", flexShrink: 0 },
});

const FACE = { mono: styles.faceMono, sans: styles.faceSans } as const;
const INK = { default: styles.inkDefault, soft: styles.inkSoft } as const;
const INPUT_SIZE = { sm: styles.inputSm, md: styles.inputMd, lg: styles.inputLg } as const;
const SEARCH_SIZE = { sm: styles.searchSm, md: styles.searchMd, lg: styles.searchLg } as const;

type FieldSize = keyof typeof INPUT_SIZE;

type SharedProps = {
  variant?: FieldEdge;
  font?: keyof typeof FACE;
  ink?: FieldInk;
  invalid?: boolean;
  className?: string;
};

function edge(variant: FieldEdge, invalid: boolean) {
  return [
    styles[variant],
    invalid && (variant === "boxed" ? styles.invalidBoxed : styles.invalidBare),
  ];
}

export type TextFieldProps = Omit<InputPrimitiveProps, "size" | "className"> &
  SharedProps & { size?: FieldSize };

export function TextField({
  variant = "boxed",
  size = "md",
  font = "mono",
  ink = "default",
  invalid = false,
  className,
  ...props
}: TextFieldProps) {
  const styled = stylex.props(
    styles.base,
    type.uiMd,
    FACE[font],
    INK[ink],
    ...edge(variant, invalid),
    // Only `boxed` states a height: the other two are sized by the thing that contains them.
    variant === "boxed" && INPUT_SIZE[size],
    props.type === "number" && styles.numeric,
  );
  return (
    <InputPrimitive
      {...props}
      data-slot="text-field"
      data-variant={variant}
      {...styled}
      className={cn(styled.className, className)}
    />
  );
}

/** A textarea's own steps. `prose` is the composer's: a reading measure, not a control step. */
type AreaSize = "sm" | "md" | "prose";

export type TextAreaProps = Omit<TextAreaPrimitiveProps, "className"> &
  Omit<SharedProps, "variant"> & {
    /** `well` is `bare` wearing the recessed face, so the block and its editor cannot drift. */
    variant?: FieldEdge | "well";
    size?: AreaSize;
    autosize?: boolean;
  };

export function TextArea({
  variant = "boxed",
  size = "md",
  font = "mono",
  ink = "default",
  invalid = false,
  autosize = false,
  className,
  ...props
}: TextAreaProps) {
  const well = variant === "well";
  const styled = stylex.props(
    styles.base,
    styles.area,
    type.uiMd,
    FACE[font],
    INK[ink],
    ...edge(variant === "well" ? "bare" : variant, invalid),
    size === "prose" ? [type.prose, styles.areaProse] : styles[size === "sm" ? "areaSm" : "areaMd"],
    // After the size step, so the well's own padding and face win over it.
    well && [WELL_SURFACE.face, type.code],
    autosize && styles.autosize,
  );
  return <TextAreaPrimitive {...props} {...styled} className={cn(styled.className, className)} />;
}

const SEARCH_GLYPH: Record<FieldSize, IconSize> = { sm: "xs", md: "sm", lg: "md" };

export type SearchFieldProps = Omit<TextFieldProps, "variant" | "invalid" | "size"> & {
  size?: FieldSize;
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
  const box = stylex.props(styles.searchBox, styles.boxed, SEARCH_SIZE[size]);
  return (
    <label {...box} className={cn(box.className, className)}>
      <Icon name="search" size={SEARCH_GLYPH[size]} {...stylex.props(styles.glyph)} />
      <TextField {...props} type="search" variant="bare" font={font} size={size} />
      {onClear && props.value !== "" && (
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onClear}
          aria-label={clearLabel}
          {...stylex.props(styles.clear)}
        >
          <Icon name="x" size="xs" />
        </Button>
      )}
    </label>
  );
}
