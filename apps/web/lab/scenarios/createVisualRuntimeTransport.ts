import { createMemoryTransport } from "@flame/runtime-client/testing";
import { type Transport, type TransportRequest } from "@flame/runtime-client";
import {
  type ApprovalModeResult,
  NOTIFICATIONS_RUNTIME_EVENT,
  type RuntimeSubscribeRequest,
  type ResolveWorkspaceRequest,
  type WorkspaceInfo,
  type MCPServer,
  type MCPServerID,
  type Diff,
  type GetDiffRequest,
  type CallPluginRequest,
  type ListModelsRequest,
  type ServerCapabilities,
  type SetApprovalModeRequest,
  type SkillDetail,
  type SkillDetailRequest,
  type SkillProposalRef,
  PROBLEM_CODES,
} from "@flame/runtime-client/wire";
import {
  VISUAL_MEMORY_PAGE,
  VISUAL_USER_MEMORY_PAGE,
  VISUAL_SCHEDULES_PAGE,
  VISUAL_USAGE,
  VISUAL_USAGE_REPORTS,
  VISUAL_PACKAGES,
  VISUAL_PLUGIN_PROVIDERS,
} from "./componentSnapshots";
import { stringifyJSON } from "@frame/sdk/json";
import {
  type MethodInputs as MemoryInputs,
  type MethodOutputs as MemoryOutputs,
} from "../../../../plugins/memory/package/io.github.tangerg.flame/contracts/methods.generated";
import { type MethodInputs as UsageReportInputs } from "../../../../plugins/usagereports/component/methods.generated";
import { VISUAL_MODEL_FACTS, type VisualModelFacts } from "./runtimeSnapshots";
import { VISUAL_WORKSPACE_PATH } from "./agentFixtureFacts";
import { VISUAL_DIRECTORY_PATH } from "./workspaceFixtureStates";
import discovery from "@flame/runtime-client/testing/samples/method.discover.resp.json";
import { visualTrajectory } from "./trajectorySnapshot";
import { VISUAL_HOOKS } from "./hookSnapshot";
import { type RuntimeAgentSessionSnapshot } from "./agentSessionSnapshots";
import {
  VISUAL_SKILL_DOCUMENTS,
  VISUAL_SKILL_LIBRARY,
  VISUAL_SKILL_PROPOSALS,
  visualSkillDiscovery,
} from "./skillSnapshots";

export const VISUAL_REMOVE_SCHEDULE_EVENT = "flame:visual-remove-schedule";

type VisualMemoryPages = Readonly<
  Record<MemoryInputs["flame.memory/list.v3"]["scope"], MemoryOutputs["flame.memory/list.v3"]>
>;

function pluginCallResult(call: CallPluginRequest, memoryPages: VisualMemoryPages) {
  switch (call.method) {
    case "flame.memory/list.v3": {
      const input = JSON.parse(call.input ?? "") as Partial<
        MemoryInputs["flame.memory/list.v3"]
      > | null;
      const scope = input?.scope;
      if (scope === "project") return { output: JSON.stringify(memoryPages.project) };
      if (scope === "user") return { output: JSON.stringify(memoryPages.user) };
      throw new Error(`Visual memory has no page for scope "${String(scope)}"`);
    }
    // stringifyJSON keeps bigint tokens exact and an absent cost absent.
    case "flame.usage-reports/list.v2": {
      const reports = VISUAL_USAGE_REPORTS.map(({ id, period, observedAt, since }) => ({
        id,
        period,
        observedAt,
        since,
      }));
      return { output: stringifyJSON({ reports }) };
    }
    case "flame.usage-reports/get.v1": {
      const input = JSON.parse(call.input ?? "") as Partial<
        UsageReportInputs["flame.usage-reports/get.v1"]
      > | null;
      const id = input?.id;
      const report = VISUAL_USAGE_REPORTS.find((candidate) => candidate.id === id);
      if (!report) throw new Error(`Visual usage reports has no report "${String(id)}"`);
      return { output: stringifyJSON(report) };
    }
    default:
      throw new Error(`Visual plugin has no response for "${call.method}"`);
  }
}

