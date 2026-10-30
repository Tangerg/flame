import { createMemoryTransport } from "@flame/runtime-client/testing";
import { type Transport, type TransportRequest } from "@flame/runtime-client";
import {
  type ApprovalModeResult,
  type MCPServer,
  type MCPServerID,
  type Diff,
  type GetDiffRequest,
  type CallPluginRequest,
  type ListModelsRequest,
  type ServerCapabilities,
  type SetApprovalModeRequest,
  type SkillDetailRequest,
  type SkillProposalRef,
  PROBLEM_CODES,
} from "@flame/runtime-client/wire";
import {
  VISUAL_MEMORY_PAGE,
  VISUAL_SCHEDULES_PAGE,
  VISUAL_USAGE,
  VISUAL_PACKAGES,
  VISUAL_PLUGIN_PROVIDERS,
} from "./componentSnapshots";
import { VISUAL_MODEL_FACTS, type VisualModelFacts } from "./runtimeSnapshots";
import { VISUAL_WORKSPACE_PATH } from "./agentFixtureFacts";
import discovery from "@flame/runtime-client/testing/samples/method.discover.resp.json";
import { visualTrajectory } from "./trajectorySnapshot";
import { VISUAL_HOOKS } from "./hookSnapshot";
import { type RuntimeAgentSessionSnapshot } from "./agentSessionSnapshots";
import {
  VISUAL_SKILL_DISCOVERY,
  VISUAL_SKILL_DOCUMENTS,
  VISUAL_SKILL_LIBRARY,
  VISUAL_SKILL_PROPOSALS,
} from "./skillSnapshots";
function resultFor(
  request: TransportRequest,
  snapshot: RuntimeAgentSessionSnapshot,
  capabilities: ServerCapabilities,
  approval: ApprovalModeResult,
  models: VisualModelFacts,
  mcpServers: MCPServer[],
  readDiff: (signal?: AbortSignal, request?: GetDiffRequest) => Promise<Diff>,
  signal?: AbortSignal,
): unknown {
  switch (request.method) {
    case "workspace.diff.get":
      return readDiff(signal, request.params as GetDiffRequest);
    case "runtime.discover":
      return { ...discovery, capabilities };
    case "sessions.trajectory":
      return { data: visualTrajectory(snapshot) };
    case "usage.summary":
      return VISUAL_USAGE;
    case "skills.discovered.list":
      return VISUAL_SKILL_DISCOVERY;
    case "skills.discovered.get": {
      const { name } = request.params as SkillDetailRequest;
      const document = VISUAL_SKILL_DOCUMENTS.find((item) => item.name === name);
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
    case "plugins.call": {
      const call = request.params as CallPluginRequest;
      if (call.method === "flame.memory/list.v2") {
        return { output: JSON.stringify(VISUAL_MEMORY_PAGE) };
      }
      if (call.method === "flame.schedules/list.v1") {
        return { output: JSON.stringify(VISUAL_SCHEDULES_PAGE) };
      }
      throw new Error(`Visual plugin has no response for "${call.method}"`);
    }
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
  } = {},
): Transport {
  const transport = createMemoryTransport();
  const skillProposals = structuredClone(VISUAL_SKILL_PROPOSALS);
  const models = structuredClone(configuration.modelFacts ?? VISUAL_MODEL_FACTS);
  const mcpServers = structuredClone(configuration.mcpServers ?? []);
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
    close: transport.close,
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
      if (request.method === "skills.proposals.list") {
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
          readDiff,
          signal,
        );
      }
      await transport.send(request, signal, options);
      transport.inject({ jsonrpc: "2.0", id: request.id, result });
    },
  };
}
