import {
  createContext,
  use,
  useMemo,
  type ComponentType,
  type ReactNode,
} from "react";
import { ScrollRegion } from "./scroll-region";

export interface CodeSourceProps {
  source: string;
  language: string;
  wrapped?: boolean;
}

export interface DiffSourceProps {
  // One complete Git file patch, presented as one scrollable reader.
  patch: string;
  layout: "unified" | "split";
  label: string;
}

export type CodeSourceComponent = ComponentType<CodeSourceProps>;
export type DiffSourceComponent = ComponentType<DiffSourceProps>;

function PlainCodeSource({ source }: CodeSourceProps) {
  return (
    <pre>
      <code>{source}</code>
    </pre>
  );
}

function PlainDiffSource({ patch, label }: DiffSourceProps) {
  return (
    <ScrollRegion label={label}>
      <pre>
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

export function DiffBody(props: DiffSourceProps) {
  const Diff = use(SourceRenderers).diff;
  return <Diff {...props} />;
}
