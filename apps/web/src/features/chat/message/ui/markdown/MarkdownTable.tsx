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

interface BodyRowAnchor {
  row: HTMLTableRowElement;
  inset: number;
}

function bodyViewport(table: HTMLTableElement, viewport: HTMLElement) {
  const frame = viewport.getBoundingClientRect();
  const top = frame.top + viewport.clientTop;
  const left = frame.left + viewport.clientLeft;
  return {
    top: Math.max(top, table.tHead?.getBoundingClientRect().bottom ?? top),
    bottom: top + viewport.clientHeight,
    left,
    right: left + viewport.clientWidth,
  };
}

function firstVisibleBodyRow(table: HTMLTableElement | null): BodyRowAnchor | null {
  const viewport = table?.parentElement;
  if (!table || !viewport || viewport.scrollTop <= 0) return null;

  const { top, bottom } = bodyViewport(table, viewport);
  if (top >= bottom) return null;

  for (const body of table.tBodies) {
    for (const row of body.rows) {
      const bounds = row.getBoundingClientRect();
      if (bounds.height > 0 && bounds.bottom > top && bounds.top < bottom) {
        return { row, inset: Math.max(0, top - bounds.top) };
      }
    }
  }
  return null;
}

function anchoredScrollTop(
  viewport: HTMLElement,
  table: HTMLTableElement,
  anchor: BodyRowAnchor,
): number | undefined {
  const { top, bottom } = bodyViewport(table, viewport);
  const bounds = anchor.row.getBoundingClientRect();
  if (top >= bottom || bounds.height <= 0) return;
  const inset = Math.min(anchor.inset, Math.max(0, bounds.height - 1));
  const delta = bounds.top + inset - top;
  return viewport.scrollTop + delta;
}

function inlineTable(viewport: HTMLElement): HTMLTableElement | null {
  const table = viewport.firstElementChild;
  return table instanceof HTMLTableElement ? table : null;
}

function visibleCellFocus(viewport: HTMLElement, table: HTMLTableElement | null): Element | null {
  const focused = viewport.ownerDocument.activeElement;
  if (!table || !focused || !table.contains(focused) || table.tHead?.contains(focused)) {
    return null;
  }
  const area = bodyViewport(table, viewport);
  const bounds = focused.getBoundingClientRect();
  return bounds.width > 0 &&
    bounds.height > 0 &&
    bounds.top >= area.top - SCROLL_EDGE_TOLERANCE &&
    bounds.bottom <= area.bottom + SCROLL_EDGE_TOLERANCE &&
    bounds.left >= area.left - SCROLL_EDGE_TOLERANCE &&
    bounds.right <= area.right + SCROLL_EDGE_TOLERANCE
    ? focused
    : null;
}

function revealedFocusTop(
  viewport: HTMLElement,
  table: HTMLTableElement,
  focused: Element,
  plannedTop: number,
): number {
  const { top, bottom } = bodyViewport(table, viewport);
  const bounds = focused.getBoundingClientRect();
  if (bounds.height <= 0) return plannedTop;

  // Constrain the row target just enough to reveal the control under the sticky header.
  const min = Math.ceil(viewport.scrollTop + bounds.bottom - bottom);
  const max = Math.floor(viewport.scrollTop + bounds.top - top);
  return min <= max ? Math.max(min, Math.min(max, plannedTop)) : plannedTop;
}

