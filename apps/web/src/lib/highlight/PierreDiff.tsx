import { useCallback, useMemo, useRef } from "react";
import {
  processFile,
  type FileDiff as DiffRenderer,
  type FileDiffMetadata,
  type PostRenderPhase,
} from "@pierre/diffs";
import { FileDiff, useWorkerPool, type FileDiffOptions } from "@pierre/diffs/react";
import {
  ScrollRegion,
  scrollWithHorizontalArrow,
  selectAllInReader,
  useContentVisible,
  type DiffSourceProps,
} from "@flame/ui";
import { useScheme } from "@/lib/appearance";
import { copyDiffSelection } from "./diffSelection";
import { CODE_RENDERING } from "./PierreFile";

const appearance = `:host {
  color-scheme: inherit;
  --diffs-font-family: var(--font-mono);
  --diffs-header-font-family: var(--font-sans);
  --diffs-font-size: var(--fs-code);
  --diffs-line-height: var(--leading-relaxed);
  --diffs-addition-base: var(--color-success);
  --diffs-deletion-base: var(--color-negative);
  --diffs-modified-base: var(--color-info);
}`;

export function PierreDiff({ patch, layout, label, wrapped = false }: DiffSourceProps) {
  const pool = useWorkerPool();
  if (!pool) throw new Error("diff rendering requires its shared worker pool");
  const scheme = useScheme();
  const visible = useContentVisible();
  const file = useMemo(() => {
    const parsed = processFile(patch, {
      cacheKey: JSON.stringify(["diff", patch]),
      isGitDiff: true,
      throwOnError: true,
      disableLineMatching: true,
    });
    if (!parsed) throw new Error("diff patch produced no file");
    return parsed;
  }, [patch]);
  // The public post-render handle binds selection to this renderer's root and retires with it.
  const root = useRef<{
    node: ShadowRoot;
    file: FileDiffMetadata;
    renderer: DiffRenderer;
  } | null>(null);
  const rendered = useCallback(
    (node: HTMLElement, renderer: DiffRenderer, phase: PostRenderPhase) => {
      if (phase !== "unmount" && node.shadowRoot)
        root.current = { node: node.shadowRoot, file, renderer };
      else if (root.current?.node === node.shadowRoot) root.current = null;
    },
    [file],
  );
  const options = useMemo<FileDiffOptions<undefined, undefined>>(
    () => ({
      ...CODE_RENDERING,
      themeType: scheme,
      diffStyle: layout,
      overflow: wrapped ? "wrap" : "scroll",
      disableFileHeader: true,
      hunkSeparators: "metadata",
      parseDiffOptions: { disableLineMatching: true },
      unsafeCSS: appearance,
      onPostRender: rendered,
    }),
    [scheme, layout, wrapped, rendered],
  );
  return (
    <ScrollRegion
      label={label}
      onKeyDown={(event) => {
        if (!visible) return;
        selectAllInReader(event, root.current?.node ?? event.currentTarget);
        const renderer = root.current?.renderer;
        if (!wrapped && renderer)
          scrollWithHorizontalArrow(event, (delta) => {
            renderer.setCodeScrollLeft(renderer.getCodeScrollLeft() + delta);
          });
      }}
      onCopy={(event) => {
        if (visible && root.current) copyDiffSelection(event, root.current.node, root.current.file);
      }}
    >
      <FileDiff fileDiff={file} options={options} />
    </ScrollRegion>
  );
}
