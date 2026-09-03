import { useId } from "react";
import { cn } from "@/lib/classNames";

interface SparklineProps {
  data: readonly number[];
  label: string;
  className?: string;
}

export function Sparkline({ data, label, className }: SparklineProps) {
  const gradientId = useId();
  if (data.length < 2) return null;

  const min = Math.min(...data);
  const max = Math.max(...data);
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
      {/* `strokeWidth` is in DEVICE pixels here, not viewBox units: `non-scaling-stroke` is
          what keeps the line even after `preserveAspectRatio="none"` squashes 100×100 into
          48×16. Six of them covered better than a third of the plot's height. */}
      <polyline
        points={points.join(" ")}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        vectorEffect="non-scaling-stroke"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