function resultFor(
  request: TransportRequest,
  snapshot: RuntimeAgentSessionSnapshot,
  capabilities: ServerCapabilities,
  approval: ApprovalModeResult,
  models: VisualModelFacts,
  mcpServers: MCPServer[],
  skillDocuments: SkillDetail[],
  memoryPages: VisualMemoryPages,
  readDiff: (signal?: AbortSignal, request?: GetDiffRequest) => Promise<Diff>,
  signal?: AbortSignal,
): unknown {
  switch (request.method) {
    case "workspaces.resolve": {
      const ref = (request.params as ResolveWorkspaceRequest | undefined)?.ref;
      if (ref !== undefined && ref.path !== VISUAL_WORKSPACE_PATH)
        throw new Error(`Visual Runtime has no recorded workspace "${ref.path}"`);
      return {
        ref: { path: VISUAL_WORKSPACE_PATH },
        projectRoot: VISUAL_WORKSPACE_PATH,
        availability: "available",
      } satisfies WorkspaceInfo;
    }
    case "workspace.diff.get":
      return readDiff(signal, request.params as GetDiffRequest);
    case "runtime.discover":
      return { ...discovery, capabilities };
    case "sessions.trajectory":
      return { data: visualTrajectory(snapshot) };
    case "usage.summary":
      return VISUAL_USAGE;
    case "skills.discovered.list":
      return visualSkillDiscovery(skillDocuments);
    case "skills.discovered.get": {
      const { name } = request.params as SkillDetailRequest;
      const document = skillDocuments.find((item) => item.name === name);
      if (!document) throw new Error(`Visual Skills has no document named "${name}"`);
      return document;
    }
    case "skills.library.list":
      return VISUAL_SKILL_LIBRARY;
    case "providers.list":
      return { data: models.providers };
    case "models.list": {
      const { provider } = request.params as ListModelsRequest;
      return { data: models.models.filter((model) => model.provider === provider) };
    }
    case "models.getUtilityRole":
      return models.utility;
    case "models.getEmbeddingRole":
      return models.embedding;
    case "mcp.servers.list":
      return { data: mcpServers };
    case "mcp.tools.list": {
      const { server } = request.params as { server: MCPServerID };
      return {
        data: [
          {
            server,
            name: "read_workspace_files",
            description: "Read files from the current workspace without changing them.",
            modelName: "read_workspace_files",
            nameConflicts: [],
          },
        ],
      };
    }
    case "mcp.tools.exposure":
      return { server: (request.params as { server: MCPServerID }).server, disabledTools: [] };
    case "plugins.list":
      return { data: VISUAL_PACKAGES };
    case "plugins.listProviders":
      return { data: VISUAL_PLUGIN_PROVIDERS };
    case "approval.listRules":
      return { rules: [] };
    case "approval.getMode":
      return { ...approval };
    case "approval.setMode":
      approval.mode = (request.params as SetApprovalModeRequest).mode;
      return { ...approval };
    case "hooks.list":
      return structuredClone(VISUAL_HOOKS);
    case "runtime.subscribe":
      return {};
    case "plugins.call":
      return pluginCallResult(request.params as CallPluginRequest, memoryPages);
    default:
      throw new Error(`Visual Runtime has no response for "${request.method}"`);
  }
}

