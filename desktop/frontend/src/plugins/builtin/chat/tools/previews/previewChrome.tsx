import { useT } from "@/lib/i18n";

export const TEXT_PREVIEW_CLASS =
  "max-h-60 overflow-y-auto px-0 pt-1 pb-0 font-mono text-ui-md leading-body text-fg-muted";

export const INLINE_PREVIEW_ROW_LIMIT = 9;

export function PreviewOverflow({ count }: { count: number }) {
  const t = useT();
  if (count <= 0) return null;
  return <div className="text-fg-faint">… {t("tools.overflow.more", { count })}</div>;
}
