import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { searchToolResult } from "@/plugins/sdk";

interface RecordedLine {
  lineNumber: number;
  text: string;
}

export function recordedReadLines(
  tool: ToolCall,
  maxLines: number,
): { lines: RecordedLine[]; hidden: number } | null {
  if (tool.result === undefined) return null;
  const all = tool.result.replace(/\n$/, "").split("\n");
  const start = tool.range?.start ?? 1;
  return {
    lines: all.slice(0, maxLines).map((text, index) => ({ lineNumber: start + index, text })),
    hidden: Math.max(0, all.length - maxLines),
  };
}

interface GrepPreviewRow {
  loc: string;
  text: string;
}

export function recordedGrepRows(
  tool: ToolCall,
  maxMatches: number,
): { shown: GrepPreviewRow[]; overflow: number } | null {
  const hits = searchToolResult(tool.result)?.hits;
  if (!Array.isArray(hits)) return null;
  const rows = hits.map((hit) => ({
    loc: hit?.lineNumber === undefined ? (hit?.path ?? "") : `${hit.path}:${hit.lineNumber}`,
    text: hit?.snippet ?? "",
  }));
  const shown = rows.slice(0, maxMatches);
  return { shown, overflow: rows.length - shown.length };
}
