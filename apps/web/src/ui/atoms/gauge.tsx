import * as stylex from "@stylexjs/stylex";
import { clamp01 } from "@/lib/clamp";
import { clsx } from "clsx";
const RING_RADIUS = 6.5;
const PIE_RADIUS = 4.5;

const styles = stylex.create({
  dial: {
    width: "var(--icon-sm)",
    height: "var(--icon-sm)",
    flexShrink: 0,
  },
});

// A filled sector inside a ring reads as an amount; a stroked arc reads as a spinner.
function sectorPath(ratio: number): string {
  const angle = ratio * 2 * Math.PI;
  const x = 8 + PIE_RADIUS * Math.sin(angle);
  const y = 8 - PIE_RADIUS * Math.cos(angle);
  const largeArc = angle > Math.PI ? 1 : 0;
  return `M8 8 L8 ${8 - PIE_RADIUS} A${PIE_RADIUS} ${PIE_RADIUS} 0 ${largeArc} 1 ${x} ${y} Z`;
}

export function Gauge({
  value,
  label,
  className,
}: {
  value: number;
  label: string;
  className?: string;
}) {
  const ratio = clamp01(value);
  const dial = stylex.props(styles.dial);
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox="0 0 16 16"
      {...dial}
      className={clsx(dial.className, className)}
    >
      <circle
        cx="8"
        cy="8"
        r={RING_RADIUS}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.25"
        vectorEffect="non-scaling-stroke"
      />
      {ratio >= 1 ? (
        <circle cx="8" cy="8" r={PIE_RADIUS} fill="currentColor" />
      ) : ratio > 0 ? (
        <path d={sectorPath(ratio)} fill="currentColor" />
      ) : null}
    </svg>
  );
}
