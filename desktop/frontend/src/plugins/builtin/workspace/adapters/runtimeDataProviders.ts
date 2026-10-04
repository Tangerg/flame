import type { Contributor } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import type {
  FlameClient,
  WorkspaceFileChange as RpcFileChange,
  WorkspaceSummary as RpcWorkspaceSummary,
} from "@flame/runtime-contract/client";
import { emptyListIfUngated } from "@/lib/rpcErrors";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import {
  WORKSPACE_PROJECTS_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_DIFF_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_DETAIL_KEY,
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
  WORKSPACE_AGENT_MEMORY_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
  type WorkspaceDiffQuery,
  type WorkspaceFileChangesQuery,
  type WorkspaceListFilesQuery,
  type WorkspaceReadFileQuery,
  type WorkspaceDiff,
  type AgentMemoryQuery,
  type WorkspaceCatalogQuery,
  type WorkspaceSkillDetailQuery,
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
    key: WORKSPACE_DIFF_KEY,
    fetcher: async (params, signal) => {
      const { cwd, ...query } = requiredParams<WorkspaceDiffQuery>(WORKSPACE_DIFF_KEY, params);
      const resources = await workspace(cwd, signal);
      const diff = await resources.diff.get({ ...query, format: "rows" }, signal);
      return {
        baseline: diff.baseline,
        files: diff.files ?? [],
        truncated: diff.truncated,
      } satisfies WorkspaceDiff;
    },
  });
  ctx.contribute(DATA_PROVIDER, {
    key: WORKSPACE_SKILLS_KEY,
    fetcher: async (params, signal) => {
      const query = requiredParams<WorkspaceCatalogQuery>(WORKSPACE_SKILLS_KEY, params);
      const resources = await workspace(query.cwd, signal);
      const catalog = await resources.skills.listDiscovered(signal);
      return {
        skills: catalog.skills.map((s) => ({
          name: s.name,
          description: s.description ?? "",
          scope: s.scope,
          installation: s.installation,
        })),
        diagnostics: catalog.diagnostics,
      };
    },
  });
  ctx.contribute(DATA_PROVIDER, {
    key: WORKSPACE_SKILL_DETAIL_KEY,
    fetcher: async (params, signal) => {
      const query = requiredParams<WorkspaceSkillDetailQuery>(WORKSPACE_SKILL_DETAIL_KEY, params);
      const resources = await workspace(query.cwd, signal);
      const detail = await resources.skills.getDiscovered(query.name, signal);
      return { ...detail, description: detail.description ?? "" };
    },
  });
  ctx.contribute(DATA_PROVIDER, {
    key: WORKSPACE_MANAGED_SKILLS_KEY,
    fetcher: async (_params, signal) =>
      (await pageData(runtimeClient().skills.listLibrary(signal)).catch(emptyListIfUngated)).map(
        (s) => ({
          name: s.name,
          description: s.description ?? "",
          lifecycle: s.lifecycle,
        }),
      ),
  });
  ctx.contribute(DATA_PROVIDER, {
    key: WORKSPACE_SKILL_PROPOSALS_KEY,
    fetcher: async (params, signal) => {
      const query = requiredParams<WorkspaceCatalogQuery>(WORKSPACE_SKILL_PROPOSALS_KEY, params);
      const resources = await workspace(query.cwd, signal);
      return (await pageData(resources.skills.listProposals(signal)).catch(emptyListIfUngated)).map(
        (p) => ({
          workspace: resources.ref.path,
          name: p.name,
          revision: p.revision,
          scope: p.scope,
          description: p.description,
          instructions: p.instructions,
          origin: p.origin ?? "mined",
          revises: p.revises === true,
          sourceSession: p.sourceSession ?? "",
        }),
      );
    },
  });
  ctx.contribute(DATA_PROVIDER, {
    key: WORKSPACE_AGENT_MEMORY_KEY,
    fetcher: async (params, signal) => {
      const q = requiredParams<AgentMemoryQuery>(WORKSPACE_AGENT_MEMORY_KEY, params);
      if (!runtimeCapability("agentMemory")) return [];
      const result =
        q.scope === "user"
          ? await runtimeClient().agentMemory.list({ scope: "user" }, signal)
          : await workspace(q.cwd, signal).then((resources) => resources.agentMemory.list(signal));
      return result.items.map((m) => ({
        id: m.id,
        scope: m.scope,
        content: m.content,
        origin: m.origin,
        status: m.status,
        pinned: m.pinned,
        sessionId: m.sessionId ?? "",
        day: m.day ?? "",
        createdAt: m.createdAt,
        updatedAt: m.updatedAt,
      }));
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
