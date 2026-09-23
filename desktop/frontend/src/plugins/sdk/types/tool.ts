import type { ComponentType } from "react";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";

export interface ToolPreviewProps {
  tool: ToolCall;
}
export type ToolPreviewComponent = ComponentType<ToolPreviewProps>;

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
  open: (tool: ToolCall) => void | Promise<void>;
}
