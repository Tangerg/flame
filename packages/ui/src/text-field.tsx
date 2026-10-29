import * as stylex from "@stylexjs/stylex";
import type { StyleXArray, StyleXStyles } from "@stylexjs/stylex";
import {
  useCallback,
  useLayoutEffect,
  useRef,
  type KeyboardEvent,
} from "react";
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
import { Icon, useFontSize } from "./font-scale";
import { Search, X } from "@flame/icons/react";
import { Button } from "./button";
import { isImeKey } from "./keyboard";
import { FIELD_SIZE, FIELD_SURFACE } from "./control-surface";
import { WELL_SURFACE } from "./well-surface";
import { Input } from "@base-ui/react/input";
import type { ComponentProps, ComponentPropsWithRef } from "react";

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
  autosize: { resize: "none" },
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
    variant === "boxed" ? FIELD_SURFACE.face : styles[variant],
    invalid && variant !== "boxed" && styles.invalidBare,
  ];
}

export type TextFieldProps = Omit<
  ComponentProps<typeof Input>,
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
    <Input
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
  const element = useRef<HTMLTextAreaElement>(null);
  const resizeRef = useRef<() => void>(() => {});
  const fontSize = useFontSize();
  // Safari 17.4 shares this sizing owner with hosts that support field-sizing.
  useLayoutEffect(() => {
    const area = element.current;
    if (!autosize || !area) return;
    const originalHeight = area.style.height;
    let active = true;
    let frame = 0;
    const resize = () => {
      if (!active || area.clientWidth === 0) return;
      const style = getComputedStyle(area);
      const padding =
        parseFloat(style.paddingTop) + parseFloat(style.paddingBottom);
      const border =
        parseFloat(style.borderTopWidth) + parseFloat(style.borderBottomWidth);
      const scrollTop = area.scrollTop;
      area.style.height = "auto";
      const height =
        area.scrollHeight +
        (style.boxSizing === "border-box" ? border : -padding);
      area.style.height = `${height}px`;
      area.scrollTop = scrollTop;
    };
    const schedule = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        frame = 0;
        resize();
      });
    };
    let width = area.getBoundingClientRect().width;
    const observer = new ResizeObserver(() => {
      const next = area.getBoundingClientRect().width;
      if (next === width) return;
      width = next;
      schedule();
    });
    observer.observe(area);
    const reset = () => schedule();
    area.addEventListener("input", resize);
    area.form?.addEventListener("reset", reset);
    document.fonts?.addEventListener("loadingdone", resize);
    void document.fonts?.ready.then(resize);
    resizeRef.current = resize;
    return () => {
      active = false;
      cancelAnimationFrame(frame);
      resizeRef.current = () => {};
      observer.disconnect();
      area.removeEventListener("input", resize);
      area.form?.removeEventListener("reset", reset);
      document.fonts?.removeEventListener("loadingdone", resize);
      area.style.height = originalHeight;
    };
  }, [autosize]);
  useLayoutEffect(
    () => resizeRef.current(),
    [
      autosize,
      variant,
      size,
      font,
      props.value,
      props.defaultValue,
      props.rows,
      props.style,
      className,
      extra,
      fontSize,
    ],
  );
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
      ref={(node) => {
        element.current = node;
        if (typeof ref === "function") return ref(node);
        if (ref) ref.current = node;
      }}
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
  ...props
}: SearchFieldProps) {
  const element = useRef<HTMLElement>(null);
  const setElement = useCallback(
    (node: HTMLElement | null) => {
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
    <div data-focus-proxy="" {...box} className={cn(box.className, className)}>
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
