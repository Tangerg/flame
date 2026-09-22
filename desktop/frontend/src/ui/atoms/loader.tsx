import * as stylex from "@stylexjs/stylex";
import { color, motion, type, weight } from "@/styles/tokens.stylex";

export interface LoaderProps {
  text: string;
}

const styles = stylex.create({
  root: {
    display: "inline-block",
    fontWeight: weight.regular,
    // The gradient IS the visible text — clipped to glyphs that are themselves transparent.
    // Solid ink with a faded band passing through, not the reverse: the label stays on screen
    // for the length of a turn, so its resting state is the state it is read in.
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
