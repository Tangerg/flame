import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import { color, leading, space, type } from "@/styles/tokens.stylex";
import { vocab } from "@/ui";

const previewText = stylex.create({
  block: {
    maxHeight: "calc(var(--spacing) * 60)",
    overflowY: "auto",
    paddingInline: 0,
    paddingTop: space.s1,
    paddingBottom: 0,
    fontFamily: "var(--font-mono)",
    lineHeight: leading.body,
    color: color.fgMuted,
  },
});

/**
 * A block of machine text a preview shows verbatim: it scrolls rather than pushing the
 * transcript, and it is mono because what it holds is output and not prose.
 *
 * An ARRAY, not one style, because the block carries a type step and its children inherit it.
 * As a single style the step went missing and every preview's footer grew from 14px to the
 * document's 16 — which is the same mistake as writing `fontSize` out by hand, one step
 * further along: there, a third of a step was copied; here, a whole one was dropped.
 */
export const TEXT_PREVIEW = [previewText.block, type.uiMd];

export const INLINE_PREVIEW_ROW_LIMIT = 9;

export function PreviewOverflow({ count }: { count: number }) {
  const t = useT();
  if (count <= 0) return null;
  return <div {...stylex.props(vocab.faint)}>… {t("tools.overflow.more", { count })}</div>;
}
