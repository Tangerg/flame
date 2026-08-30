import { cn } from "@/lib/classNames";

const CIRCUMFERENCE = 2 * Math.PI * 6;

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
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox="0 0 16 16"
      className={cn("size-[var(--icon-sm)] shrink-0 -rotate-90", className)}
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
