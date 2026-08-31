import type { ReactNode } from "react";

/**
 * Crossfades `rest` into `hover` while the enclosing control is hovered or keyboard-focused.
 *
 * CSS-driven, so a hover renders nothing. A glyph that rebuilds its DOM between mousedown and
 * mouseup loses the click: the browser fires none once the mousedown target is detached.
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
