import * as stylex from "@stylexjs/stylex";
import { color, motion, type, weight } from "@/styles/tokens.stylex";

type LoaderSize = "sm" | "md" | "lg";

export interface LoaderProps {
  size?: LoaderSize;
  text?: string;
}

const styles = stylex.create({
  root: {
    display: "inline-block",
    fontWeight: weight.medium,
    // The gradient is the visible text: it is clipped to the glyphs, which are transparent.
    backgroundImage: `linear-gradient(90deg, ${color.fgMuted} 35%, ${color.fg} 50%, ${color.fgMuted} 65%)`,
    backgroundSize: "200% 100%",
    backgroundClip: "text",
    color: "transparent",
    // Not `animation: none`: the token carries a duration that already tracks `--motion-scale`,
    // and the reduced-motion answer is to stop moving, not to unset what is playing.
    animation: {
      default: motion.shimmer,
      "@media (prefers-reduced-motion: reduce)": "none",
    },
  },
});

const SIZE = { sm: type.uiXs, md: type.uiSm, lg: type.uiMd } as const;

/**
 * Waiting, said one way.
 *
 * No live region of its own: `RunAnnouncer` is the one owner of "what is the run doing", and a
 * region a reader first meets already carrying a message announces nothing.
 */
export function Loader({ size = "md", text: label = "Thinking" }: LoaderProps) {
  return (
    <span data-slot="loader" {...stylex.props(styles.root, SIZE[size])}>
      {label}
    </span>
  );
}
