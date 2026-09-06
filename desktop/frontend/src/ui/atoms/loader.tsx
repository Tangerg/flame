import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { color, motion, type } from "@/styles/tokens.stylex";

type LoaderSize = "sm" | "md" | "lg";

export interface LoaderProps {
  size?: LoaderSize;
  text?: string;
  className?: string;
}

/**
 * Waiting, said one way. This carried seven variants — dots, typing, pulse-dot, wave, bars,
 * terminal, shimmer — and both call sites in the tree asked for the shimmer, so six of them
 * and five sets of keyframes animated nothing. No guard could see it: a `variant` the product
 * never passes is still reachable through the union, so `knip` reads the component as used and
 * `check-dead-utilities` reads every class as emitted.
 *
 * A variant this needs again is one rung to add back, which is cheaper than six kept warm.
 *
 * No live region of its own. It carried an `<output>` saying "Loading", which said less than
 * the visible text beside it and less than `RunAnnouncer` — the one owner of "what is the run
 * doing" — already says. It also never spoke: this mounts WITH its content, and a region a
 * reader first meets already carrying a message announces nothing, which is the rule the
 * announcer's own test is built on. The visible label cannot take its place either, since it
 * counts elapsed time and would announce a new one every second.
 *
 * First component on StyleX. `className` survives on purpose: every caller is still Tailwind,
 * and a migration that demands both ends move at once is a rewrite. `stylex.props()` yields a
 * class list like any other, so the incoming one composes after it and still wins.
 */
const styles = stylex.create({
  root: {
    display: "inline-block",
    fontWeight: 500,
    // The gradient is the visible text: it is clipped to the glyphs, which are transparent.
    backgroundImage: `linear-gradient(90deg, ${color.fgMuted} 35%, ${color.fg} 50%, ${color.fgMuted} 65%)`,
    backgroundSize: "200% 100%",
    backgroundClip: "text",
    color: "transparent",
    animation: motion.shimmer,
  },
  // Not `animation: none`: the token carries a duration that already tracks `--motion-scale`,
  // and the reduced-motion answer is to stop moving, not to unset what is playing.
  still: {
    animation: {
      default: motion.shimmer,
      "@media (prefers-reduced-motion: reduce)": "none",
    },
  },
});

const SIZE = { sm: type.uiXs, md: type.uiSm, lg: type.uiMd } as const;

export function Loader({ size = "md", text: label = "Thinking", className }: LoaderProps) {
  const props = stylex.props(styles.root, styles.still, SIZE[size]);
  return (
    <span data-slot="loader" {...props} className={cn(props.className, className)}>
      {label}
    </span>
  );
}
