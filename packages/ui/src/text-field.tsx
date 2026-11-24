import * as stylex from "@stylexjs/stylex";
import type { StyleXArray, StyleXStyles } from "@stylexjs/stylex";
import { useCallback, useRef, type KeyboardEvent } from "react";
import type { IconSize } from "./icon-scale";
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
import { isImeKey } from "./keyboard";
import { FIELD_SIZE, FIELD_SURFACE } from "./control-surface";
import { WELL_SURFACE } from "./well-surface";
import type { ComponentPropsWithRef } from "react";

type FieldEdge = "boxed" | "bare" | "inline";

type FieldInk = "default" | "soft";

const styles = stylex.create({
  base: {
    appearance: "none",
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
  bare: { borderWidth: 0, backgroundColor: "transparent" },
  // Under a form label (FormField sets --field-hang) the fill bleeds so the
  // field's text starts on the label's edge.
  hang: {
    marginInlineStart: {
      default: 0,
      ":first-child": "calc(-1 * var(--field-hang, 0px))",
    },
    width: {
      default: "100%",
      ":first-child": "calc(100% + var(--field-hang, 0px))",
    },
  },
  inline: {
    borderWidth: 0,
    borderRadius: radius.xs,
    backgroundColor: surface.surface3,
    paddingInline: space.s1,
    lineHeight: leading.body,
  },
  inkDefault: { color: color.fg },
  inkSoft: { color: color.fgSoft },
  invalidBare: { boxShadow: `inset 0 0 0 1px ${color.negative}` },
  numeric: { fontVariantNumeric: "tabular-nums" },
  faceMono: { fontFamily: "var(--font-mono)" },
  faceSans: { fontFamily: "var(--font-sans)" },
  input: { lineHeight: leading.tight },
  area: {
    resize: "vertical",
    lineHeight: leading.body,
    borderRadius: radius.field,
  },
  areaProse: {
    lineHeight: "calc(1em + 6px)",
    "::placeholder": { letterSpacing: "normal" },
  },
  areaSm: { paddingBlock: space.s1_5 },
  areaMd: { paddingBlock: space.s2 },
  autosize: {
    resize: "vertical",
    minHeight: "2lh",
    maxHeight: "min(24lh, 60dvh)",
    overflowY: "auto",
    fieldSizing: {
      default: null,
      "@supports (field-sizing: content)": "content",
    },
  },
  searchBox: {
    display: "flex",
    width: "100%",
    minWidth: 0,
    maxWidth: "100%",
    alignItems: "center",
    color: { default: color.fgMuted, ":focus-within": color.fg },
  },
  searchSm: { gap: space.s1_5 },
  searchMd: { gap: space.s1_5 },
  searchLg: { gap: space.s2 },
  customClear: { "::-webkit-search-cancel-button": { display: "none" } },
  glyph: { flexShrink: 0 },
  clear: { marginInlineEnd: "calc(var(--spacing) * -1)", flexShrink: 0 },
});

const FACE = { mono: styles.faceMono, sans: styles.faceSans } as const;
const INK = { default: styles.inkDefault, soft: styles.inkSoft } as const;
const SEARCH_SIZE = {
  sm: styles.searchSm,
  md: styles.searchMd,
  lg: styles.searchLg,
} as const;

type FieldSize = keyof typeof FIELD_SIZE;

type SharedProps = {
  variant?: FieldEdge;
  font?: keyof typeof FACE;
  ink?: FieldInk;
  invalid?: boolean;
  className?: string;
  styles?: StyleXArray<StyleXStyles | null | false>;
  pending?: boolean;
};

function edge(variant: FieldEdge, invalid: boolean) {
  return [
    variant === "boxed" ? [FIELD_SURFACE.face, styles.hang] : styles[variant],
    invalid && variant !== "boxed" && styles.invalidBare,
  ];
}

export type TextFieldProps = Omit<
  ComponentPropsWithRef<"input">,
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
  styles: extra,
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
    variant === "boxed" && FIELD_SIZE[size],
    props.type === "number" && styles.numeric,
    extra,
  );
  return (
    <input
      {...props}
      aria-invalid={invalid || undefined}
      {...(invalid && { "data-focus-outset": "" })}
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
  ComponentPropsWithRef<"textarea">,
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
  styles: extra,
  pending,
  ref,
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
    variant === "boxed" && FIELD_SIZE[size === "sm" ? "sm" : "md"],
    size === "prose"
      ? [type.prose, styles.areaProse]
      : styles[size === "sm" ? "areaSm" : "areaMd"],
    well && [WELL_SURFACE.face, type.code],
    autosize && styles.autosize,
    extra,
  );
  return (
    <textarea
      {...props}
      ref={ref}
      rows={props.rows ?? (autosize ? 4 : undefined)}
      aria-invalid={invalid || undefined}
      {...(invalid && { "data-focus-outset": "" })}
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
  "variant" | "invalid" | "size" | "value" | "defaultValue" | "type"
> & {
  value: string;
  defaultValue?: never;
  size?: FieldSize;
  // A popup's only input holds focus for the popup's whole life, so its caret
  // already marks focus and a ring would only restate that the popup is open.
  ring?: boolean;
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
  styles: extra,
  ref,
  onKeyDown,
  ring = true,
  ...props
}: SearchFieldProps) {
  const element = useRef<HTMLInputElement>(null);
  const setElement = useCallback(
    (node: HTMLInputElement | null) => {
      element.current = node;
      if (typeof ref === "function") return ref(node);
      if (ref) ref.current = node;
    },
    [ref],
  );
  // A nonempty query owns Escape before any enclosing navigation; only an
  // empty field lets it reach the host. Without onClear the browser's native
  // search cancel performs the clear, so its default must survive.
  const clearOnEscape = (event: KeyboardEvent<HTMLElement>) => {
    if (
      event.key !== "Escape" ||
      event.defaultPrevented ||
      event.isPropagationStopped() ||
      isImeKey(event.nativeEvent) ||
      props.value === "" ||
      props.disabled ||
      props.readOnly ||
      props.pending
    )
      return;
    event.stopPropagation();
    if (!onClear) return;
    event.preventDefault();
    onClear();
    element.current?.focus();
  };
  const box = stylex.props(
    FIELD_SURFACE.face,
    FIELD_SIZE[size],
    styles.searchBox,
    SEARCH_SIZE[size],
    extra,
  );
  return (
    <div
      data-focus-proxy={ring ? "" : undefined}
      {...box}
      className={cn(box.className, className)}
    >
      <Icon
        glyph={Search}
        size={SEARCH_GLYPH[size]}
        {...stylex.props(styles.glyph)}
      />
      <TextField
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
        {...props}
        onKeyDown={(event) => {
          onKeyDown?.(event);
          clearOnEscape(event);
        }}
        ref={setElement}
        type="search"
        variant="bare"
        font={font}
        size={size}
        data-chrome-focus=""
        styles={onClear && styles.customClear}
      />
      {onClear && props.value !== "" && (
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => {
            onClear();
            element.current?.focus();
          }}
          onKeyDown={clearOnEscape}
          disabled={props.disabled || props.readOnly}
          pending={props.pending}
          aria-label={clearLabel}
          styles={styles.clear}
        >
          <Icon glyph={X} size="xs" />
        </Button>
      )}
    </div>
  );
}