export function createVisualRuntimeTransport(
  snapshot: RuntimeAgentSessionSnapshot,
  capabilities: ServerCapabilities,
  readDiff: (signal?: AbortSignal, request?: GetDiffRequest) => Promise<Diff>,
  configuration: {
    acceptSkillProposals?: boolean;
    modelFacts?: VisualModelFacts;
    mcpServers?: MCPServer[];
    skillDocuments?: readonly SkillDetail[];
    memoryPages?: VisualMemoryPages;
  } = {},
): Transport & { removeSchedule(id: string): void; notifyDirectoryChange(): void } {
  const transport = createMemoryTransport();
  const skillDocuments = structuredClone([
    ...(configuration.skillDocuments ?? VISUAL_SKILL_DOCUMENTS),
  ]);
  const memoryPages = structuredClone(
    configuration.memoryPages ?? { project: VISUAL_MEMORY_PAGE, user: VISUAL_USER_MEMORY_PAGE },
  );
  const skillProposals = structuredClone(VISUAL_SKILL_PROPOSALS);
  const models = structuredClone(configuration.modelFacts ?? VISUAL_MODEL_FACTS);
  const mcpServers = structuredClone(configuration.mcpServers ?? []);
  const schedules = structuredClone(VISUAL_SCHEDULES_PAGE);
  const watches = new Map<
    TransportRequest["id"],
    { topics: RuntimeSubscribeRequest["topics"]; signal?: AbortSignal }
  >();
  let sequence = 0;
  const publish = (type: "plugins.dataChanged" | "files.changed") => {
    sequence += 1;
    const event =
      type === "files.changed"
        ? {
            type,
            sequence,
            workspace: { path: VISUAL_WORKSPACE_PATH },
            paths: [VISUAL_DIRECTORY_PATH],
          }
        : { type, sequence };
    for (const [rpcId, watch] of watches) {
      if (watch.signal?.aborted || !watch.topics.includes(type)) continue;
      transport.inject(
        { jsonrpc: "2.0", method: NOTIFICATIONS_RUNTIME_EVENT, params: { event } },
        undefined,
        rpcId,
      );
    }
  };
  const approval: ApprovalModeResult = {
    mode: "balanced",
    modes: [
      { mode: "safe", write: "prompt", exec: "prompt", network: "prompt" },
      { mode: "balanced", write: "pass", exec: "prompt", network: "pass" },
      { mode: "yolo", write: "pass", exec: "pass", network: "pass" },
    ],
  };
  return {
    recv: transport.recv,
    close() {
      watches.clear();
      return transport.close();
    },
    notifyDirectoryChange() {
      publish("files.changed");
    },
    removeSchedule(id) {
      const index = schedules.rows.findIndex((row) => row.id === id);
      if (index < 0) throw new Error(`visual schedules has no record named "${id}"`);
      schedules.rows.splice(index, 1);
      publish("plugins.dataChanged");
    },
    async send(request, signal, options) {
      signal?.throwIfAborted();
      if (
        request.method === "models.setUtilityRole" ||
        request.method === "models.setEmbeddingRole" ||
        request.method === "mcp.servers.test" ||
        request.method === "mcp.servers.create" ||
        request.method === "mcp.servers.update" ||
        request.method === "mcp.servers.delete" ||
        request.method === "mcp.servers.reconnect" ||
        request.method === "mcp.tools.setExposure"
      ) {
        await transport.send(request, signal, options);
        transport.inject({
          jsonrpc: "2.0",
          id: request.id,
          error: {
            code: PROBLEM_CODES.method_not_found,
            message: "method_not_found",
            data: {
              type: "method_not_found",
              detail: `Visual Runtime has no response for "${request.method}"`,
            },
          },
        });
        return;
      }
      let result: unknown;
      if (
        request.method === "plugins.call" &&
        (request.params as CallPluginRequest).method === "flame.schedules/list.v2"
      ) {
        result = { output: JSON.stringify(schedules) };
      } else if (request.method === "skills.proposals.list") {
        result = structuredClone(skillProposals);
      } else if (
        configuration.acceptSkillProposals &&
        (request.method === "skills.proposals.approve" ||
          request.method === "skills.proposals.reject")
      ) {
        const captured = structuredClone(request.params as SkillProposalRef);
        if (captured.workspace.path !== VISUAL_WORKSPACE_PATH) {
          throw new Error("Visual Skills cannot decide a proposal in another workspace");
        }
        const index = skillProposals.data.findIndex(
          (proposal) =>
            proposal.scope === captured.scope &&
            proposal.name === captured.name &&
            proposal.revision === captured.revision,
        );
        if (index < 0) throw new Error("Visual Skills has no matching proposal revision");
        skillProposals.data.splice(index, 1);
        result = {};
      } else {
        result = await resultFor(
          request,
          snapshot,
          capabilities,
          approval,
          models,
          mcpServers,
          skillDocuments,
          memoryPages,
          readDiff,
          signal,
        );
      }
      await transport.send(request, signal, options);
      if (request.method === "runtime.subscribe" && !signal?.aborted) {
        watches.set(request.id, {
          topics: [...(request.params as RuntimeSubscribeRequest).topics],
          signal,
        });
        signal?.addEventListener("abort", () => watches.delete(request.id), { once: true });
      }
      transport.inject({ jsonrpc: "2.0", id: request.id, result });
    },
  };
}
