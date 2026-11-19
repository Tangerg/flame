import * as stylex from "@stylexjs/stylex";
import {
  createContext,
  use,
  useMemo,
  type ComponentType,
  type ReactNode,
  type RefObject,
} from "react";
import { ScrollRegion } from "./scroll-region";

export interface CodeSourceProps {
  source: string;
  language: string;
  wrapped?: boolean;
  // The renderer publishes its live selection boundary, including inside a shadow root.
  selectionContentRef?: RefObject<Element | null>;
}

export interface DiffSourceProps {
  // One complete Git file patch, presented as one scrollable reader.
  patch: string;
  layout: "unified" | "split";
  label: string;
  wrapped?: boolean;
}

export type CodeSourceComponent = ComponentType<CodeSourceProps>;
export type DiffSourceComponent = ComponentType<DiffSourceProps>;

export const literalSourceStyles = stylex.create({
  wrapped: { whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
});

function PlainCodeSource({ source }: CodeSourceProps) {
  return (
    <pre>
      <code>{source}</code>
    </pre>
  );
}

function PlainDiffSource({ patch, label, wrapped }: DiffSourceProps) {
  return (
    <ScrollRegion label={label}>
      <pre {...stylex.props(wrapped && literalSourceStyles.wrapped)}>
        <code>{patch}</code>
      </pre>
    </ScrollRegion>
  );
}

// The host owns syntax highlighting, its themes and its worker pool. Packages
// present source through these slots instead of bundling a second highlighter.
const SourceRenderers = createContext<{
  code: CodeSourceComponent;
  diff: DiffSourceComponent;
}>({ code: PlainCodeSource, diff: PlainDiffSource });

export function CodeHighlightProvider({
  code,
  diff,
  children,
}: {
  code: CodeSourceComponent;
  diff: DiffSourceComponent;
  children: ReactNode;
}) {
  const renderers = useMemo(() => ({ code, diff }), [code, diff]);
  return <SourceRenderers value={renderers}>{children}</SourceRenderers>;
}

export function useCodeSource(): CodeSourceComponent {
  return use(SourceRenderers).code;
}

export function useDiffSource(): DiffSourceComponent {
  return use(SourceRenderers).diff;
}
