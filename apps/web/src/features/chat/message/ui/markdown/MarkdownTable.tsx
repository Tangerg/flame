import { iconGlyph } from "@/ui/icons/index";
import { useCallback, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { FileReferencePresentationContext } from "@/features/workspace/public/FileRefLink";
import { copyRichText } from "@/lib/clipboard";
import { useT } from "@/lib/i18n/index";
import {
  IconButton,
  LightboxDialog,
  scrollReaderWithForeignSelection,
  scrollReaderWithLocalArrow,
  useContentVisible,
  useCopy,
} from "@flame/ui";
import { useScrollReach } from "@/ui/atoms/use-scroll-reach";
interface Props {
  markdownSource: string;
  children?: ReactNode;
}

const SCROLL_EDGE_TOLERANCE = 0.5;

function firstVisibleBodyRow(table: HTMLTableElement | null): number | null {
  const viewport = table?.parentElement;
  if (!table || !viewport || viewport.scrollTop <= 0) return null;

  const top = viewport.getBoundingClientRect().top + viewport.clientTop;
  const bottom = top + viewport.clientHeight;
  const belowHeader = Math.max(top, table.tHead?.getBoundingClientRect().bottom ?? top);
  if (belowHeader >= bottom) return null;

  for (const body of table.tBodies) {
    for (const row of body.rows) {
      const bounds = row.getBoundingClientRect();
      if (bounds.height > 0 && bounds.bottom > belowHeader && bounds.top < bottom) {
        return row.rowIndex;
      }
    }
  }
  return null;
}

function trailingVisibleHeaderIndex(table: HTMLTableElement | null): number | null {
  const viewport = table?.parentElement;
  const header = table?.tHead?.rows.item(0);
  if (!viewport || !header || viewport.clientWidth <= 0) return null;

  const rtl = getComputedStyle(viewport).direction === "rtl";
  if ((rtl ? -viewport.scrollLeft : viewport.scrollLeft) <= SCROLL_EDGE_TOLERANCE) {
    return null;
  }

  const left = viewport.getBoundingClientRect().left + viewport.clientLeft;
  const right = left + viewport.clientWidth;
  let trailingEdge = -Infinity;
  let index: number | null = null;
  for (const cell of header.cells) {
    const bounds = cell.getBoundingClientRect();
    const visibleWidth = Math.min(right, bounds.right) - Math.max(left, bounds.left);
    const edge = rtl ? -bounds.left : bounds.right;
    if (visibleWidth > SCROLL_EDGE_TOLERANCE && edge > trailingEdge) {
      trailingEdge = edge;
      index = cell.cellIndex;
    }
  }
  return index;
}

function previewColumnOffset(
  viewport: HTMLDivElement,
  table: HTMLTableElement | null,
  index: number | null,
): number {
  if (!table || index === null) return 0;
  const cell = table.tHead?.rows.item(0)?.cells.item(index);
  const viewportWidth = viewport.clientWidth;
  if (!cell || viewportWidth <= 0) return 0;

  const renderedWidth = table.getBoundingClientRect().width;
  const layoutWidth = table.offsetWidth;
  if (renderedWidth <= 0 || layoutWidth <= 0) return 0;
  // Use the table to remove the dialog's entry scale without relying on offset parents.
  const layoutPixels = layoutWidth / renderedWidth;
  const bounds = cell.getBoundingClientRect();
  const left =
    (bounds.left - viewport.getBoundingClientRect().left) * layoutPixels +
    viewport.scrollLeft -
    viewport.clientLeft;
  const width = bounds.width * layoutPixels;
  const rtl = getComputedStyle(viewport).direction === "rtl";
  const start = rtl ? viewportWidth - (left + width) : left;
  // An oversized column can only align its leading edge.
  const distance = Math.min(start, start + width - viewportWidth);
  if (distance <= SCROLL_EDGE_TOLERANCE) return 0;
  return (rtl ? -1 : 1) * Math.ceil(distance);
}

export function MarkdownTable({ markdownSource, children }: Props) {
  const t = useT();
  const scroll = useScrollReach();
  const reachable = scroll.tabIndex === 0;
  const [preview, setPreview] = useState<"closed" | "open" | "navigated">("closed");
  const previewOpen = preview === "open";
  const expandRef = useRef<HTMLButtonElement>(null);
  const tableRef = useRef<HTMLTableElement>(null);
  const previewTableRef = useRef<HTMLTableElement>(null);
  const pendingPreview = useRef<{
    rowIndex: number | null;
    headerIndex: number | null;
  } | null>(null);
  const frameRef = useRef<HTMLDivElement>(null);
  const actionsRef = useRef<HTMLDivElement>(null);
  const visible = useContentVisible();
  useLayoutEffect(() => {
    const root = frameRef.current;
    const actions = actionsRef.current;
    const table = tableRef.current;
    if (!visible || !root || !actions || !table) return;
    const head = table.querySelector(":scope > thead");
    const cell = table.querySelector(":scope > tbody > tr > :is(td, th)");
    // A small reading budget must not let the sticky header cover every body line.
    const measure = () => {
      let height =
        actions.getBoundingClientRect().height +
        parseFloat(getComputedStyle(root).rowGap) +
        (head?.getBoundingClientRect().height ?? 0);
      if (cell) {
        const line = getComputedStyle(cell);
        height +=
          parseFloat(line.lineHeight) +
          parseFloat(line.paddingBlockStart) +
          parseFloat(line.paddingBlockEnd);
      }
      root.style.setProperty("--table-min-height", `${height}px`);
    };
    measure();
    const resize = new ResizeObserver(measure);
    resize.observe(actions);
    if (head) resize.observe(head);
    if (cell) resize.observe(cell);
    return () => {
      resize.disconnect();
      root.style.removeProperty("--table-min-height");
    };
  }, [children, visible]);
  const writeTable = useCallback(
    (plainText: string) =>
      copyRichText({
        plainText,
        htmlText: tableRef.current?.outerHTML,
      }),
    [],
  );
  const copy = useCopy(markdownSource, t("message.code.copy"), writeTable);

  function handlePreviewOpenChange(open: boolean) {
    const table = tableRef.current;
    pendingPreview.current = open
      ? {
          rowIndex: firstVisibleBodyRow(table),
          headerIndex: trailingVisibleHeaderIndex(table),
        }
      : null;
    setPreview(open ? "open" : "closed");
  }

  function preparePreview(viewport: HTMLDivElement | null) {
    const opening = pendingPreview.current;
    pendingPreview.current = null;
    if (!opening || !viewport) return;

    const table = previewTableRef.current;
    const firstRow = table?.tBodies.item(0)?.rows.item(0);
    const row = opening.rowIndex === null ? null : table?.rows.item(opening.rowIndex);
    // Row layout offsets are unaffected by the dialog's opening scale.
    const top = row && firstRow ? row.offsetTop - firstRow.offsetTop : 0;
    const left = previewColumnOffset(viewport, table, opening.headerIndex);
    viewport.scrollTo({ top, left, behavior: "instant" });
  }

  return (
    <div
      ref={frameRef}
      className="md-table-container"
      data-markdown-table
      data-focus-unit=""
      tabIndex={-1}
    >
      <div
        ref={actionsRef}
        className="md-table-actions"
        data-reveal="hover"
        data-markdown-copy="exclude"
      >
        <LightboxDialog
          open={previewOpen}
          onOpenChange={handlePreviewOpenChange}
          prepareDocument={preparePreview}
          finalFocus={preview !== "navigated"}
          title={t("message.table.preview")}
          kind="document"
          closeLabel={t("message.table.closePreview")}
          contentLabel={t("message.table.content")}
          trigger={
            <IconButton
              ref={expandRef}
              icon={iconGlyph("maximize")}
              size="xs"
              quiet
              aria-expanded={previewOpen}
              aria-haspopup="dialog"
              aria-label={t("message.table.expand")}
            />
          }
        >
          <FileReferencePresentationContext
            value={{
              origin: expandRef,
              onOpen: () => {
                pendingPreview.current = null;
                setPreview("navigated");
              },
            }}
          >
            <div className="md md-table-preview" data-prose="">
              <table ref={previewTableRef} dir="auto" data-slot="prose-table">
                {children}
              </table>
            </div>
          </FileReferencePresentationContext>
        </LightboxDialog>
        <IconButton
          icon={iconGlyph("copy")}
          size="xs"
          quiet
          onClick={() => void copy()}
          aria-label={t("message.table.copy")}
        />
      </div>
      <div
        {...scroll}
        className="md-table-scroller"
        role={reachable ? "region" : undefined}
        aria-label={reachable ? t("message.table.content") : undefined}
        data-focus-inset={reachable ? "" : undefined}
        onKeyDown={(event) => {
          scrollReaderWithForeignSelection(event, "both", "nearest");
          // WebKit can move the document caret and jump to a different table row.
          scrollReaderWithLocalArrow(event);
        }}
      >
        <table ref={tableRef} dir="auto" data-slot="prose-table">
          {children}
        </table>
      </div>
    </div>
  );
}
