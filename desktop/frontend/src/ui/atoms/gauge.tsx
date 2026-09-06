import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";

const CIRCUMFERENCE = 2 * Math.PI * 6;

// Rotated so zero starts at twelve o'clock rather than three. The stroke is `currentColor`,
// so the ink is the caller's own text colour — not an escape hatch but the thing the gauge
// reads: it sits in a row and takes that row's meaning.
const styles = stylex.create({
  dial: {
    width: "var(--icon-sm)",
    height: "var(--icon-sm)",
    flexShrink: 0,
    transform: "rotate(-90deg)",
  },
});

export function Gauge({
  value,
  label,
  className,
}: {
  value: number;
  label: string;
  className?: string;
}) {
  const ratio = Math.min(1, Math.max(0, value));
  const dial = stylex.props(styles.dial);
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox="0 0 16 16"
      {...dial}
      className={cn(dial.className, className)}
    >
      <circle
        cx="8"
        cy="8"
        r="6"
        fill="none"
        stroke="var(--gauge-track)"
        strokeWidth="2.5"
        vectorEffect="non-scaling-stroke"
      />
      <circle
        cx="8"
        cy="8"
        r="6"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeDasharray={`${ratio * CIRCUMFERENCE} ${CIRCUMFERENCE}`}
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}
