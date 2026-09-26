import {
  MODEL_INVOCATIONS_KEY,
  type ModelInvocationQuery,
} from "@/plugins/builtin/agent/public/run";
import type { ApprovalRulesQuery } from "@/plugins/builtin/agent/public/approvalPolicy";
import { emptyListIfUngated } from "@/lib/rpcErrors";
import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import { AGENT_SESSIONS_KEY } from "@/plugins/builtin/agent/public/session";
import {} from "@/plugins/builtin/workspace/public/queries";
import { HOOKS_KEY, type HooksQuery } from "@/plugins/builtin/settings/hooks/public/queries";
import {
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  PROVIDERS_KEY,
  ProviderConfiguration,
  SelectableModel,
  UTILITY_ROLE_KEY,
} from "@/plugins/builtin/settings/providers/public/queries";
import type {
  WorkspaceDiffQuery,
  WorkspaceFileChangesQuery,
  WorkspaceListFilesQuery,
  WorkspaceReadFileQuery,
  WorkspaceDiff,
  AgentMemoryQuery,
  WorkspaceCatalogQuery,
} from "@/plugins/builtin/workspace/public/queries";
import {
  WORKSPACE_DIFF_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_PROJECTS_KEY,
  WORKSPACE_READ_FILE_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_DETAIL_KEY,
  type WorkspaceSkillDetailQuery,
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
  WORKSPACE_AGENT_MEMORY_KEY,
} from "@/plugins/builtin/workspace/public/queries";
import type { DataProviderSpec, Contributor } from "@/plugins/sdk";
import { getContainer } from "@/main/container";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { asSessionId, type FlameClient } from "@flame/runtime-contract/client";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import {
  toWorkspaceFileChangeSummary,
  toWorkspaceProjectSummary,
  toAgentSessionSummary,
} from "./runtimeReadModelAdapters";

function optionalParams<P>(params: unknown): P | undefined {
  return params as P | undefined;
}

function requiredParams<P>(key: string, params: unknown): P {
  const value = optionalParams<P>(params);
  if (value === undefined) throw new Error(`Data provider "${key}" requires parameters`);
  return value;
}

function pageData<T>(request: Promise<{ data: T[] }>): Promise<T[]> {
  return request.then((page) => page.data);
}

class RuntimeProviderRead {
  private constructor(
    readonly client: FlameClient,
    readonly signal: AbortSignal | undefined,
  ) {}

  static begin(signal: AbortSignal | undefined): RuntimeProviderRead {
    return new RuntimeProviderRead(getContainer().client(), signal);
  }

  workspace(cwd?: string) {
    return this.client.workspaces.open(cwd ? { path: cwd } : undefined, this.signal);
  }
}

interface RuntimeProviderSpec {
  readonly key: string;
  readonly fetcher: (read: RuntimeProviderRead, params?: unknown) => Promise<unknown>;
}

