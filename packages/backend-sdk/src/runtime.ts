import {
  createFlameClient,
  createHttpTransport,
  PROTOCOL_VERSION,
  type FlameClient,
  type ServerCapabilities,
} from "@flame/runtime-client";
import { HTTP_ENDPOINTS } from "@flame/runtime-client/wire";
import { callbackFetch, callbackRef } from "@qimu/node/callback";
import { freezeJSON } from "@qimu/node/contract";
import type { CallContext, Invocation, SetupContext } from "@qimu/node";

export function runtime(context: CallContext): Promise<FlameClient>;
export function runtime(
  context: SetupContext,
  invocation: Invocation,
): Promise<FlameClient>;
export async function runtime(
  context: CallContext | SetupContext,
  invocation?: Invocation,
): Promise<FlameClient> {
  if (!context.callback)
    throw new Error("plugin: calling-owner callback is required");
  const captured = callbackRef(context.callback);
  const owner =
    invocation ?? ("invocation" in context ? context.invocation : undefined);
  const capacity = Number(captured.maxResponseBytes);
  if (!Number.isSafeInteger(capacity))
    throw new Error("plugin: callback message capacity is not representable");
  let client: FlameClient | undefined;
  context.defer(async () => {
    await client?.close();
  });
  let capabilities: ServerCapabilities | undefined;
  client = createFlameClient(
    createHttpTransport({
      baseUrl: captured.endpoint,
      fetch: callbackFetch(captured, owner, HTTP_ENDPOINTS.rpc.path),
      maximumEventStreamFrameCharacters: capacity,
      maximumEventStreamFrameBytes: capacity,
    }),
    {
      requestMeta: () => ({ protocolVersion: PROTOCOL_VERSION }),
      capabilities: () => capabilities,
    },
  );
  const discovery = await client.runtime.discover(context.signal);
  if (discovery.protocolVersion !== PROTOCOL_VERSION)
    throw new Error(
      "plugin: host protocol differs from the published Flame client",
    );
  capabilities = freezeJSON(discovery.capabilities);
  return client;
}