function trackTableScroll(viewport: HTMLElement) {
  const direction = getComputedStyle(viewport).direction === "rtl" ? -1 : 1;
  let clientWidth = viewport.clientWidth;
  let scrollWidth = viewport.scrollWidth;
  let clientHeight = viewport.clientHeight;
  let scrollHeight = viewport.scrollHeight;
  let observedLeft = viewport.scrollLeft;
  let observedTop = viewport.scrollTop;
  let atEnd = false;
  let anchor = firstVisibleBodyRow(inlineTable(viewport));
  let focused = visibleCellFocus(viewport, inlineTable(viewport));
  const currentWidth = () =>
    viewport.clientWidth === clientWidth && viewport.scrollWidth === scrollWidth;
  const currentHeight = () =>
    viewport.clientHeight === clientHeight && viewport.scrollHeight === scrollHeight;

  const onScroll = () => {
    // Native clamping can dispatch scroll before the new layout is observed.
    if (clientWidth <= 0 || !currentWidth()) return;
    if (scrollWidth > clientWidth + 1) {
      atEnd = direction * viewport.scrollLeft >= scrollWidth - clientWidth - 1;
    }
    if (!currentHeight()) return;
    const left = viewport.scrollLeft;
    const top = viewport.scrollTop;
    if (left === observedLeft && top === observedTop) return;

    const table = inlineTable(viewport);
    if (top !== observedTop) anchor = firstVisibleBodyRow(table);
    focused = visibleCellFocus(viewport, table);
    observedLeft = left;
    observedTop = top;
  };
  const onFocusIn = () => {
    if (currentWidth() && currentHeight()) {
      focused = visibleCellFocus(viewport, inlineTable(viewport));
    }
  };
  const onFocusOut = (event: FocusEvent) => {
    if (event.target === focused && focused !== viewport.ownerDocument.activeElement) {
      focused = null;
    }
  };
  onScroll();
  viewport.addEventListener("scroll", onScroll, { passive: true });
  viewport.addEventListener("focusin", onFocusIn);
  viewport.addEventListener("focusout", onFocusOut);

  return {
    measure() {
      const nextClientWidth = viewport.clientWidth;
      const nextScrollWidth = viewport.scrollWidth;
      const nextClientHeight = viewport.clientHeight;
      const nextScrollHeight = viewport.scrollHeight;
      const widthChanged = nextClientWidth !== clientWidth || nextScrollWidth !== scrollWidth;
      const heightChanged = nextClientHeight !== clientHeight || nextScrollHeight !== scrollHeight;
      if (nextClientWidth <= 0 || nextClientHeight <= 0 || (!widthChanged && !heightChanged)) {
        return;
      }
      clientWidth = nextClientWidth;
      scrollWidth = nextScrollWidth;
      clientHeight = nextClientHeight;
      scrollHeight = nextScrollHeight;

      const edge = direction * Math.max(0, scrollWidth - clientWidth);
      const left = widthChanged && atEnd && viewport.scrollLeft !== edge ? edge : undefined;
      const table = inlineTable(viewport);
      const currentTop = viewport.scrollTop;
      let plannedTop = currentTop;
      if (widthChanged) {
        if (table && anchor && table.contains(anchor.row)) {
          plannedTop = anchoredScrollTop(viewport, table, anchor) ?? currentTop;
        } else {
          anchor = firstVisibleBodyRow(table);
        }
      }
      if (focused) {
        if (table && table.contains(focused) && focused === viewport.ownerDocument.activeElement) {
          plannedTop = revealedFocusTop(viewport, table, focused, plannedTop);
        } else {
          focused = null;
        }
      }
      const top =
        Math.abs(plannedTop - currentTop) > SCROLL_EDGE_TOLERANCE ? plannedTop : undefined;
      if (left !== undefined || top !== undefined) {
        viewport.scrollTo({ left, top, behavior: "instant" });
      }
      // Keep the row and focused control through our own scroll and native clamp.
      observedLeft = viewport.scrollLeft;
      observedTop = viewport.scrollTop;
    },
    disconnect() {
      viewport.removeEventListener("scroll", onScroll);
      viewport.removeEventListener("focusin", onFocusIn);
      viewport.removeEventListener("focusout", onFocusOut);
    },
  };
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
  const visible = useContentVisible();
  const scroll = useScrollReach(visible ? trackTableScroll : undefined);
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
          rowIndex: firstVisibleBodyRow(table)?.row.rowIndex ?? null,
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
