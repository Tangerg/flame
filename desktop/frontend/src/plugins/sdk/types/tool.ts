import type { ComponentType, ReactNode } from "react";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import type { PluginRendererSelector } from "@flame/runtime-contract/wire";

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
  open: (tool: ToolCall) => void | Promise<void>;
}
