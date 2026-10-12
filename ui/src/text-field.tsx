import * as stylex from "@stylexjs/stylex";
import type { IconSize } from "./font-scale";
import { clsx as cn } from "clsx";
import {
  color,
  leading,
  motion,
  radius,
  space,
  surface,
  type,
} from "./tokens.stylex";
import { Icon } from "./font-scale";
import { Search, X } from "@flame/icons/react";
import { Button } from "./button";
import { WELL_SURFACE } from "./well-surface";
import {
  InputPrimitive,
  TextAreaPrimitive,
  type InputPrimitiveProps,
  type TextAreaPrimitiveProps,
} from "./primitives/index";

type FieldEdge = "boxed" | "bare" | "inline";

type FieldInk = "default" | "soft";

const styles = stylex.create({
  base: {
    width: "100%",
    minWidth: 0,
    transitionProperty: "color, background-color, border-color, outline-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
    "::placeholder": { color: color.fgFaint },
    cursor: {
      default: null,
      ':is(:disabled, [aria-disabled="true"])': "not-allowed",
    },
    opacity: {
      default: null,
      ':is(:disabled, [aria-disabled="true"])':
        "var(--control-disabled-opacity)",
    },
  },
  boxed: {
    borderRadius: radius.field,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: { default: surface.controlEdge, ":focus": surface.fieldFocus },
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
  invalidBoxed: {
    borderColor: { default: color.negative, ":focus": color.negative },
  },
  invalidBare: { outline: `1px solid ${color.negative}` },
  numeric: { fontVariantNumeric: "tabular-nums" },
  faceMono: { fontFamily: "var(--font-mono)" },
  faceSans: { fontFamily: "var(--font-sans)" },
  input: { lineHeight: leading.tight },
  inputSm: { minHeight: "var(--control-height-sm)", paddingInline: space.s2 },
  inputMd: { minHeight: "var(--control-height-md)", paddingInline: space.s2_5 },
  inputLg: { minHeight: "var(--control-height-lg)", paddingInline: space.s3 },
  area: { resize: "vertical", lineHeight: leading.body },
  areaProse: {
    lineHeight: "calc(1em + 6px)",
    "::placeholder": { letterSpacing: "normal" },
  },
  areaSm: { paddingInline: space.s2_5, paddingBlock: space.s1_5 },
  areaMd: { paddingInline: space.s3, paddingBlock: space.s2 },
  autosize: { fieldSizing: "content", resize: "none" },
  searchBox: {
    display: "flex",
    width: "min(100%,20rem)",
    minWidth: 0,
    maxWidth: "100%",
    alignItems: "center",
    borderRadius: radius.field,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: {
      default: surface.controlEdge,
      ":focus-within": surface.fieldFocus,
    },
    backgroundColor: surface.canvas,
    color: { default: color.fgMuted, ":focus-within": color.fg },
    transitionProperty: "color, border-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  searchSm: {
    height: "var(--control-height-sm)",
    gap: space.s1_5,
    paddingInline: space.s2,
  },
  searchMd: {
    height: "var(--control-height-md)",
    gap: space.s1_5,
    paddingInline: space.s2_5,
  },
  searchLg: {
    height: "var(--control-height-lg)",
    gap: space.s2,
    paddingInline: space.s3,
  },
  glyph: { flexShrink: 0 },
  clear: { marginRight: "calc(var(--spacing) * -1)", flexShrink: 0 },
});

const FACE = { mono: styles.faceMono, sans: styles.faceSans } as const;
const INK = { default: styles.inkDefault, soft: styles.inkSoft } as const;
const INPUT_SIZE = {
  sm: styles.inputSm,
  md: styles.inputMd,
  lg: styles.inputLg,
} as const;
const SEARCH_SIZE = {
  sm: styles.searchSm,
  md: styles.searchMd,
  lg: styles.searchLg,
} as const;

type FieldSize = keyof typeof INPUT_SIZE;

type SharedProps = {
  variant?: FieldEdge;
  font?: keyof typeof FACE;
  ink?: FieldInk;
  invalid?: boolean;
  className?: string;
  pending?: boolean;
};

function edge(variant: FieldEdge, invalid: boolean) {
  return [
    styles[variant],
    invalid && (variant === "boxed" ? styles.invalidBoxed : styles.invalidBare),
  ];
}

export type TextFieldProps = Omit<
  InputPrimitiveProps,
  "size" | "className" | "aria-invalid"
> &
  SharedProps & { size?: FieldSize };

export function TextField({
  variant = "boxed",
  size = "md",
  font = "sans",
  ink = "default",
  invalid = false,
  className,
  pending,
  ...props
}: TextFieldProps) {
  const styled = stylex.props(
    styles.base,
    styles.input,
    type.uiMd,
    FACE[font],
    INK[ink],
    ...edge(variant, invalid),
    variant === "boxed" && INPUT_SIZE[size],
    props.type === "number" && styles.numeric,
  );
  return (
    <InputPrimitive
      {...props}
      aria-invalid={invalid || undefined}
      data-chrome-focus={
        variant === "boxed" || "data-chrome-focus" in props ? "" : undefined
      }
      readOnly={pending || props.readOnly}
      aria-disabled={pending ? true : props["aria-disabled"]}
      data-slot="text-field"
      data-variant={variant}
      {...styled}
      className={cn(styled.className, className)}
    />
  );
}

type AreaSize = "sm" | "md" | "prose";

export type TextAreaProps = Omit<
  TextAreaPrimitiveProps,
  "className" | "aria-invalid"
> &
  Omit<SharedProps, "variant"> & {
    variant?: FieldEdge | "well";
    size?: AreaSize;
    autosize?: boolean;
  };

export function TextArea({
  variant = "boxed",
  size = "md",
  font = "sans",
  ink = "default",
  invalid = false,
  autosize = false,
  className,
  pending,
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
    size === "prose"
      ? [type.prose, styles.areaProse]
      : styles[size === "sm" ? "areaSm" : "areaMd"],
    well && [WELL_SURFACE.face, type.code],
    autosize && styles.autosize,
  );
  return (
    <TextAreaPrimitive
      {...props}
      aria-invalid={invalid || undefined}
      data-chrome-focus={
        variant === "boxed" || "data-chrome-focus" in props ? "" : undefined
      }
      readOnly={pending || props.readOnly}
      aria-disabled={pending ? true : props["aria-disabled"]}
      {...styled}
      className={cn(styled.className, className)}
    />
  );
}

const SEARCH_GLYPH: Record<FieldSize, IconSize> = {
  sm: "xs",
  md: "sm",
  lg: "md",
};

export type SearchFieldProps = Omit<
  TextFieldProps,
  "variant" | "invalid" | "size"
> & {
  size?: FieldSize;
} & (
    | { onClear: () => void; clearLabel: string }
    | { onClear?: never; clearLabel?: never }
  );

export function SearchField({
  size = "md",
  font = "sans",
  onClear,
  clearLabel,
  className,
  ...props
}: SearchFieldProps) {
  const box = stylex.props(styles.searchBox, SEARCH_SIZE[size]);
  return (
    <div {...box} className={cn(box.className, className)}>
      <Icon
        glyph={Search}
        size={SEARCH_GLYPH[size]}
        {...stylex.props(styles.glyph)}
      />
      <TextField
        {...props}
        type="search"
        variant="bare"
        font={font}
        size={size}
        data-chrome-focus=""
      />
      {onClear && props.value !== "" && (
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onClear}
          aria-label={clearLabel}
          styles={styles.clear}
        >
          <Icon glyph={X} size="xs" />
        </Button>
      )}
    </div>
  );
}
