import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import {
  TOOL_PREVIEW,
  toolResultShape,
  useExtensionByKey,
  type ToolPreviewProps,
} from "@/plugins/sdk";
import { ToolInspector } from "./ToolInspector";
import { toolRoutingKey, toolShapeKey } from "../public/toolIcon";
import { createElement } from "react";

export function ToolPreview({ tool, onOpenView }: ToolPreviewProps) {
  const name = toolRoutingKey(tool);
  const shape = toolResultShape(tool.result);
  const byName = useExtensionByKey(TOOL_PREVIEW, name);
  const byShape = useExtensionByKey(TOOL_PREVIEW, shape ? toolShapeKey(shape) : "");
  const Preview = byName ?? byShape;
  if (!Preview) {
    return <ToolInspector tool={tool} />;
  }
  const key = byName ? name : toolShapeKey(shape!);
  return (
    <PluginBoundary plugin={key} label={`${tool.fn} preview`}>
      {createElement(Preview, { tool, onOpenView })}
    </PluginBoundary>
  );
}
