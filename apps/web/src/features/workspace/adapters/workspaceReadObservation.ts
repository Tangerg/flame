import { type FlameClient } from "@flame/runtime-client";
import { queryClient, replaceCachedRead } from "@/lib/queryClient";
import {
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
} from "@/features/workspace/application/workspaceQueries";
import { type WorkspaceReadTarget } from "@/features/workspace/application/workspaceEventLoop";

export function observeWorkspaceDiffReads(
  runtime: FlameClient,
  cwd: string,
  componentSignal: AbortSignal,
): FlameClient {
  return {
    ...runtime,
    workspace(ref) {
      const workspace = runtime.workspace(ref);
      if (workspace.ref.path !== cwd) return workspace;
      return {
        ...workspace,
        diff: {
          ...workspace.diff,
          get(params, signal) {
            const mode = params?.mode ?? "worktree";
            const read = workspace.diff.get(params, signal);
            if (mode !== "worktree") return read;
            return read.then((diff) => {
              if (diff.type === "visual" && !componentSignal.aborted && !signal?.aborted) {
                // The comparison can be scoped or truncated. The summary owns its full read.
                void replaceCachedRead({
                  queryKey: [WORKSPACE_FILES_CHANGED_KEY, { cwd }],
                  exact: true,
                });
              }
              return diff;
            });
          },
        },
      };
    },
  };
}

export function workspaceReadTargets(): WorkspaceReadTarget[] {
  const scopes = new Map<string | undefined, Set<string>>();
  for (const query of queryClient.getQueryCache().getAll()) {
    const key = query.queryKey[0];
    if (key !== WORKSPACE_READ_FILE_KEY && key !== WORKSPACE_LIST_FILES_KEY) continue;
    if (!query.isActive()) continue;
    const params = query.queryKey[1] as { cwd?: string; path?: string } | undefined;
    if (!params) continue;
    const paths = scopes.get(params.cwd) ?? new Set<string>();
    paths.add(params.path || ".");
    scopes.set(params.cwd, paths);
  }
  return [...scopes]
    .sort(([a], [b]) => (a ?? "").localeCompare(b ?? ""))
    .map(([cwd, paths]) => ({
      ...(cwd ? { cwd } : {}),
      paths: [...paths].sort(),
    }));
}

export function subscribeWorkspaceReadTargets(onChange: () => void): () => void {
  let previous = JSON.stringify(workspaceReadTargets());
  return queryClient.getQueryCache().subscribe(() => {
    const next = JSON.stringify(workspaceReadTargets());
    if (next === previous) return;
    previous = next;
    onChange();
  });
}

export function installWorkspaceFocusRefresh(): () => void {
  const refresh = () => {
    if (document.visibilityState === "hidden") return;
    void replaceCachedRead({
      predicate: (query) =>
        [WORKSPACE_READ_FILE_KEY, WORKSPACE_LIST_FILES_KEY, WORKSPACE_FILES_CHANGED_KEY].includes(
          query.queryKey[0] as string,
        ),
    });
  };
  window.addEventListener("focus", refresh);
  document.addEventListener("visibilitychange", refresh);
  return () => {
    window.removeEventListener("focus", refresh);
    document.removeEventListener("visibilitychange", refresh);
  };
}
