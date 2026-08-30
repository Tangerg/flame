import { cn } from "@/lib/classNames";

export function FilePath({ path, className }: { path: string; className?: string }) {
  const cut = path.lastIndexOf("/");
  const directory = cut > 0 ? path.slice(0, cut) : "";
  const filename = cut >= 0 ? path.slice(cut + 1) : path;

  return (
    <span className={cn("flex min-w-0 items-baseline", className)} title={path}>
      {directory !== "" && (
        <span dir="rtl" className="min-w-0 max-w-max flex-1 truncate text-left text-fg-faint">
          <span dir="ltr">{directory}</span>
        </span>
      )}
      {cut >= 0 && <span className="shrink-0 text-fg-faint">/</span>}
      <span className="min-w-0 shrink truncate">{filename}</span>
    </span>
  );
}
