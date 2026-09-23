import { lazy, Suspense, useEffect } from "react";
import type { MarkdownMessageProps } from "./MarkdownRenderer";
import { useVisibleTextMaterial } from "../messageVisibleMaterial";

const loadRenderer = () => import("./MarkdownRenderer");

const MarkdownRenderer = lazy(() => loadRenderer().then((m) => ({ default: m.MarkdownRenderer })));

function PendingMarkdown({ text }: { text: string }) {
  useVisibleTextMaterial(false);
  return (
    <div data-surface-pending="" className="md" dir="auto">
      {text}
    </div>
  );
}

export function MarkdownMessage(props: MarkdownMessageProps) {
  return (
    <Suspense fallback={<PendingMarkdown text={props.text} />}>
      <MarkdownRenderer {...props} />
    </Suspense>
  );
}

export function useWarmMarkdownRenderer(): void {
  useEffect(() => {
    void loadRenderer();
  }, []);
}
