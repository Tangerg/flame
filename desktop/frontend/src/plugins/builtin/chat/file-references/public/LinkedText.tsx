import { useMemo } from "react";
import { parseFileRefs } from "@/plugins/builtin/agent/public/fileRefs";
import { FileRefLink } from "./FileRefLink";

export function LinkedText({ text }: { text: string }) {
  const segments = useMemo(() => parseFileRefs(text), [text]);
  if (segments.length === 1 && typeof segments[0] === "string") return text;
  return (
    <>
      {segments.map((seg, i) =>
        typeof seg === "string" ? (
          seg
        ) : (
          <FileRefLink key={i} path={seg.path} line={seg.line} column={seg.column} />
        ),
      )}
    </>
  );
}
