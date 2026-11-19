import * as stylex from "@stylexjs/stylex";
import { Component, useCallback, useMemo, useRef, type RefObject } from "react";
import {
  processFile,
  type FileDiff as DiffRenderer,
  type FileDiffMetadata,
  type PostRenderPhase,
} from "@pierre/diffs";
import { FileDiff, useWorkerPool, type FileDiffOptions } from "@pierre/diffs/react";
import {
  ScrollRegion,
  scrollReaderWithLocalArrow,
  scrollWithHorizontalArrow,
  selectAllInReader,
  useContentVisible,
  type DiffSourceProps,
} from "@flame/ui";
import { useScheme } from "@/lib/appearance";
import { copyDiffSelection, selectedDiffRange } from "./diffSelection";
import { CODE_RENDERING } from "./PierreFile";

const styles = stylex.create({
  reader: {
    // Clip Pierre's gutter and change fills before they reach the inset outline.
    borderInlineWidth: "var(--focus-inset-edge)",
    borderInlineStyle: "solid",
    borderInlineColor: "transparent",
  },
});

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

interface PresentedDiff {
  node: ShadowRoot;
  file: FileDiffMetadata;
  renderer: DiffRenderer;
}

interface DiffLineAnchor {
  viewport: HTMLElement;
  index: string;
  offset: number;
}

function firstVisibleLine(
  root: ShadowRoot,
  viewport: HTMLElement,
  preferredIndex: string | undefined,
): DiffLineAnchor | null {
  const top = viewport.getBoundingClientRect().top + viewport.clientTop;
  const bottom = top + viewport.clientHeight;
  let first: DiffLineAnchor | null = null;
  for (const column of root.querySelectorAll("[data-code]")) {
    for (const line of column.querySelectorAll("[data-line][data-line-index]")) {
      const bounds = line.getBoundingClientRect();
      if (bounds.top >= bottom) break;
      if (Math.min(bounds.bottom, bottom) - Math.max(bounds.top, top) < 1) continue;

      // Pierre stamps the same unified,split index pair in both layouts. Paired
      // cells share the second index but have distinct unified indices.
      const index = line.getAttribute("data-line-index")!;
      const offset = bounds.top - top;
      const paired = first && index.split(",")[1] === first.index.split(",")[1];
      if (
        !first ||
        (paired
          ? index === preferredIndex ||
            (first.index !== preferredIndex && column.hasAttribute("data-additions"))
          : offset < first.offset)
      ) {
        first = { viewport, index, offset };
      }
      break;
    }
  }
  return first;
}

interface DiffPresentationProps {
  file: FileDiffMetadata;
  options: FileDiffOptions<undefined, undefined>;
  visible: boolean;
  presented: RefObject<PresentedDiff | null>;
}

function lineLayoutChanged(previous: DiffPresentationProps, current: DiffPresentationProps) {
  return (
    previous.options.diffStyle !== current.options.diffStyle ||
    previous.options.overflow !== current.options.overflow
  );
}

class DiffPresentation extends Component<DiffPresentationProps, unknown, DiffLineAnchor | null> {
  private preferredIndex?: string;

  // React captures before FileDiff's child layout effect reflows its lines.
  override getSnapshotBeforeUpdate(previous: DiffPresentationProps): DiffLineAnchor | null {
    const { file, presented, visible } = this.props;
    const current = presented.current;
    if (
      !visible ||
      !previous.visible ||
      file !== previous.file ||
      !lineLayoutChanged(previous, this.props) ||
      current?.file !== file
    )
      return null;
    const viewport = current.node.host.closest<HTMLElement>('[data-slot="panel-body"]');
    return viewport?.isConnected && viewport.clientHeight > 0 && viewport.scrollTop > 0
      ? firstVisibleLine(current.node, viewport, this.preferredIndex)
      : null;
  }

  override componentDidUpdate(
    previous: DiffPresentationProps,
    _state: unknown,
    anchor: DiffLineAnchor | null,
  ) {
    const { file, options, presented, visible } = this.props;
    if (!visible || file !== previous.file || lineLayoutChanged(previous, this.props))
      this.preferredIndex = undefined;
    const current = presented.current;
    if (!anchor || current?.file !== file || !anchor.viewport.contains(current.node.host)) return;

    const { viewport, index, offset } = anchor;
    const line = current.node.querySelector(
      `[data-code] [data-line][data-line-index="${CSS.escape(index)}"]`,
    );
    const bounds = line?.getBoundingClientRect();
    if (!bounds?.height) return;
    const top = viewport.getBoundingClientRect().top + viewport.clientTop;
    const unwrapping = previous.options.overflow === "wrap" && options.overflow === "scroll";
    const visibleOffset =
      unwrapping && offset + bounds.height <= 0
        ? 0
        : Math.max(1 - bounds.height, Math.min(offset, viewport.clientHeight - 1));
    const lineTop = viewport.scrollTop + bounds.top - top;
    // A fractional tail of the preceding row would take over the next capture.
    viewport.scrollTo({
      top: Math.abs(visibleOffset) < 1 ? Math.ceil(lineTop) : Math.round(lineTop - visibleOffset),
      behavior: "instant",
    });
    if (options.diffStyle === "split") this.preferredIndex = index;
  }

  override render() {
    return <FileDiff fileDiff={this.props.file} options={this.props.options} />;
  }
}

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
  const root = useRef<PresentedDiff | null>(null);
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
      data-focus-inset=""
      styles={styles.reader}
      onKeyDown={(event) => {
        if (!visible) return;
        const presented = root.current;
        selectAllInReader(event, presented?.node ?? event.currentTarget);
        const renderer = presented?.renderer;
        if (!wrapped && renderer)
          scrollWithHorizontalArrow(event, (delta) => {
            renderer.setCodeScrollLeft(renderer.getCodeScrollLeft() + delta);
          });
        if (
          (event.key === "ArrowUp" || event.key === "ArrowDown") &&
          (!presented || !selectedDiffRange(presented.node))
        )
          scrollReaderWithLocalArrow(event, "nearest");
      }}
      onCopy={(event) => {
        if (visible && root.current) copyDiffSelection(event, root.current.node, root.current.file);
      }}
    >
      <DiffPresentation file={file} options={options} visible={visible} presented={root} />
    </ScrollRegion>
  );
}
