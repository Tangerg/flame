import { useEffect, useState } from "react";
import type { FileDiffMetadata, FileDiffOptions } from "@pierre/diffs";
import { FileDiff, WorkerPoolContext } from "@pierre/diffs/react";
import { WorkerPoolManager } from "@pierre/diffs/worker";
import workerSource from "@pierre/diffs/worker/worker-portable.js?raw";

export const DIFF_RENDERING_OPTIONS = {
  theme: "flame-workspace-diff",
  preferredHighlighter: "shiki-js" as const,
};

const poolOptions = {
  poolSize: 1,
  totalASTLRUCacheSize: 1,
  workerFactory() {
    const url = URL.createObjectURL(
      new Blob([workerSource], { type: "text/javascript" }),
    );
    try {
      return new Worker(url, { type: "module" });
    } finally {
      URL.revokeObjectURL(url);
    }
  },
};

type Preparation = { file: FileDiffMetadata } & (
  | { status: "ready"; pool: WorkerPoolManager }
  | { status: "failed"; error: unknown }
);

export function PreparedDiff({
  file,
  options,
  loadingLabel,
}: {
  file: FileDiffMetadata;
  options: Omit<
    FileDiffOptions<undefined, undefined>,
    "theme" | "preferredHighlighter"
  >;
  loadingLabel: string;
}) {
  const [prepared, setPrepared] = useState<Preparation>();
  useEffect(() => {
    const owned = new WorkerPoolManager(poolOptions, DIFF_RENDERING_OPTIONS);
    let active = true;
    void owned.primeDiffHighlightCache(file).then(
      () => {
        if (active) setPrepared({ file, status: "ready", pool: owned });
      },
      (error: unknown) => {
        if (active) setPrepared({ file, status: "failed", error });
      },
    );
    return () => {
      active = false;
      owned.terminate();
    };
  }, [file]);
  // Publish stable token nodes; later replacement would invalidate native text ranges.
  if (prepared?.file !== file) return <div role="status">{loadingLabel}</div>;
  if (prepared.status === "failed") throw prepared.error;
  return (
    <WorkerPoolContext.Provider value={prepared.pool}>
      <FileDiff
        fileDiff={file}
        options={{ ...options, ...DIFF_RENDERING_OPTIONS }}
      />
    </WorkerPoolContext.Provider>
  );
}
