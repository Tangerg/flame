import { useQueryClient } from "@tanstack/react-query";
import {
  type WorkspaceFileEntry,
  useWorkspaceFileLists,
  workspaceFileListQuery,
} from "@/features/workspace/application/workspaceQueries";

export function useFileTreeData(
  roots: readonly WorkspaceFileEntry[],
  cwd: string | undefined,
  expanded: readonly string[],
  subscribed: boolean,
) {
  const client = useQueryClient();
  const opened = new Set(expanded);
  const requests: Array<{ cwd: string | undefined; path: string }> = [];
  const collect = (entries: readonly WorkspaceFileEntry[]) => {
    for (const entry of entries) {
      if (entry.type !== "dir" || !opened.has(entry.path)) continue;
      const request = { cwd, path: entry.path };
      requests.push(request);
      collect(client.getQueryData(workspaceFileListQuery(request).queryKey) ?? []);
    }
  };
  collect(roots);
  const results = useWorkspaceFileLists(requests, subscribed);
  const directories = new Map(requests.map((request, index) => [request.path, results[index]!]));
  const entries = new Map(roots.map((entry) => [entry.path, entry]));
  for (const query of results) {
    for (const entry of query.data ?? []) entries.set(entry.path, entry);
  }
  return { entries, directories };
}
