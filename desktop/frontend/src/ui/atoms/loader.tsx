import * as stylex from "@stylexjs/stylex";
import { color, motion, type, weight } from "@/styles/tokens.stylex";

export interface LoaderProps {
  text: string;
}

const styles = stylex.create({
  root: {
    display: "inline-block",
    fontWeight: weight.regular,
    backgroundImage: `linear-gradient(90deg, ${color.fg} 35%, ${color.fgMuted} 50%, ${color.fg} 65%)`,
    backgroundSize: "200% 100%",
    backgroundPosition: "150% 0",
    backgroundClip: "text",
    color: "transparent",
    animation: {
      default: motion.shimmer,
      "@media (prefers-reduced-motion: reduce)": "none",
    },
  },
});

export function Loader({ text }: LoaderProps) {
  return (
    <span data-slot="loader" {...stylex.props(styles.root, type.uiSm)}>
      {text}
    </span>
  );
}
