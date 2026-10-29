import { iconGlyph } from "@/ui/icons/index";
import { useCallback, useRef, useState, type ReactNode } from "react";
import { FileReferencePresentationContext } from "@/features/workspace/public/FileRefLink";
import { copyRichText } from "@/lib/clipboard";
import { useT } from "@/lib/i18n/index";
import { useCopyFeedback } from "@flame/ui";
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
    <div className="md-table-container" data-markdown-table tabIndex={-1}>
      <div className="md-table-actions" data-reveal="hover" data-markdown-copy="exclude">
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
