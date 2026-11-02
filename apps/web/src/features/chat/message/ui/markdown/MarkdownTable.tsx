import { iconGlyph } from "@/ui/icons/index";
import { useCallback, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { FileReferencePresentationContext } from "@/features/workspace/public/FileRefLink";
import { copyRichText } from "@/lib/clipboard";
import { useT } from "@/lib/i18n/index";
import { useContentVisible, useCopyFeedback } from "@flame/ui";
import { LightboxDialog } from "@flame/ui";
import { IconButton } from "@flame/ui";
import { useScrollReach } from "@/ui/atoms/use-scroll-reach";
interface Props {
  markdownSource: string;
  children?: ReactNode;
}

export function MarkdownTable({ markdownSource, children }: Props) {
  const t = useT();
  const scroll = useScrollReach();
  const reachable = scroll.tabIndex === 0;
  const [preview, setPreview] = useState<"closed" | "open" | "navigated">("closed");
  const previewOpen = preview === "open";
  const expandRef = useRef<HTMLButtonElement>(null);
  const tableRef = useRef<HTMLTableElement>(null);
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
  const { copied, copy } = useCopyFeedback(markdownSource, 1500, writeTable);

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
          onOpenChange={(open) => setPreview(open ? "open" : "closed")}
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
            value={{ origin: expandRef, onOpen: () => setPreview("navigated") }}
          >
            <div className="md md-table-preview" data-prose="">
              <table dir="auto" data-slot="prose-table">
                {children}
              </table>
            </div>
          </FileReferencePresentationContext>
        </LightboxDialog>
        <IconButton
          icon={iconGlyph(copied ? "check" : "copy")}
          size="xs"
          quiet
          onClick={() => void copy()}
          aria-label={t(copied ? "message.table.copied" : "message.table.copy")}
        />
      </div>
      <div
        {...scroll}
        className="md-table-scroller"
        role={reachable ? "region" : undefined}
        aria-label={reachable ? t("message.table.content") : undefined}
        data-focus-inset={reachable ? "" : undefined}
      >
        <table ref={tableRef} dir="auto" data-slot="prose-table">
          {children}
        </table>
      </div>
    </div>
  );
}
