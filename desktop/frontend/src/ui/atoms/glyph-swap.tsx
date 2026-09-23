import type { ReactNode } from "react";

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
