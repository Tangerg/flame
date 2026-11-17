import { type Ref } from "react";
import { PierreFile } from "@/lib/highlight/PierreFile";
export function FileView({
  path,
  content,
  startLine,
  targetLine,
  intent,
  ref,
}: {
  path: string;
  content: string;
  startLine: number;
  targetLine: number;
  intent: object | null;
  ref?: Ref<HTMLDivElement>;
}) {
  return (
    <div ref={ref} data-quote-source="file" data-quote-path={path}>
      <PierreFile
        path={path}
        source={content}
        startLine={startLine}
        targetLine={targetLine}
        intent={intent}
      />
    </div>
  );
}
