import { type ReactNode } from "react";
import { WorkerPoolContextProvider } from "@pierre/diffs/react";
import workerSource from "@pierre/diffs/worker/worker-portable.js?raw";
import { CodeHighlightProvider, type CodeSourceProps } from "@flame/ui";
import { CODE_RENDERING, PierreFile } from "./PierreFile";

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
  return (
    <WorkerPoolContextProvider poolOptions={poolOptions} highlighterOptions={CODE_RENDERING}>
      <CodeHighlightProvider component={HighlightedCodeSource}>{children}</CodeHighlightProvider>
    </WorkerPoolContextProvider>
  );
}
