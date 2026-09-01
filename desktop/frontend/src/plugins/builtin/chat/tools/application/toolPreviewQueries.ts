import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { useWorkspaceFileHead, useWorkspaceGrep } from "@/plugins/builtin/workspace/public/queries";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { searchToolResult } from "@/plugins/sdk";

export function useFileToolPreview(tool: ToolCall, maxLines: number) {
  const workspace = useActiveSessionWorkspace();
  const path = tool.fn && tool.fn !== tool.name ? tool.fn : undefined;
  return useWorkspaceFileHead(
    path && workspace.status === "ready"
      ? { path, cwd: workspace.cwd, lines: maxLines }
      : undefined,
  );
}

interface GrepPreviewRow {
  loc: string;
  text: string;
}

// The runtime projects every grep output mode into one `hits` envelope, so nothing here
// has to guess which mode produced the rows.
function inlineGrepRows(result: string | undefined): GrepPreviewRow[] | undefined {
  const hits = searchToolResult(result)?.hits;
  if (!Array.isArray(hits)) return undefined;
  return hits.map((hit) => ({
    loc: hit?.lineNumber === undefined ? (hit?.path ?? "") : `${hit.path}:${hit.lineNumber}`,
    text: hit?.snippet ?? "",
  }));
}

export function useGrepToolPreview(tool: ToolCall, maxMatches: number) {
  const inline = inlineGrepRows(tool.result);
  const workspace = useActiveSessionWorkspace();
  const query =
    !inline && tool.name === "grep" && tool.fn && tool.fn !== "search" ? tool.fn : undefined;
  const { data } = useWorkspaceGrep(
    query && workspace.status === "ready"
      ? { query, cwd: workspace.cwd, limit: maxMatches }
      : undefined,
  );
  const rows =
    inline ??
    (data?.matches ?? []).map((match) => ({
      loc: `${match.path}:${match.lineNumber}`,
      text: match.text,
    }));
  const shown = rows.slice(0, maxMatches);
  return {
    shown,
    overflow: inline ? rows.length - shown.length : (data?.total ?? 0) - shown.length,
  };
}
