import type { Contributor } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import type {
  FlameClient,
  WorkspaceFileChange as RpcFileChange,
  WorkspaceSummary as RpcWorkspaceSummary,
} from "@flame/runtime-contract/client";
import { readInstalledMemory } from "../public/memory";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { agentMemoryEntry } from "./runtimeAgentMemoryGateway";
import {
  WORKSPACE_PROJECTS_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_AGENT_MEMORY_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
  type WorkspaceFileChangesQuery,
  type WorkspaceListFilesQuery,
  type WorkspaceReadFileQuery,
  type AgentMemoryQuery,
  type WorkspaceFileChange as WorkspaceFileChangeSummary,
  type WorkspaceProjectSummary,
} from "../application/workspaceQueries";

function requiredParams<P>(key: string, params: unknown): P {
  if (params === undefined) throw new Error(`Data provider "${key}" requires parameters`);
  return params as P;
}

function pageData<T>(request: Promise<{ data: T[] }>): Promise<T[]> {
  return request.then((page) => page.data);
}

export function registerWorkspaceDataProviders(
  ctx: Contributor,
  runtimeClient: () => FlameClient,
): void {
  const workspace = (cwd: string | undefined, signal: AbortSignal | undefined) =>
    runtimeClient().workspaces.open(cwd ? { path: cwd } : undefined, signal);
  ctx.contribute(DATA_PROVIDER, {
    key: WORKSPACE_PROJECTS_KEY,
    fetcher: async (_params, signal) =>
      (await pageData(runtimeClient().workspaces.list(signal))).map(toWorkspaceProjectSummary),
  });
  ctx.contribute(DATA_PROVIDER, {
    key: WORKSPACE_FILES_CHANGED_KEY,
    fetcher: async (params, signal) => {
      const resources = await workspace(
        (params as WorkspaceFileChangesQuery | undefined)?.cwd,
        signal,
      );
      return (await pageData(resources.changes.list(signal))).map(toWorkspaceFileChangeSummary);
    },
  });
  ctx.contribute(DATA_PROVIDER, {
    key: WORKSPACE_AGENT_MEMORY_KEY,
    fetcher: async (params, signal) => {
      const q = requiredParams<AgentMemoryQuery>(WORKSPACE_AGENT_MEMORY_KEY, params);
      if (!runtimeCapability("plugins")) return [];
      const client = runtimeClient();
      const cwd =
        q.scope === "project"
          ? (await client.workspaces.open(q.cwd ? { path: q.cwd } : undefined, signal)).ref.path
          : "";
      const result = await readInstalledMemory(client, q.scope, cwd, signal);
      return result.map(agentMemoryEntry);
    },
  });
  ctx.contribute(DATA_PROVIDER, {
    key: WORKSPACE_LIST_FILES_KEY,
    fetcher: async (params, signal) => {
      const { cwd, ...query } = requiredParams<WorkspaceListFilesQuery>(
        WORKSPACE_LIST_FILES_KEY,
        params,
      );
      const resources = await workspace(cwd, signal);
      return (await resources.files.list(query, signal).autoPagingToArray()).map((e) => ({
        path: e.path,
        name: e.name,
        type: e.type,
        sizeBytes: e.sizeBytes,
      }));
    },
  });
  ctx.contribute(DATA_PROVIDER, {
    key: WORKSPACE_READ_FILE_KEY,
    fetcher: async (params, signal) => {
      const { cwd, ...query } = requiredParams<WorkspaceReadFileQuery>(
        WORKSPACE_READ_FILE_KEY,
        params,
      );
      const r = await (await workspace(cwd, signal)).files.read(query, signal);
      return {
        content: r.content,
        startLine: r.startLine ?? 1,
        totalLines: r.totalLines,
        truncated: r.truncated,
      };
    },
  });
}

function toWorkspaceProjectSummary(summary: RpcWorkspaceSummary): WorkspaceProjectSummary {
  return {
    id: summary.workspace.ref.path,
    name: summary.name,
    sessionCount: summary.sessionCount,
    ...(summary.workspace.availability === "missing" ? { cwdMissing: true } : {}),
  };
}

const FILE_CHANGE: Record<RpcFileChange["status"], WorkspaceFileChangeSummary["change"]> = {
  added: "add",
  untracked: "add",
  modified: "mod",
  renamed: "renamed",
  deleted: "del",
};

function toWorkspaceFileChangeSummary(change: RpcFileChange): WorkspaceFileChangeSummary {
  return {
    path: change.path,
    change: FILE_CHANGE[change.status],
    ...(change.previousPath ? { previousPath: change.previousPath } : {}),
    added: change.added,
    removed: change.removed,
    binary: change.binary,
  };
}
