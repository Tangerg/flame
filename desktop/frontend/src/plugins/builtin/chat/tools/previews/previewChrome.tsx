import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { useT } from "@/lib/i18n";

export const TEXT_PREVIEW_CLASS =
  "max-h-60 overflow-y-auto px-0 pt-1 pb-0 font-mono text-ui-md leading-body text-fg-muted";

export const INLINE_PREVIEW_ROW_LIMIT = 9;

export function PreviewOverflow({ count }: { count: number }) {
  const t = useT();
  if (count <= 0) return null;
  return <div className="text-fg-faint">… {t("tools.overflow.more", { count })}</div>;
}

/** A tool whose result IS the answer: plain prose, or the placeholder when there is none
 *  yet. Two previews had this rendered identically and separately. */
export function ToolResultProse({ tool }: ToolPreviewProps) {
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {tool.result?.trim() ? (
        <p className="whitespace-pre-wrap break-words font-sans text-ui-sm leading-body text-fg-soft">
          {tool.result}
        </p>
      ) : (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.running"
          idle="tools.preview.idle.empty"
        />
      )}
    </div>
  );
}
