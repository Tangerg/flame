import type { ToolDetail } from "@/plugins/builtin/agent/public/messagePresentation";
import { FilePath } from "@/ui";
import { cn } from "@/lib/classNames";

/** A path keeps its filename when the row runs out of width, which is the whole reason the
 *  model names which kind of value it holds — and why both slots come through here. */
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
