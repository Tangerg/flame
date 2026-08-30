import type { ToolDetail } from "@/plugins/builtin/agent/public/messagePresentation";
import { FilePath } from "@/ui";
import { cn } from "@/lib/classNames";

export function ToolText({ value, className }: { value: ToolDetail; className?: string }) {
  if (value.kind === "path") {
    return <FilePath path={value.value} className={className} />;
  }
  return (
    <span className={cn("truncate", className)} title={value.value}>
      {value.value}
    </span>
  );
}
