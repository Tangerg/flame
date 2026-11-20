import { type Ref, type RefObject } from "react";
import { quoteSourceProps } from "@flame/ui";
import { PierreFile } from "@/lib/highlight/PierreFile";
export function FileView({
  path,
  content,
  startLine,
  endLine,
  targetLine,
  intent,
  ref,
  selectionContentRef,
  sourceScrollRef,
}: {
  path: string;
  content: string;
  startLine: number;
  endLine: number;
  targetLine: number;
  intent: object | null;
  ref?: Ref<HTMLDivElement>;
  selectionContentRef: RefObject<Element | null>;
  sourceScrollRef: RefObject<HTMLElement | null>;
}) {
  return (
    <div ref={ref} {...quoteSourceProps({ kind: "file", path })}>
      <PierreFile
        path={path}
        source={content}
        lineRange={{ start: startLine, end: endLine }}
        targetLine={targetLine}
        intent={intent}
        selectionContentRef={selectionContentRef}
        sourceScrollRef={sourceScrollRef}
      />
    </div>
  );
}
