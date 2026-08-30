// Plugin-contributed tool surface: inline previews + header actions +
// icon glyphs for tool function names.

import type { ComponentType } from "react";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";

export interface ToolPreviewProps {
  tool: ToolCall;
  /** Absent when the tool has no workspace view, in which case the preview hides its foot
   *  rather than offering a dead button. */
  onOpenView?: () => void;
}
export type ToolPreviewComponent = ComponentType<ToolPreviewProps>;

/**
 * A button on every ToolCard header, before the expand button. The optional `predicate`
 * scopes the action to a subset of tool calls.
 */
export interface ToolActionSpec {
  id: string;
  /** Icon name. */
  icon: string;
  /** Tooltip / aria label — a catalog key, resolved where the action renders
   *  (see `CommandSpec.label`: a contribution is registered once, and nothing
   *  re-registers on a language switch). */
  title: string;
  /** Lower comes first. */
  order?: number;
  /** Optional gate — return false to hide the action for this tool. */
  predicate?: (tool: ToolCall) => boolean;
  run: (tool: ToolCall) => void | Promise<void>;
}

export interface ToolViewOpenerSpec {
  id: string;
  order?: number;
  predicate: (tool: ToolCall) => boolean;
  open: (tool: ToolCall) => void | Promise<void>;
}
