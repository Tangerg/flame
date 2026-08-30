import { useId } from "react";
import { cn } from "@/lib/classNames";

interface SparklineProps {
  /** Fewer than two points renders nothing. */
  data: readonly number[];
  label: string;
  className?: string;
}

// `currentColor`, no palette: the line takes whatever ink its row is set in, so a tone
// cannot end up with two answers to "what colour is this state".
export function Sparkline({ data, label, className }: SparklineProps) {
  const gradientId = useId();
  if (data.length < 2) return null;

  const min = Math.min(...data);
  const max = Math.max(...data);
  // A flat series has no range to divide by, and drawn on the bottom edge it would read
  // as zero rather than unchanged — so it runs down the middle.
  const range = max - min || 1;
  const flat = max === min;

  const points = data.map((value, index) => {
    const x = (index / (data.length - 1)) * 100;
    const y = flat ? 50 : 100 - ((value - min) / range) * 100;
    return `${x},${y}`;
  });

  return (
    <svg
      role="img"
      aria-label={label}
      viewBox="0 0 100 100"
      preserveAspectRatio="none"
      className={cn("h-4 w-12 overflow-visible", className)}
    >
      <defs>
        <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="currentColor" stopOpacity="0.18" />
          <stop offset="100%" stopColor="currentColor" stopOpacity="0" />
        </linearGradient>
      </defs>
      <polygon points={`0,100 ${points.join(" ")} 100,100`} fill={`url(#${gradientId})`} />
      <polyline
        points={points.join(" ")}
        fill="none"
        stroke="currentColor"
        strokeWidth="6"
        // Non-scaling stroke, because the viewBox is stretched to the element's box:
        // without it the line is thick horizontally and hairline vertically.
        vectorEffect="non-scaling-stroke"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
