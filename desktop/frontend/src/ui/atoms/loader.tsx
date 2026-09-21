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
    //
    // SOLID ink with a faded band passing through it, which is the direction zcode's own
    // thinking label takes and the legible one: the label sits on screen for the length of the
    // turn, so the state it rests in is the state it is read in, and resting muted spends that
    // whole time below the contrast of the words around it.
    backgroundImage: `linear-gradient(90deg, ${color.fg} 35%, ${color.fgMuted} 50%, ${color.fg} 65%)`,
    backgroundSize: "200% 100%",
    // The window outside the band, so a word whose sweep is not playing is solid.
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
