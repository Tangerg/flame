import type { ReactNode } from "react";

// CSS-driven, never React state: a glyph that rebuilds its DOM between mousedown and mouseup
// loses the click entirely — the browser fires none once the mousedown target is detached.
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
