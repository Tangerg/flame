import { cn } from "@/lib/classNames";

const CIRCUMFERENCE = 2 * Math.PI * 6;

// Prints no value of its own: a gauge that did would grow and shrink with the digits and
// reflow the control row it sits in. The number belongs in the caller's tooltip.
export function Gauge({
  value,
  label,
  className,
}: {
  /** 0…1; values outside are clamped. */
  value: number;
  label: string;
  className?: string;
}) {
  const ratio = Math.min(1, Math.max(0, value));
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox="0 0 16 16"
      // SVG starts an arc at three o'clock; the rotation moves it to twelve. Rotating the
      // whole element is exact because the track is a full circle.
      className={cn("size-[var(--icon-sm)] shrink-0 -rotate-90", className)}
    >
      <circle
        cx="8"
        cy="8"
        r="6"
        fill="none"
        // `--gauge-track`, not `--color-border`: that token is a chrome hairline and
        // vanishes at this weight — the track is data.
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
