import type { ToolDetail } from "@/plugins/builtin/agent/public/messagePresentation";
import * as stylex from "@stylexjs/stylex";
import { FilePath, vocab } from "@/ui";
import { cn } from "@/lib/classNames";

export function ToolText({ value, className }: { value: ToolDetail; className?: string }) {
  if (value.kind === "path") {
    return <FilePath path={value.value} className={className} />;
  }
  return (
    <span className={cn(stylex.props(vocab.truncate).className, className)} title={value.value}>
      {value.value}
    </span>
  );
}
