import * as stylex from "@stylexjs/stylex";
import { motion, radius, space, surface } from "@/styles/tokens.stylex";

const styles = stylex.create({
  line: {
    position: "relative",
    display: "inline-block",
    overflow: "hidden",
    borderRadius: radius.xs,
    backgroundColor: surface.surface2,
  },
  // The sweep is a light passing over the placeholder, so it is drawn on its own layer rather
  // than as a background of the line: the line keeps its fill visible under the moving highlight.
  sweep: {
    position: "absolute",
    inset: 0,
    backgroundImage:
      "linear-gradient(100deg, transparent 0%, var(--color-surface) 50%, transparent 100%)",
    animation: { default: motion.sweep, "@media (prefers-reduced-motion: reduce)": "none" },
  },
  rowStacked: { display: "flex", flexDirection: "column", gap: space.s1_5, paddingBlock: space.s2 },
  rowCompact: {
    display: "flex",
    height: "calc(var(--spacing) * 7)",
    alignItems: "center",
    gap: space.s2,
    paddingInline: space.s2,
  },
  listStacked: {
    display: "flex",
    flexDirection: "column",
    gap: space.s2,
    paddingInline: space.s3,
    paddingBlock: space.s2,
  },
  listCompact: {
    display: "flex",
    flexDirection: "column",
    gap: space.s0_5,
    paddingBlock: space.s1,
  },
});

function SkeletonLine({ width = "100%", height = 10 }: { width?: string; height?: number }) {
  return (
    <span {...stylex.props(styles.line)} style={{ width, height }}>
      <span aria-hidden {...stylex.props(styles.sweep)} />
    </span>
  );
}

function SkeletonRow({ variant }: { variant: SkeletonListVariant }) {
  if (variant === "compact") {
    return (
      <div {...stylex.props(styles.rowCompact)}>
        <SkeletonLine width="14px" height={14} />
        <SkeletonLine width="62%" height={8} />
      </div>
    );
  }
  return (
    <div {...stylex.props(styles.rowStacked)}>
      <SkeletonLine width="68%" />
      <SkeletonLine width="38%" height={8} />
    </div>
  );
}

export type SkeletonListVariant = "stacked" | "compact";

export function SkeletonList({
  count = 4,
  label = "Loading…",
  variant = "stacked",
}: {
  count?: number;
  label?: string;
  variant?: SkeletonListVariant;
}) {
  return (
    <output
      {...stylex.props(variant === "compact" ? styles.listCompact : styles.listStacked)}
      aria-busy="true"
      aria-live="polite"
    >
      <span className="sr-only">{label}</span>
      {Array.from({ length: count }, (_, i) => (
        <SkeletonRow key={i} variant={variant} />
      ))}
    </output>
  );
}
