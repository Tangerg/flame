import * as stylex from "@stylexjs/stylex";
import { useCallback, useRef, useState, type ReactNode } from "react";
import { copyRichText } from "@/lib/clipboard";
import { useT } from "@/lib/i18n";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { IconButton, LightboxDialog } from "@/ui";
import { space } from "@/styles/tokens.stylex";

const mt = stylex.create({
  close: { position: "absolute", top: space.s2, right: space.s2 },
});

interface Props {
  markdownSource: string;
  children?: ReactNode;
}

export function MarkdownTable({ markdownSource, children }: Props) {
  const t = useT();
  const [previewOpen, setPreviewOpen] = useState(false);
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
      <div className="md-table-scroller">
        <div className="md-table-wrap">
          <table ref={tableRef} dir="auto">
            {children}
          </table>
        </div>
      </div>
      <div className="md-table-actions" data-reveal="hover" data-markdown-copy="exclude">
        <LightboxDialog
          open={previewOpen}
          onOpenChange={setPreviewOpen}
          title={t("message.table.preview")}
          kind="document"
          trigger={
            <IconButton
              icon="maximize"
              size="xs"
              quiet
              aria-expanded={previewOpen}
              aria-haspopup="dialog"
              title={t("message.table.expand")}
            />
          }
        >
          <IconButton
            icon="x"
            size="sm"
            quiet
            onClick={() => setPreviewOpen(false)}
            title={t("message.table.closePreview")}
            className={stylex.props(mt.close).className}
          />
          <div className="md md-table-preview">
            <table dir="auto">{children}</table>
          </div>
        </LightboxDialog>
        <IconButton
          icon={copied ? "check" : "copy"}
          size="xs"
          quiet
          onClick={() => void copy()}
          title={t(copied ? "message.table.copied" : "message.table.copy")}
        />
      </div>
    </div>
  );
}
