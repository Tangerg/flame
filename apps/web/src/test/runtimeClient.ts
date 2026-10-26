import {
  createFlameClient,
  type FlameClientOptions,
  type Transport,
  type MutationPromise,
  type MutationAttemptOptions,
} from "@flame/runtime-client";
import { createMemoryTransport } from "@flame/runtime-client/testing";

export type RuntimeCallOptions = NonNullable<Parameters<Transport["send"]>[2]> & {
  signal?: AbortSignal;
};
export type RuntimeCall = (
  method: string,
  params: unknown,
  options: RuntimeCallOptions,
) => Promise<unknown>;

export function createRuntimeTestClient(call: RuntimeCall, options?: FlameClientOptions) {
  const transport = createMemoryTransport();
  transport.send = async (request, signal, metadata) => {
    const result = await call(request.method, request.params, { ...metadata, signal });
    transport.inject({ jsonrpc: "2.0", id: request.id, result } as Parameters<
      typeof transport.inject
    >[0]);
  };
  return createFlameClient(transport, options);
}

// One scripted attempt lets UI tests control settlement; transport tests own SDK replay policy.
export function mutationResult<T>(
  execute: (key: string, options: MutationAttemptOptions) => Promise<T>,
  key: string = crypto.randomUUID(),
  options: MutationAttemptOptions = {},
): MutationPromise<T> {
  return Object.assign(
    Promise.resolve().then(() => execute(key, options)),
    {
      idempotencyKey: key,
      retry: (next: MutationAttemptOptions = options) => mutationResult(execute, key, next),
    },
  );
}
