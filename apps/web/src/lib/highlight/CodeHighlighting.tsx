import { useLayoutEffect, type ReactNode } from "react";
import { i18n } from "@pierre/diffs";
import { WorkerPoolContextProvider } from "@pierre/diffs/react";
import workerSource from "@pierre/diffs/worker/worker-portable.js?raw";
import { CodeHighlightProvider, type CodeSourceProps } from "@flame/ui";
import { localeToBcp47, useLocale, useT } from "@/lib/i18n";
import { CODE_RENDERING, PierreFile } from "./PierreFile";
import { PierreDiff } from "./PierreDiff";

const poolOptions = {
  // Blob workers inherit the page CSP, which grants worker-src blob: but no Wasm.
  workerFactory() {
    const url = URL.createObjectURL(new Blob([workerSource], { type: "text/javascript" }));
    try {
      return new Worker(url, { type: "module" });
    } finally {
      URL.revokeObjectURL(url);
    }
  },
};

function HighlightedCodeSource({ source, language, wrapped }: CodeSourceProps) {
  return <PierreFile source={source} language={language} wrapped={wrapped} />;
}

export function CodeHighlighting({ children }: { children: ReactNode }) {
  const locale = useLocale();
  const t = useT();
  // Pierre keeps one module-wide catalog for every reader it renders.
  useLayoutEffect(() => {
    i18n.set({
      locale: localeToBcp47(locale),
      messages: {
        unmodifiedLines: t("diff.unmodifiedLines"),
        unknownUnmodifiedLines: t("diff.unknownUnmodifiedLines"),
        expandAll: t("diff.expandAll"),
        noNewlineAtEndOfFile: t("diff.noNewline"),
      },
    });
  }, [locale, t]);
  return (
    <WorkerPoolContextProvider poolOptions={poolOptions} highlighterOptions={CODE_RENDERING}>
      <CodeHighlightProvider code={HighlightedCodeSource} diff={PierreDiff}>
        {children}
      </CodeHighlightProvider>
    </WorkerPoolContextProvider>
  );
}