export function registerDefaultDataProviders(ctx: Contributor): void {
  const contribute = ({ key, fetcher }: RuntimeProviderSpec): void => {
    const provider: DataProviderSpec = {
      key,
      fetcher: (params, signal) => fetcher(RuntimeProviderRead.begin(signal), params),
    };
    ctx.contribute(DATA_PROVIDER, provider);
  };

  contribute({
    key: MODEL_INVOCATIONS_KEY,
    fetcher: (read, params) =>
      read.client.modelInvocations.list(
        requiredParams<ModelInvocationQuery>(MODEL_INVOCATIONS_KEY, params),
        read.signal,
      ),
  });
  contribute({
    key: AGENT_SESSIONS_KEY,
    fetcher: async (read) =>
      (await read.client.sessions.list(undefined, read.signal).autoPagingToArray()).map(
        toAgentSessionSummary,
      ),
  });
  contribute({
    key: WORKSPACE_PROJECTS_KEY,
    fetcher: async (read) =>
      (await pageData(read.client.workspaces.list(read.signal))).map(toWorkspaceProjectSummary),
  });
  contribute({
    key: WORKSPACE_FILES_CHANGED_KEY,
    fetcher: async (read, params) => {
      const resources = await read.workspace(
        optionalParams<WorkspaceFileChangesQuery>(params)?.cwd,
      );
      return (await pageData(resources.changes.list(read.signal))).map(
        toWorkspaceFileChangeSummary,
      );
    },
  });
  contribute({
    key: WORKSPACE_DIFF_KEY,
    fetcher: async (read, params) => {
      const { cwd, ...query } = requiredParams<WorkspaceDiffQuery>(WORKSPACE_DIFF_KEY, params);
      const resources = await read.workspace(cwd);
      const diff = await resources.diff.get({ ...query, format: "rows" }, read.signal);
      return {
        baseline: diff.baseline,
        files: diff.files ?? [],
        truncated: diff.truncated,
      } satisfies WorkspaceDiff;
    },
  });
  contribute({
    key: WORKSPACE_SKILLS_KEY,
    fetcher: async (read, params) => {
      const query = requiredParams<WorkspaceCatalogQuery>(WORKSPACE_SKILLS_KEY, params);
      const resources = await read.workspace(query.cwd);
      const catalog = await resources.skills.listDiscovered(read.signal);
      return {
        skills: catalog.skills.map((s) => ({
          name: s.name,
          description: s.description ?? "",
          scope: s.scope,
        })),
        diagnostics: catalog.diagnostics,
      };
    },
  });
  contribute({
    key: WORKSPACE_SKILL_DETAIL_KEY,
    fetcher: async (read, params) => {
      const query = requiredParams<WorkspaceSkillDetailQuery>(WORKSPACE_SKILL_DETAIL_KEY, params);
      const resources = await read.workspace(query.cwd);
      const detail = await resources.skills.getDiscovered(query.name, read.signal);
      return { ...detail, description: detail.description ?? "" };
    },
  });
  contribute({
    key: WORKSPACE_MANAGED_SKILLS_KEY,
    fetcher: async (read) =>
      (await pageData(read.client.skills.listLibrary(read.signal)).catch(emptyListIfUngated)).map(
        (s) => ({
          name: s.name,
          description: s.description ?? "",
          lifecycle: s.lifecycle,
        }),
      ),
  });
  contribute({
    key: WORKSPACE_SKILL_PROPOSALS_KEY,
    fetcher: async (read, params) => {
      const query = requiredParams<WorkspaceCatalogQuery>(WORKSPACE_SKILL_PROPOSALS_KEY, params);
      const resources = await read.workspace(query.cwd);
      return (
        await pageData(resources.skills.listProposals(read.signal)).catch(emptyListIfUngated)
      ).map((p) => ({
        workspace: resources.ref.path,
        name: p.name,
        revision: p.revision,
        scope: p.scope,
        description: p.description,
        instructions: p.instructions,
        origin: p.origin ?? "mined",
        revises: p.revises === true,
        sourceSession: p.sourceSession ?? "",
      }));
    },
  });
  contribute({
    key: WORKSPACE_AGENT_MEMORY_KEY,
    fetcher: async (read, params) => {
      const q = requiredParams<AgentMemoryQuery>(WORKSPACE_AGENT_MEMORY_KEY, params);
      if (!runtimeCapability("agentMemory")) return [];
      const result =
        q.scope === "user"
          ? await read.client.agentMemory.list({ scope: "user" }, read.signal)
          : await read
              .workspace(q.cwd)
              .then((resources) => resources.agentMemory.list(read.signal));
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
  contribute({
    key: MODELS_KEY,
    fetcher: async (read) => {
      const configured = (await pageData(read.client.providers.list(read.signal))).filter(
        (provider) => provider.configured,
      );
      const lists = await Promise.all(
        configured.map((provider) => pageData(read.client.models.list(provider.id, read.signal))),
      );
      return lists.flat().map(
        (m) =>
          new SelectableModel({
            id: m.id,
            provider: m.provider,
            label: m.displayName ?? m.id,
            tokenLimits: m.tokenLimits,
            knowledgeCutoff: m.knowledgeCutoff,
            deprecated: m.deprecated,
            reasoning: m.capabilities?.reasoning,
            reasoningLevels: m.capabilities?.reasoningLevels,
            reasoningDefaultLevel: m.capabilities?.reasoningDefaultLevel,
            inputModalities: m.capabilities?.inputModalities,
            outputModalities: m.capabilities?.outputModalities,
            toolUse: m.capabilities?.toolUse,
            structuredOutput: m.capabilities?.structuredOutput,
          }),
      );
    },
  });
  contribute({
    key: PROVIDERS_KEY,
    fetcher: async (read) =>
      (await pageData(read.client.providers.list(read.signal))).map((provider) =>
        ProviderConfiguration.restore(provider),
      ),
  });
  contribute({
    key: APPROVAL_MODE_KEY,
    fetcher: async (read) => (await read.client.approval.getMode(read.signal)).mode,
  });
  contribute({
    key: UTILITY_ROLE_KEY,
    fetcher: (read) => read.client.models.getUtilityRole(read.signal),
  });
  contribute({
    key: EMBEDDING_ROLE_KEY,
    fetcher: (read) => read.client.models.getEmbeddingRole(read.signal),
  });
  contribute({
    key: APPROVAL_RULES_KEY,
    fetcher: async (read, params) => {
      const query = requiredParams<ApprovalRulesQuery>(APPROVAL_RULES_KEY, params);
      return (await read.client.approval.listRules(asSessionId(query.sessionId), read.signal))
        .rules;
    },
  });
  contribute({
    key: HOOKS_KEY,
    fetcher: async (read, params) =>
      (await read.workspace(optionalParams<HooksQuery>(params)?.cwd)).hooks.list(read.signal),
  });
  contribute({
    key: WORKSPACE_LIST_FILES_KEY,
    fetcher: async (read, params) => {
      const { cwd, ...query } = requiredParams<WorkspaceListFilesQuery>(
        WORKSPACE_LIST_FILES_KEY,
        params,
      );
      const resources = await read.workspace(cwd);
      return (await resources.files.list(query, read.signal).autoPagingToArray()).map((e) => ({
        path: e.path,
        name: e.name,
        type: e.type,
        sizeBytes: e.sizeBytes,
      }));
    },
  });
  contribute({
    key: WORKSPACE_READ_FILE_KEY,
    fetcher: async (read, params) => {
      const { cwd, ...query } = requiredParams<WorkspaceReadFileQuery>(
        WORKSPACE_READ_FILE_KEY,
        params,
      );
      const r = await (await read.workspace(cwd)).files.read(query, read.signal);
      return {
        content: r.content,
        startLine: r.startLine ?? 1,
        totalLines: r.totalLines,
        truncated: r.truncated,
      };
    },
  });
}
