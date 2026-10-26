import * as stylex from "@stylexjs/stylex";
import { Check, Copy, UnfoldHorizontal, WrapText } from "@flame/icons/react";
import {
  createContext,
  useContext,
  useSyncExternalStore,
  type ComponentPropsWithoutRef,
  type ComponentType,
  type ReactNode,
} from "react";
import { ARTIFACT_SURFACE } from "./artifact-surface";
import { useCopyFeedback } from "./clipboard";
import { IconButton } from "./icon-button";
import { selectAllInReader } from "./reader-selection";
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

export interface CodeSourceProps {
  source: string;
  language: string;
}

export type CodeSourceComponent = ComponentType<CodeSourceProps>;

function PlainCodeSource({ source }: CodeSourceProps) {
  return <code>{source}</code>;
}

const CodeSource = createContext<CodeSourceComponent>(PlainCodeSource);

export function CodeHighlightProvider({
  component,
  children,
}: {
  component: CodeSourceComponent;
  children: ReactNode;
}) {
  return <CodeSource value={component}>{children}</CodeSource>;
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
  frame: {
    display: "flex",
    flexDirection: "column",
    minWidth: 0,
    maxWidth: "100%",
    overflow: "hidden",
  },
  caption: {
    display: "flex",
    flexWrap: "wrap",
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
    minWidth: 0,
    maxWidth: "100%",
    maxHeight: "min(28rem,60vh)",
    overflow: "auto",
    overscrollBehavior: "contain auto",
    whiteSpace: "pre",
    paddingInline: space.s3,
    paddingBlockStart: space.s0_5,
    paddingBlockEnd: space.s3,
    fontFamily: "var(--font-mono)",
    lineHeight: leading.relaxed,
    userSelect: "text",
  },
  wrapped: { whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
});

export function CodeFrame({
  children,
  ...props
}: Omit<ComponentPropsWithoutRef<"div">, "className" | "style">) {
  return (
    <div
      data-slot="code-frame"
      {...props}
      {...stylex.props(ARTIFACT_SURFACE.face, styles.frame)}
    >
      {children}
    </div>
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
  const wrapped = useCodeWrap();
  const { copied, copy } = useCopyFeedback(code);
  return (
    <div
      {...props}
      data-slot="code-caption"
      {...stylex.props(styles.caption, type.uiXs)}
    >
      <span {...stylex.props(styles.language)}>{language || "text"}</span>
      <span {...stylex.props(styles.actions)}>
        {children}
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
      </span>
    </div>
  );
}

export function CodeBody({
  children,
  label,
  language = "text",
  styles: extra,
}: {
  children: string;
  label: string;
  language?: string;
  styles?: stylex.StyleXArray<stylex.StyleXStyles | null | false>;
}) {
  const wrapped = useCodeWrap();
  const Source = useContext(CodeSource);
  return (
    <pre
      dir="ltr"
      role="region"
      aria-label={label}
      tabIndex={0}
      data-slot="code-body"
      data-focus-inset=""
      data-wrap={wrapped}
      onKeyDown={selectAllInReader}
      {...stylex.props(
        styles.code,
        type.code,
        wrapped && styles.wrapped,
        extra,
      )}
    >
      <Source source={children} language={language} />
    </pre>
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
  return (
    <CodeFrame data-slot="code-block">
      <CodeCaption language={language} code={children} labels={labels} />
      <CodeBody label={label} language={language || "text"}>
        {children}
      </CodeBody>
    </CodeFrame>
  );
}
