import { useMemo, type ComponentType, type ReactNode } from "react";
import type { CapturedFile } from "@flame/plugin-sdk";
import type { WorkspaceFileRendererProps } from "@/plugins/sdk/types/workspace";

export function RecordedFileComponent({
  cwd,
  path,
  content,
  startLine,
  totalLines,
  truncated,
  fallback,
  entry: Entry,
}: WorkspaceFileRendererProps & {
  entry: ComponentType<{ file: CapturedFile; fallback: ReactNode }>;
}) {
  const file = useMemo<CapturedFile>(
    () =>
      Object.freeze({
        workspace: cwd ? Object.freeze({ path: cwd }) : undefined,
        path,
        content,
        startLine,
        totalLines,
        truncated: truncated === true,
      }),
    [cwd, path, content, startLine, totalLines, truncated],
  );
  if (startLine !== 1 || truncated) return fallback;
  return <Entry file={file} fallback={fallback} />;
}
