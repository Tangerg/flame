import * as stylex from "@stylexjs/stylex";
import { Check, Copy, UnfoldHorizontal, WrapText } from "@flame/icons/react";
import { useSyncExternalStore, type ComponentPropsWithoutRef } from "react";
import { ARTIFACT_SURFACE } from "./artifact-surface";
import { useCopyFeedback } from "./clipboard";
import { IconButton } from "./icon-button";
import { color, leading, space, type, weight } from "./tokens.stylex";

let wrapCode = false;
const wrapListeners = new Set<() => void>();

function subscribeWrap(listener: () => void): () => void {
  wrapListeners.add(listener);
  return () => wrapListeners.delete(listener);
}

const readWrap = () => wrapCode;

export function useCodeWrap(): boolean {
  return useSyncExternalStore(subscribeWrap, readWrap, readWrap);
}

export function setCodeWrap(next: boolean): void {
  if (wrapCode === next) return;
  wrapCode = next;
  for (const listener of wrapListeners) listener();
}

export interface CodeActionLabels {
  copy: string;
  copied: string;
  wrap: string;
  unwrap: string;
}

const ENGLISH_CODE_ACTIONS: CodeActionLabels = {
  copy: "Copy code",
  copied: "Copied",
  wrap: "Enable word wrap",
  unwrap: "Disable word wrap",
};

const styles = stylex.create({
  frame: { minWidth: 0, maxWidth: "100%", overflow: "hidden" },
  caption: {
    display: "flex",
    alignItems: "center",
    gap: space.s1,
    minHeight: "calc(var(--control-height-xs) + var(--spacing) * 2)",
    paddingBlock: space.s1,
    paddingInlineStart: space.s3,
    paddingInlineEnd: space.s1_5,
    fontFamily: "var(--font-sans)",
  },
  language: {
    minWidth: 0,
    flex: 1,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    color: color.fgMuted,
    fontWeight: weight.medium,
  },
  code: {
    margin: 0,
    minWidth: 0,
    maxWidth: "100%",
    maxHeight: "min(28rem,60vh)",
    overflow: "auto",
    overscrollBehavior: "contain",
    whiteSpace: "pre",
    paddingInline: space.s3,
    paddingBlockStart: space.s0_5,
    paddingBlockEnd: space.s3,
    fontFamily: "var(--font-mono)",
    lineHeight: leading.relaxed,
  },
  wrapped: { whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
});

export function CodeCaption({
  language,
  code,
  labels = ENGLISH_CODE_ACTIONS,
  wrappable = true,
  copyable = true,
  copyReveal,
  ...props
}: Omit<ComponentPropsWithoutRef<"div">, "children" | "className" | "style"> & {
  language?: string;
  code: string;
  labels?: CodeActionLabels;
  wrappable?: boolean;
  copyable?: boolean;
  copyReveal?: { "data-reveal": string; className: string };
}) {
  const wrapped = useCodeWrap();
  const { copied, copy } = useCopyFeedback(code);
  return (
    <div
      {...props}
      data-slot="code-caption"
      {...stylex.props(styles.caption, type.uiXs)}
    >
      <span {...stylex.props(styles.language)}>{language || "text"}</span>
      {wrappable && (
        <IconButton
          icon={wrapped ? WrapText : UnfoldHorizontal}
          size="xs"
          active={wrapped}
          aria-pressed={wrapped}
          onClick={() => setCodeWrap(!wrapped)}
          aria-label={wrapped ? labels.unwrap : labels.wrap}
        />
      )}
      {copyable && (
        <IconButton
          icon={copied ? Check : Copy}
          size="xs"
          tone={copied ? "success" : undefined}
          onClick={() => void copy()}
          {...copyReveal}
          aria-label={copied ? labels.copied : labels.copy}
        />
      )}
    </div>
  );
}

export function CodeBlock({
  children,
  label,
  language,
  labels,
}: {
  children: string;
  label: string;
  language?: string;
  labels?: CodeActionLabels;
}) {
  const wrapped = useCodeWrap();
  return (
    <div
      data-slot="code-block"
      {...stylex.props(ARTIFACT_SURFACE.face, styles.frame)}
    >
      <CodeCaption language={language} code={children} labels={labels} />
      <pre
        dir="ltr"
        role="region"
        aria-label={label}
        tabIndex={0}
        data-focus-inset=""
        data-wrap={wrapped}
        {...stylex.props(styles.code, type.code, wrapped && styles.wrapped)}
      >
        <code>{children}</code>
      </pre>
    </div>
  );
}
