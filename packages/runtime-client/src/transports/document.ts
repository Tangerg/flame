import { errorMessage, RpcConnectionError, RpcProtocolError, RpcTransportError } from "../errors";

const DEFAULT_DOCUMENT_BYTES = 128 * 1024 * 1024;

export function responseDocumentLimit(configured = DEFAULT_DOCUMENT_BYTES): number {
  if (!Number.isSafeInteger(configured) || configured <= 0)
    throw new RangeError("HTTP document capacity must be a positive safe integer");
  return configured;
}

export async function readResponseDocument(
  response: Response,
  limit: number,
  label: string,
  requestId: string | undefined,
  signal?: AbortSignal,
): Promise<string> {
  if (!response.body) {
    signal?.throwIfAborted();
    return "";
  }
  const reader = response.body.getReader();
  const abort = () => {
    void reader.cancel(signal?.reason).catch(() => undefined);
  };
  signal?.addEventListener("abort", abort, { once: true });
  let content = new Uint8Array(0);
  let used = 0;
  let complete = false;
  try {
    signal?.throwIfAborted();
    for (;;) {
      const chunk = await reader.read();
      signal?.throwIfAborted();
      if (chunk.done) break;
      const needed = used + chunk.value.byteLength;
      if (needed > limit)
        throw new RpcTransportError(
          `${label} exceeds its ${limit}-byte capacity`,
          undefined,
          requestId,
        );
      // Bound retained memory independently of the peer's chunk count.
      if (needed > content.byteLength) {
        const larger = new Uint8Array(Math.min(limit, Math.max(needed, content.byteLength * 2)));
        larger.set(content);
        content = larger;
      }
      content.set(chunk.value, used);
      used = needed;
    }
    complete = true;
  } catch (cause) {
    if (cause instanceof RpcTransportError || signal?.aborted) throw cause;
    const error = new RpcConnectionError(
      `${label} could not be read: ${errorMessage(cause)}`,
      requestId,
    );
    error.cause = cause;
    throw error;
  } finally {
    signal?.removeEventListener("abort", abort);
    if (!complete) await reader.cancel().catch(() => undefined);
    reader.releaseLock();
  }
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(content.subarray(0, used));
  } catch (cause) {
    const error = new RpcProtocolError(
      label,
      [{ path: "$", detail: "must contain valid UTF-8" }],
      requestId,
    );
    error.cause = cause;
    throw error;
  }
}
