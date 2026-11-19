import { type Ref, type RefObject } from "react";
import { quoteSourceProps } from "@flame/ui";
import { PierreFile } from "@/lib/highlight/PierreFile";
export function FileView({
  path,
  content,
  startLine,
  targetLine,
  intent,
  ref,
  selectionContentRef,
}: {
  path: string;
  content: string;
  startLine: number;
  targetLine: number;
  intent: object | null;
  ref?: Ref<HTMLDivElement>;
  selectionContentRef: RefObject<Element | null>;
}) {
  return (
    <div ref={ref} {...quoteSourceProps({ kind: "file", path })}>
      <PierreFile
        path={path}
        source={content}
        startLine={startLine}
        targetLine={targetLine}
        intent={intent}
        selectionContentRef={selectionContentRef}
      />
    </div>
  );
}
