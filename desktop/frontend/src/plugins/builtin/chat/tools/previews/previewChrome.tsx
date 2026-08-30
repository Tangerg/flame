// Shared container shape for the list/text inline previews. The disclosure's body
// carries no fill of its own — the card behind it is the ground — so a text preview
// is padding and typography, and only the panels that hold program output cut a
// well into it.
import { useT } from "@/lib/i18n";

export const TEXT_PREVIEW_CLASS =
  "max-h-60 overflow-y-auto px-0 pt-1 pb-0 font-mono text-ui-md leading-body text-fg-muted";

// The recessed well, so program output reads as cut INTO the card rather than stacked on
// it. Deliberately not a `bg-fg` panel — that inverts per theme and would turn bright on
// dark. The only fill inside a preview; the disclosure body stays the ground.
export const CODE_PREVIEW_CLASS =
  "max-h-60 overflow-y-auto rounded-sm bg-sunken px-3 py-2.5 font-mono text-code leading-relaxed text-fg-soft";

export const INLINE_PREVIEW_ROW_LIMIT = 9;

export function PreviewOverflow({ count }: { count: number }) {
  const t = useT();
  if (count <= 0) return null;
  return <div className="text-fg-faint">… {t("tools.overflow.more", { count })}</div>;
}
