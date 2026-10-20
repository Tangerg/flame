import { useMemo, type ComponentType, type ReactNode } from "react";
import { type CapturedToolResult } from "@flame/component-sdk";
import { type ToolPreviewProps } from "@/features/chat/public/tools/types";
export function RecordedToolComponent({
  tool,
  fallback,
  entry: Entry,
}: ToolPreviewProps & {
  fallback: ReactNode;
  entry: ComponentType<{ toolResult: CapturedToolResult; fallback: ReactNode }>;
}) {
  const toolResult = useMemo<CapturedToolResult>(() => {
    const value = structuredClone(tool);
    return Object.freeze({
      ...value,
      range: value.range ? Object.freeze(value.range) : undefined,
      changes: value.changes
        ? Object.freeze(value.changes.map((change) => Object.freeze(change)))
        : undefined,
    });
  }, [tool]);
  if (tool.status === "running" || typeof tool.result !== "string") return fallback;
  return <Entry toolResult={toolResult} fallback={fallback} />;
}
