import * as stylex from "@stylexjs/stylex";
import { clsx as cn } from "clsx";
import { Copy, UnfoldHorizontal, WrapText } from "@flame/icons/react";
import {
  useRef,
  useSyncExternalStore,
  type ComponentPropsWithRef,
  type ComponentPropsWithoutRef,
  type Ref,
} from "react";
import { ARTIFACT_SURFACE } from "./artifact-surface";
import { useCopy } from "./clipboard";
import { IconButton } from "./icon-button";
import { isImeKey } from "./keyboard";
import {
  scrollReaderWithForeignSelection,
  selectAllInReader,
} from "./reader-selection";
import { scrollWithHorizontalArrow } from "./scroll-region";
import {
  literalSourceStyles,
  useCodeSource,
  useDiffSource,
  type DiffSourceProps,
} from "./source-renderers";
import { color, leading, space, type, weight } from "./tokens.stylex";

let wrapCode = false;
const wrapListeners = new Set<() => void>();

function subscribeWrap(listener: () => void): () => void {
  wrapListeners.add(listener);
  return () => wrapListeners.delete(listener);
}

const readWrap = () => wrapCode;

function useCodeWrap(): boolean {
  return useSyncExternalStore(subscribeWrap, readWrap, readWrap);
}

function setCodeWrap(next: boolean): void {
  if (wrapCode === next) return;
  wrapCode = next;
  for (const listener of wrapListeners) listener();
}

export interface CodeActionLabels {
  copy: string;
  wrap: string;
  unwrap: string;
}

const ENGLISH_CODE_ACTIONS: CodeActionLabels = {
  copy: "Copy code",
  wrap: "Enable word wrap",
  unwrap: "Disable word wrap",
};

// The border keeps scrolling glyphs outside the inset focus perimeter.
const SOURCE_EDGE = "var(--focus-inset-edge)";

const styles = stylex.create({
  frame: {
    display: "flex",
    flexDirection: "column",
    minWidth: 0,
    maxWidth: "100%",
    overflow: "hidden",
  },
  literalFrame: {
    minHeight: 0,
    maxHeight: "min(36rem, 80dvh)",
    fontFamily: "var(--font-mono)",
    lineHeight: leading.relaxed,
  },
  caption: {
    display: "flex",
    flexWrap: "wrap",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s1,
    minHeight: "calc(var(--control-height-xs) + var(--spacing) * 2)",
    paddingBlock: space.s1,
    paddingInlineStart: space.s3,
    paddingInlineEnd: space.s1_5,
    fontFamily: "var(--font-sans)",
  },
  language: {
    flex: "1 0 auto",
    maxWidth: "100%",
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    color: color.fgMuted,
    fontWeight: weight.medium,
  },
  actions: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    justifyContent: "flex-end",
    gap: space.s1,
    minWidth: 0,
    maxWidth: "100%",
    marginInlineStart: "auto",
  },
  code: {
    margin: 0,
    flex: "1 1 auto",
    minHeight: 0,
    minWidth: 0,
    maxWidth: "100%",
    maxHeight: "min(28rem,60vh)",
    overflow: "auto",
    overscrollBehavior: "contain auto",
    borderWidth: SOURCE_EDGE,
    borderStyle: "solid",
    borderColor: "transparent",
    whiteSpace: "pre",
    paddingInline: `calc(${space.s3} - ${SOURCE_EDGE})`,
    paddingBlockStart: space.s0_5,
    paddingBlockEnd: space.s3,
    fontFamily: "var(--font-mono)",
    lineHeight: leading.relaxed,
    userSelect: "text",
  },
});

export function CodeFrame({
  children,
  bounded = false,
  className,
  styles: extra,
  ...props
}: Omit<ComponentPropsWithRef<"div">, "style"> & {
  "data-slot"?: string;
  bounded?: boolean;
  styles?: stylex.StyleXArray<stylex.StyleXStyles | null | false>;
}) {
  const styled = stylex.props(
    ARTIFACT_SURFACE.face,
    styles.frame,
    bounded && [styles.literalFrame, type.code],
    extra,
  );
  return (
    <div
      data-slot="code-frame"
      {...props}
      {...styled}
      className={cn(styled.className, className)}
    >
      {children}
    </div>
  );
}

