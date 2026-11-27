import { observeNotifications } from "./client";
import { isErrorType, OperationFailure } from "./errors";
import { MutationLookupError, mutationSettlementIsUnknown } from "./mutation";
import { MutationJournalError } from "./mutationJournalErrors";
import type {
  MutationReservation,
  PluginCommandInput,
  PluginCommandBinding,
} from "./mutationJournal";
import type { WireCallPath } from "./wireCallPath";
import { pluginMethod } from "./request";
import { parseJSON } from "@frame/sdk/json";
import { NOTIFICATIONS_PLUGIN_STREAM, type CallPluginRequest } from "@flame/runtime-client/wire";

function observePluginStream(
  consume: (chunk: unknown, signal: AbortSignal) => void | Promise<void>,
  parent?: AbortSignal,
  failure: (cause: unknown) => unknown = (cause) => cause,
) {
  const controller = new AbortController();
  const signal = parent ? AbortSignal.any([parent, controller.signal]) : controller.signal;
  const done = Promise.withResolvers<unknown>();
  let consuming = false;
  const abort = () => done.reject(failure(signal.reason));
  const cleanup = () => {
    signal.removeEventListener("abort", abort);
    controller.abort();
  };
  const fail = (cause: unknown) => {
    if (signal.aborted) return;
    done.reject(failure(cause));
    cleanup();
  };

  if (signal.aborted) abort();
  else signal.addEventListener("abort", abort, { once: true });
  return {
    result: done.promise,
    signal,
    cleanup,
    observe: () =>
      observeNotifications(NOTIFICATIONS_PLUGIN_STREAM, {
        async next(event) {
          if (consuming) {
            fail(new Error("concurrent plugin stream delivery"));
            return;
          }
          if (event.kind === "chunk") {
            consuming = true;
            try {
              await consume(parseJSON(event.value!), signal);
            } catch (error) {
              fail(error);
            } finally {
              consuming = false;
            }
            return;
          }
          if (event.kind === "failure") {
            done.reject(new OperationFailure(event.failure!.code, event.failure!.message));
          } else {
            try {
              done.resolve(parseJSON(event.value!));
            } catch (error) {
              fail(error);
            }
          }
          cleanup();
        },
        fail,
        end: (error) => fail(error ?? new Error("plugin stream ended without final evidence")),
      }),
  };
}

export async function callPluginStream<T>(
  wire: Pick<WireCallPath, "dispatchCommand" | "openMutation" | "capturePluginCommand">,
  intent: string,
  input: PluginCommandInput,
  prepare: () => Promise<PluginCommandBinding>,
  consume: (chunk: unknown, signal: AbortSignal) => void | Promise<void>,
  decode: (output: unknown) => T,
  signal?: AbortSignal,
): Promise<T> {
  const method = pluginMethod(input);
  if (method.delivery !== "plugins.stream" || method.kind !== "command")
    throw new TypeError("Plugin stream requires a streaming command method");
  parseJSON(input.input);
  const { request, reservation } = await wire.capturePluginCommand(intent, input, prepare, signal);
  return wire.openMutation(
    "plugins.stream",
    request,
    (prepared, original, attemptSignal) =>
      deliverPluginStream(wire, prepared, original, consume, decode, attemptSignal),
    {
      signal,
      original: { reserve: () => reservation },
    },
  );
}

export async function deliverPluginStream<T>(
  wire: Pick<WireCallPath, "dispatchCommand">,
  request: CallPluginRequest,
  reservation: MutationReservation,
  consume: (chunk: unknown, signal: AbortSignal) => void | Promise<void>,
  decode: (output: unknown) => T,
  signal?: AbortSignal,
): Promise<T> {
  const stream = observePluginStream(consume, signal, (cause) => new MutationLookupError(cause));
  // The response can fail before a notification arrives; retain the same
  // observation rejection without allowing an orphan unhandled promise.
  const outcome = stream.result.then(
    (value) => ({ value }),
    (error: unknown) => ({ error }),
  );
  try {
    await wire.dispatchCommand("plugins.stream", request, reservation, {
      signal: stream.signal,
      observe: stream.observe,
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
      isErrorType(error, "plugin_command_retired") ||
      error instanceof MutationJournalError ||
      error instanceof OperationFailure ||
      mutationSettlementIsUnknown(error)
    )
      throw error;
    throw new MutationLookupError(error);
  } finally {
    stream.cleanup();
  }
}

export async function queryPluginStream<T>(
  wire: Pick<WireCallPath, "callUnkeyed">,
  request: CallPluginRequest,
  consume: (chunk: unknown, signal: AbortSignal) => void | Promise<void>,
  decode: (output: unknown) => T,
  signal?: AbortSignal,
): Promise<T> {
  const method = pluginMethod(request.route);
  if (method.delivery !== "plugins.stream" || method.kind !== "query")
    throw new TypeError("Plugin query stream requires a streaming query method");
  parseJSON(request.input);
  const stream = observePluginStream(consume, signal);
  const outcome = stream.result.then(
    (value) => ({ value }),
    (error: unknown) => ({ error }),
  );
  try {
    await wire.callUnkeyed("plugins.stream", structuredClone(request), {
      signal: stream.signal,
      observe: stream.observe,
    });
    const final = await outcome;
    if ("error" in final) throw final.error;
    return decode(final.value);
  } finally {
    stream.cleanup();
  }
}
