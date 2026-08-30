import type { ReactNode } from "react";

/**
 * Crossfades `rest` into `hover` while the enclosing control is hovered or keyboard-focused.
 *
 * There is no state and no render: the control's own `:hover` / `:focus-visible` drives it in
 * CSS. That is the point. It also makes the affordance available to a control that carries a
 * LABEL beside its glyph, which is where a bare IconButton could not go.
 */
export function GlyphSwap({ rest, hover }: { rest: ReactNode; hover: ReactNode }) {
  return (
    <span className="t-icon-swap">
      <span className="t-icon" data-glyph="rest">
        {rest}
      </span>
      <span className="t-icon" data-glyph="hover">
        {hover}
      </span>
    </span>
  );
}
