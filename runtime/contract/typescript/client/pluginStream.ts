import type { RpcClient } from "./client";
import type { RpcId } from "./types";
import { OperationFailure } from "./errors";
import { MutationLookupError, mutationSettlementIsUnknown } from "./mutation";
import { MutationJournalError } from "./durableMutationJournal";
import type { MutationJournal, PluginCommandInput, PluginCommandBinding } from "./mutationJournal";
import type { WireCallPath } from "./wireCallPath";
import { pluginDeliveryMethod, pluginOperationKind } from "./request";
import { parseReviewedJSON } from "./json";
import {
  NOTIFICATIONS_PLUGIN_STREAM,
  type PluginStreamEvent,
  type CallPluginRequest,
} from "@flame/runtime-contract/wire";

function observePluginStream(
  client: RpcClient,
  consume: (chunk: unknown, signal: AbortSignal) => void | Promise<void>,
  parent?: AbortSignal,
  failure: (cause: unknown) => unknown = (cause) => cause,
) {
  const controller = new AbortController();
  const signal = AbortSignal.any([
    controller.signal,
    client.closedSignal,
    ...(parent ? [parent] : []),
  ]);
  const done = Promise.withResolvers<unknown>();
  let requestId: RpcId | undefined;
  let ended = false;
  let consuming = false;
  let unsubscribe: () => void = () => undefined;
  const cleanup = () => {
    signal.removeEventListener("abort", abort);
    unsubscribe();
    controller.abort();
  };
  const fail = (cause: unknown) => {
    if (ended) return;
    ended = true;
    done.reject(failure(cause));
    cleanup();
  };
  const abort = () =>
    fail(signal.reason ?? new DOMException("stream observation ended", "AbortError"));
  const events = client.subscribe(NOTIFICATIONS_PLUGIN_STREAM, {
    async next(event: PluginStreamEvent, owner: RpcId) {
      if (ended || owner !== requestId) return;
      if (consuming) {
        fail(new Error("concurrent plugin stream delivery"));
        return;
      }
      if (event.kind === "chunk") {
        consuming = true;
        try {
          await consume(event.value, signal);
        } catch (error) {
          fail(error);
        } finally {
          consuming = false;
        }
        return;
      }
      if (event.kind === "failure") {
        ended = true;
        done.reject(new OperationFailure(event.failure!.code, event.failure!.message));
        cleanup();
      } else {
        ended = true;
        done.resolve(event.value);
        cleanup();
      }
    },
    error(error, owner) {
      if (owner === undefined || owner === requestId) fail(error);
    },
  });
  const down = client.onStreamEnd((event) => {
    if (event.method === "plugins.stream" && event.requestRpcId === requestId) {
      fail(event.error ?? new Error("plugin stream ended without final evidence"));
    }
  });
  unsubscribe = () => {
    events();
    down();
  };
  signal.addEventListener("abort", abort, { once: true });
  if (signal.aborted) abort();
  return {
    result: done.promise,
    signal,
    bindRequest(id: RpcId) {
      if (requestId !== undefined) throw new Error("plugin stream is already bound to a request");
      requestId = id;
    },
    cleanup,
  };
}

export async function callPluginStream<T>(
  journal: MutationJournal,
  client: RpcClient,
  wire: Pick<WireCallPath, "perform" | "openMutation">,
  intent: string,
  input: PluginCommandInput,
  prepare: () => Promise<PluginCommandBinding>,
  consume: (chunk: unknown, signal: AbortSignal) => void | Promise<void>,
  decode: (output: unknown) => T,
  signal?: AbortSignal,
): Promise<T> {
  if (pluginDeliveryMethod(input) !== "plugins.stream" || pluginOperationKind(input) !== "command")
    throw new TypeError("Plugin stream requires a streaming command method");
  parseReviewedJSON(JSON.stringify(input.input));
  const { request, reservation } = await journal.capturePluginCommand(intent, input, prepare);
  return reservation.track(() =>
    wire.openMutation(
      "plugins.stream",
      request,
      async (key, attempt, prepared) => {
        const stream = observePluginStream(
          client,
          consume,
          attempt.signal,
          (cause) => new MutationLookupError(cause),
        );
        // The response can fail before a notification arrives; retain the same
        // observation rejection without allowing an orphan unhandled promise.
        const outcome = stream.result.then(
          (value) => ({ value }),
          (error: unknown) => ({ error }),
        );
        try {
          await wire.perform("plugins.stream", prepared, {
            signal: stream.signal,
            onRequestRpcId: stream.bindRequest,
            idempotencyKey: key,
            idempotencyNamespace: reservation.authorizeDispatch(),
          });
          const final = await outcome;
          if ("error" in final) throw final.error;
          try {
            return decode(final.value);
          } catch (error) {
            throw new MutationLookupError(error);
          }
        } catch (error) {
          if (
            error instanceof MutationJournalError ||
            error instanceof OperationFailure ||
            mutationSettlementIsUnknown(error)
          )
            throw error;
          throw new MutationLookupError(error);
        } finally {
          stream.cleanup();
        }
      },
      signal,
      reservation.idempotencyKey,
    ),
  );
}

export async function queryPluginStream<T>(
  client: RpcClient,
  wire: Pick<WireCallPath, "perform">,
  request: CallPluginRequest,
  consume: (chunk: unknown, signal: AbortSignal) => void | Promise<void>,
  decode: (output: unknown) => T,
  signal?: AbortSignal,
): Promise<T> {
  if (
    pluginDeliveryMethod(request) !== "plugins.stream" ||
    pluginOperationKind(request) !== "query"
  )
    throw new TypeError("Plugin query stream requires a streaming query method");
  const stream = observePluginStream(client, consume, signal);
  const outcome = stream.result.then(
    (value) => ({ value }),
    (error: unknown) => ({ error }),
  );
  try {
    await wire.perform("plugins.stream", structuredClone(request), {
      signal: stream.signal,
      onRequestRpcId: stream.bindRequest,
    });
    const final = await outcome;
    if ("error" in final) throw final.error;
    return decode(final.value);
  } finally {
    stream.cleanup();
  }
}
