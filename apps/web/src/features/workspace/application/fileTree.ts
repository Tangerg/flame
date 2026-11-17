import { useWorkspaceDirectory, type WorkspaceFileEntry } from "./workspaceQueries";
export function useDirectory(cwd: string | undefined, path: string, enabled: boolean) {
  const query = useWorkspaceDirectory({ cwd, path, limit: 100 }, enabled);
  const entries = new Map<string, WorkspaceFileEntry>();
  for (const page of query.data?.pages ?? [])
    for (const entry of page.data) entries.set(entry.path, entry);
  return {
    ...query,
    entries: [...entries.values()].sort(
      (a, b) => Number(b.type === "dir") - Number(a.type === "dir") || a.name.localeCompare(b.name),
    ),
    last: query.data?.pages.at(-1),
  };
}