export function CodeWrapButton({
  labels = ENGLISH_CODE_ACTIONS,
  size = "xs",
}: {
  labels?: Pick<CodeActionLabels, "wrap" | "unwrap">;
  size?: ComponentPropsWithoutRef<typeof IconButton>["size"];
}) {
  const wrapped = useCodeWrap();
  return (
    <IconButton
      icon={wrapped ? WrapText : UnfoldHorizontal}
      size={size}
      active={wrapped}
      onClick={() => setCodeWrap(!wrapped)}
      aria-label={wrapped ? labels.unwrap : labels.wrap}
    />
  );
}

export function CodeCaption({
  language,
  code,
  labels = ENGLISH_CODE_ACTIONS,
  wrappable = true,
  copyable = true,
  copyReveal,
  children,
  ...props
}: Omit<ComponentPropsWithoutRef<"div">, "className" | "style"> & {
  language?: string;
  code: string;
  labels?: CodeActionLabels;
  wrappable?: boolean;
  copyable?: boolean;
  copyReveal?: { "data-reveal": string; className: string };
}) {
  const copy = useCopy(code, labels.copy);
  return (
    <div
      {...props}
      data-slot="code-caption"
      {...stylex.props(styles.caption, type.uiXs)}
    >
      <span {...stylex.props(styles.language)}>{language || "text"}</span>
      <span {...stylex.props(styles.actions)}>
        {children}
        {wrappable && <CodeWrapButton labels={labels} />}
        {copyable && (
          <IconButton
            icon={Copy}
            size="xs"
            tone={undefined}
            onClick={() => void copy()}
            {...copyReveal}
            aria-label={labels.copy}
          />
        )}
      </span>
    </div>
  );
}

export function CodeBody({
  children,
  label,
  language = "text",
  styles: extra,
  ref,
}: {
  children: string;
  label: string;
  language?: string;
  styles?: stylex.StyleXArray<stylex.StyleXStyles | null | false>;
  ref?: Ref<HTMLDivElement>;
}) {
  const wrapped = useCodeWrap();
  const Source = useCodeSource();
  const selectionContentRef = useRef<Element | null>(null);
  return (
    <div
      ref={ref}
      dir="ltr"
      role="region"
      aria-label={label}
      tabIndex={0}
      data-slot="code-body"
      data-focus-inset=""
      data-wrap={wrapped}
      onKeyDown={(event) => {
        selectAllInReader(
          event,
          selectionContentRef.current ?? event.currentTarget,
        );
        if (event.cancelable && !isImeKey(event.nativeEvent))
          scrollWithHorizontalArrow(event);
        scrollReaderWithForeignSelection(event, "both", "nearest");
      }}
      {...stylex.props(
        styles.code,
        type.code,
        wrapped && literalSourceStyles.wrapped,
        extra,
      )}
    >
      <Source
        source={children}
        language={language}
        wrapped={wrapped}
        selectionContentRef={selectionContentRef}
      />
    </div>
  );
}

export function DiffBody(props: Omit<DiffSourceProps, "wrapped">) {
  const wrapped = useCodeWrap();
  const Source = useDiffSource();
  return <Source {...props} wrapped={wrapped} />;
}

export function CodeBlock({
  children,
  label,
  caption,
  language,
  labels,
  sourceBodyRef,
}: {
  children: string;
  label: string;
  caption?: string;
  language?: string;
  labels?: CodeActionLabels;
  sourceBodyRef?: Ref<HTMLDivElement>;
}) {
  return (
    <CodeFrame bounded data-slot="code-block" data-focus-unit="">
      <CodeCaption
        language={caption ?? language}
        code={children}
        labels={labels}
      />
      <CodeBody ref={sourceBodyRef} label={label} language={language || "text"}>
        {children}
      </CodeBody>
    </CodeFrame>
  );
}
