import { applyEdits, format } from "jsonc-parser";
import { type ToolCall } from "@/features/agent/public/sessionView";
export interface ToolInspectorBody {
  text: string;
  isJson: boolean;
}

export interface ToolInspectorModel {
  args: ToolInspectorBody;
  result: ToolInspectorBody;
  showNoResult: boolean;
}

export function formatToolInspectorBody(raw: string | undefined): ToolInspectorBody {
  if (!raw) return emptyBody();
  const trimmed = raw.trim();
  if (!trimmed) return emptyBody();
  if (!looksStructured(trimmed) || !isStrictJson(trimmed)) return { text: raw, isJson: false };
  return { text: reindentJson(trimmed), isJson: true };
}

export function toolInspectorModel(
  tool: Pick<ToolCall, "args" | "result" | "status">,
): ToolInspectorModel {
  const args = formatToolInspectorBody(tool.args);
  const result = formatToolInspectorBody(tool.result);
  return {
    args,
    result,
    showNoResult: !result.text && tool.status === "ok",
  };
}

function emptyBody(): ToolInspectorBody {
  return { text: "", isJson: false };
}

function looksStructured(value: string): boolean {
  return value[0] === "{" || value[0] === "[";
}

function isStrictJson(source: string): boolean {
  try {
    JSON.parse(source);
    return true;
  } catch (error) {
    if (error instanceof SyntaxError) return false;
    throw error;
  }
}

// Re-serializing parsed values would round unsafe integers, collapse -0 and
// overflowing exponents, and drop duplicate members; editing only the
// whitespace between tokens shows exactly what the tool reported.
function reindentJson(source: string): string {
  return applyEdits(
    source,
    format(source, undefined, { tabSize: 2, insertSpaces: true, eol: "\n" }),
  );
}
