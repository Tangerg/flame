import { isExpectedAbort, retainCleanupFailure } from "./body";
import { type EventSourceMessage } from "eventsource-parser";
import { errorMessage, RpcConnectionError, RpcProtocolError, RpcTransportError } from "../errors";
import { isNotification, parseRpcMessage, type RpcMessage } from "../types";
import {
  wireMethodAcceptsReplayCursor,
  type WireStreamingMethodName,
} from "@flame/runtime-client/methods";

interface EventStreamTextParser {
  feed(chunk: string): void;
}

const sseMessageEvent = "message";

export function admitStreamFrame(
  event: EventSourceMessage,
  method: WireStreamingMethodName,
  maximumFrameBytes: number | undefined,
  requestId: string | undefined,
): RpcMessage {
  if (
    maximumFrameBytes !== undefined &&
    new TextEncoder().encode(event.data).byteLength + 1 > maximumFrameBytes
  )
    throw new RpcTransportError(
      "event-stream frame exceeds its byte capacity",
      undefined,
      requestId,
    );
  const invalid = (path: string, detail: string) =>
    new RpcProtocolError("event-stream frame", [{ path, detail }], requestId);
  if (event.event !== undefined && event.event !== sseMessageEvent)
    throw invalid("$.event", "must use the default event name");
  const message = parseRpcMessage(event.data);
  if (!message) throw invalid("$", "must contain a valid JSON-RPC envelope");
  const cursor = event.id;
  if (
    cursor !== undefined &&
    cursor !== "" &&
    (!isNotification(message) || !wireMethodAcceptsReplayCursor(method))
  )
    throw invalid("$.id", "is only valid on Run replay notifications");
  return message;
}

export function utf8StreamDecoder(requestId: string | undefined): (chunk?: Uint8Array) => string {
  const decoder = new TextDecoder("utf-8", { fatal: true });
  return (chunk) => {
    try {
      return decoder.decode(chunk, { stream: chunk !== undefined });
    } catch (cause) {
      const error = new RpcProtocolError(
        "event stream",
        [{ path: "$", detail: "must contain valid UTF-8" }],
        requestId,
      );
      error.cause = cause;
      throw error;
    }
  };
}

export async function feedEventStreamText(
  parser: EventStreamTextParser,
  text: string,
  afterLine: () => Promise<boolean> | undefined,
): Promise<boolean> {
  let start = 0;
  while (start < text.length) {
    let lineEnd = start;
    while (lineEnd < text.length && text[lineEnd] !== "\n" && text[lineEnd] !== "\r") {
      lineEnd++;
    }
    if (lineEnd === text.length) {
      parser.feed(text.slice(start));
      const delivery = afterLine();
      return delivery === undefined ? true : delivery;
    }
    if (text[lineEnd] === "\r" && text[lineEnd + 1] === "\n") lineEnd++;
    parser.feed(text.slice(start, lineEnd + 1));
    const delivery = afterLine();
    if (delivery !== undefined && !(await delivery)) return false;
    start = lineEnd + 1;
  }
  return true;
}

// Resolves false when the caller abandons the stream before its consumer
// settles the delivered event; a consumer failure still rejects.
export function consumedBeforeAbort(
  delivery: Promise<void>,
  signal?: AbortSignal,
): Promise<boolean> {
  return new Promise<boolean>((resolve, reject) => {
    const abort = () => {
      cleanup();
      resolve(false);
    };
    const cleanup = () => signal?.removeEventListener("abort", abort);
    void delivery.then(
      () => {
        cleanup();
        resolve(true);
      },
      (error: unknown) => {
        cleanup();
        reject(error);
      },
    );
    if (signal?.aborted) {
      resolve(false);
      return;
    }
    signal?.addEventListener("abort", abort, { once: true });
  });
}

// A read failure the owner did not cause by aborting becomes the stream's error.
export function streamReadError(
  err: unknown,
  signal: AbortSignal | undefined,
  requestId: string | undefined,
): Error | undefined {
  if (signal && isExpectedAbort(err, signal)) return undefined;
  const error =
    err instanceof RpcTransportError || err instanceof RpcProtocolError
      ? err
      : new RpcConnectionError(errorMessage(err), requestId);
  if (error !== err) error.cause = err;
  return error;
}

export function withCleanupFailure(
  streamError: Error | undefined,
  cancelError: unknown,
  requestId: string | undefined,
): Error {
  if (streamError) {
    retainCleanupFailure(streamError, cancelError);
    return streamError;
  }
  const error = new RpcConnectionError("response body cancellation failed", requestId);
  error.cause = cancelError;
  return error;
}
