import { cn } from "@/lib/classNames";

export function DiffStat({
  added,
  removed,
  binary,
  className,
}: {
  added: number;
  removed: number;
  binary?: string;
  className?: string;
}) {
  const base = cn("shrink-0 items-center gap-1.5 font-mono", className);

  if (binary !== undefined) {
    return <span className={cn("inline-flex text-fg-faint", base)}>{binary}</span>;
  }
  if (added === 0 && removed === 0) {
    return (
      <span aria-hidden className={cn("inline-flex text-fg-faint", base)}>
        —
      </span>
    );
  }

  return (
    <span className={cn("inline-flex", base)}>
      {added > 0 && <span className="text-success">+{added}</span>}
      {removed > 0 && <span className="text-negative">−{removed}</span>}
    </span>
  );
}
