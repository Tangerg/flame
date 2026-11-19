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

export function MarkdownTable({ markdownSource, children }: Props) {
  const t = useT();
  const scroll = useScrollReach();
  const reachable = scroll.tabIndex === 0;
  const [preview, setPreview] = useState<"closed" | "open" | "navigated">("closed");
  const previewOpen = preview === "open";
  const expandRef = useRef<HTMLButtonElement>(null);
  const tableRef = useRef<HTMLTableElement>(null);
  const previewTableRef = useRef<HTMLTableElement>(null);
  const pendingPreview = useRef<{ rowIndex: number | null } | null>(null);
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
    pendingPreview.current = open ? { rowIndex: firstVisibleBodyRow(tableRef.current) } : null;
    setPreview(open ? "open" : "closed");
  }

  function preparePreview(viewport: HTMLDivElement | null) {
    const opening = pendingPreview.current;
    pendingPreview.current = null;
    if (!opening || !viewport) return;

    const table = previewTableRef.current;
    const firstRow = table?.tBodies.item(0)?.rows.item(0);
    const row = opening.rowIndex === null ? null : table?.rows.item(opening.rowIndex);
    // Table layout offsets are unaffected by the dialog's opening scale.
    viewport.scrollTo({
      top: row && firstRow ? row.offsetTop - firstRow.offsetTop : 0,
      behavior: "instant",
    });
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
