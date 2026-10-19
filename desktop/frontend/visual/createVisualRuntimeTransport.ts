import { createMemoryTransport } from "@flame/runtime-contract/client/transports/memory";
import type { Transport, TransportRequest } from "@flame/runtime-contract/client/transport";
import type {
  ApprovalModeResult,
  Diff,
  GetDiffRequest,
  CallPluginRequest,
  ListModelsRequest,
  ServerCapabilities,
  SetApprovalModeRequest,
  SkillDetailRequest,
  SkillProposalRef,
} from "@flame/runtime-contract/wire";
import {
  VISUAL_MEMORY_PAGE,
  VISUAL_SCHEDULES_PAGE,
  VISUAL_USAGE,
  VISUAL_PACKAGES,
  VISUAL_PLUGIN_PROVIDERS,
} from "./componentSnapshots";
import { VISUAL_MODELS, VISUAL_PROVIDERS } from "./runtimeSnapshots";
import { VISUAL_WORKSPACE_PATH } from "./agentFixtureFacts";

import discovery from "@flame/runtime-contract/samples/method.discover.resp.json";
import { visualTrajectory } from "./trajectorySnapshot";
import type { RuntimeAgentSessionSnapshot } from "./agentSessionSnapshots";
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
      return { data: VISUAL_PROVIDERS };
    case "models.list": {
      const { provider } = request.params as ListModelsRequest;
      return { data: VISUAL_MODELS.filter((model) => model.provider === provider) };
    }
    case "models.getUtilityRole":
      return { provider: VISUAL_MODELS[0]!.provider, model: VISUAL_MODELS[0]!.id };
    case "models.getEmbeddingRole":
      return {};
    case "mcp.servers.list":
      return { data: [] };
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
      return { hooks: [], projectTrusted: false };
    case "runtime.subscribe":
      return {};
    case "plugins.call": {
      const call = request.params as CallPluginRequest;
      if (call.method === "flame.memory/list.v1") {
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
  acceptSkillProposals = false,
): Transport {
  const transport = createMemoryTransport();
  const skillProposals = structuredClone(VISUAL_SKILL_PROPOSALS);
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
      let result: unknown;
      if (request.method === "skills.proposals.list") {
        result = structuredClone(skillProposals);
      } else if (
        acceptSkillProposals &&
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
        result = await resultFor(request, snapshot, capabilities, approval, readDiff, signal);
      }
      await transport.send(request, signal, options);
      transport.inject({ jsonrpc: "2.0", id: request.id, result });
    },
  };
}
