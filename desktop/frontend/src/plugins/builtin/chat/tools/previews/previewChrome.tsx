import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import { vocab } from "@/ui";

export const INLINE_PREVIEW_ROW_LIMIT = 9;

export function PreviewOverflow({ count }: { count: number }) {
  const t = useT();
  if (count <= 0) return null;
  return <div {...stylex.props(vocab.faint)}>… {t("tools.overflow.more", { count })}</div>;
}
