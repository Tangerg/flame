import { type ComponentType, type ReactNode } from "react";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { type RendererSelectorKind } from "@/foundation/protocolValues.generated";
export interface PluginRendererSelector {
  name: string;
  type: RendererSelectorKind;
}

import { defineExtensionPoint } from "@/extensions/contracts";
import { lookupExtensionOwner } from "@/extensions/selectors/extensions";
export interface ToolPreviewProps {
  tool: ToolCall;
}

export interface ToolResultRendererSpec {
  id: string;
  title: string;
  selector: PluginRendererSelector;
  component: ComponentType<ToolPreviewProps & { fallback: ReactNode }>;
}

export interface ToolActionSpec {
  id: string;
  icon: string;
  title: string;
  order?: number;
  predicate?: (tool: ToolCall) => boolean;
  run: (tool: ToolCall) => void | Promise<void>;
}

export interface ToolViewOpenerSpec {
  id: string;
  order?: number;
  predicate: (tool: ToolCall) => boolean;
  label: (tool: ToolCall) => string;
  open: (tool: ToolCall, origin: HTMLElement) => void | Promise<void>;
}

export const TOOL_ACTION = defineExtensionPoint<ToolActionSpec>({
  id: "flame.tool.action",
  keying: "single",
});

export const TOOL_VIEW_OPENER = defineExtensionPoint<ToolViewOpenerSpec>({
  id: "flame.tool.viewOpener",
  keying: "single",
});

export const TOOL_RESULT_RENDERER = defineExtensionPoint<ToolResultRendererSpec>({
  id: "flame.tool.resultRenderer",
  keying: "single",
});

export const TOOL_ICON = defineExtensionPoint<string>({
  id: "flame.tool.icon",
  keying: "single",
});

export const TOOL_STANDING_SURFACE = defineExtensionPoint<string>({
  id: "flame.tool.standingSurface",
  keying: "single",
});

export function lookupToolActionOwner(id: string): string | undefined {
  return lookupExtensionOwner(TOOL_ACTION, id);
}

export function lookupToolViewOpenerOwner(id: string): string | undefined {
  return lookupExtensionOwner(TOOL_VIEW_OPENER, id);
}
