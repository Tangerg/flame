import * as stylex from "@stylexjs/stylex";
import { color, motion, type, weight } from "@/styles/tokens.stylex";

export interface LoaderProps {
  text: string;
}

const styles = stylex.create({
  root: {
    display: "inline-block",
    fontWeight: weight.regular,
    // The gradient is the visible text: it is clipped to the glyphs, which are transparent.
    backgroundImage: `linear-gradient(90deg, ${color.fgMuted} 35%, ${color.fg} 50%, ${color.fgMuted} 65%)`,
    backgroundSize: "200% 100%",
    // The one window that clears the gradient's ink band. Stated so that a word whose sweep is
    // not playing is the same muted word the cadence rests on.
    backgroundPosition: "150% 0",
    backgroundClip: "text",
    color: "transparent",
    animation: {
      default: motion.shimmer,
      "@media (prefers-reduced-motion: reduce)": "none",
    },
  },
});

/**
 * Waiting, said one way.
 *
 * No live region of its own: `RunAnnouncer` is the one owner of "what is the run doing", and a
 * region a reader first meets already carrying a message announces nothing.
 */
export function Loader({ text }: LoaderProps) {
  return (
    <span data-slot="loader" {...stylex.props(styles.root, type.uiSm)}>
      {text}
    </span>
  );
}
