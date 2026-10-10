import { createMemoryTransport } from "@flame/runtime-contract/client/transports/memory";
import type { Transport, TransportRequest } from "@flame/runtime-contract/client/transport";
import { VISUAL_MODELS, VISUAL_PROVIDERS } from "./runtimeSnapshots";

function resultFor(request: TransportRequest): unknown {
  switch (request.method) {
    case "providers.list":
      return { data: VISUAL_PROVIDERS };
    case "models.list": {
      const { provider } = request.params as { provider?: string };
      return { data: VISUAL_MODELS.filter((model) => !provider || model.provider === provider) };
    }
    case "models.getUtilityRole":
      return { provider: VISUAL_MODELS[0]!.provider, model: VISUAL_MODELS[0]!.id };
    case "models.getEmbeddingRole":
      return {};
    case "mcp.servers.list":
    case "plugins.list":
      return { data: [] };
    case "hooks.list":
      return { hooks: [], projectTrusted: false };
    case "runtime.subscribe":
      return {};
    default:
      throw new Error(`Visual Runtime has no response for "${request.method}"`);
  }
}

export function createVisualRuntimeTransport(): Transport {
  const transport = createMemoryTransport();
  return {
    recv: transport.recv,
    close: transport.close,
    async send(request, signal, options) {
      signal?.throwIfAborted();
      const result = resultFor(request);
      await transport.send(request, signal, options);
      transport.inject({ jsonrpc: "2.0", id: request.id, result });
    },
  };
}
