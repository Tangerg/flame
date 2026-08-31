import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { TOOL_PREVIEW, useExtensionByKey, type ToolPreviewProps } from "@/plugins/sdk";
import { ToolInspector } from "./ToolInspector";
import { toolRoutingKey } from "../public/toolIcon";
import { createElement } from "react";

export function ToolPreview({ tool, onOpenView }: ToolPreviewProps) {
  const key = toolRoutingKey(tool);
  const Preview = useExtensionByKey(TOOL_PREVIEW, key);
  if (!Preview) {
    return <ToolInspector tool={tool} />;
  }
  return (
    <PluginBoundary plugin={key} label={`${tool.fn} preview`}>
      {createElement(Preview, { tool, onOpenView })}
    </PluginBoundary>
  );
}
