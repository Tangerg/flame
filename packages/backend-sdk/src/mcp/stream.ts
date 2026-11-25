import { randomUUID } from "node:crypto";
import { exactFields, type Method } from "./contract.ts";
import { callbackRef, callHost, streamDelivery } from "./callback.ts";
import type { CallContext } from "./invocation.ts";

export type StreamFrame<C, R> =
  { kind: "chunk"; value: C } | { kind: "result"; value: R };

// The host acknowledges chunks by an unsigned 64-bit sequence number.
const MAX_SEQUENCE = 2n ** 64n - 1n;

export async function deliverStream(
  frames: () => AsyncIterable<StreamFrame<unknown, unknown>>,
  method: Method,
  context: CallContext,
  reference: unknown,
): Promise<unknown> {
  const ref = callbackRef(reference);
  let final: { value: unknown } | undefined;
  let sequence = 0n;
  for await (const frame of frames()) {
    context.signal.throwIfAborted();
    exactFields(frame, ["kind", "value"]);
    if (final)
      throw new Error("plugin: stream continued after its final result");
    if (frame.kind === "result") {
      final = { value: method.admitOutput(frame.value) };
    } else if (frame.kind === "chunk") {
      const value = method.admitChunk!(frame.value);
      if (sequence === MAX_SEQUENCE)
        throw new Error("plugin: stream sequence exhausted");
      sequence++;
      const acknowledged = await callHost(
        ref,
        streamDelivery,
        { sequence, value },
        context.signal,
        context.invocation,
        randomUUID(),
      );
      if (acknowledged !== true)
        throw new Error(
          "plugin: stream consumer did not acknowledge its chunk",
        );
    } else throw new Error("plugin: invalid stream frame kind");
  }
  if (!final) throw new Error("plugin: stream final result is unconfirmed");
  return final.value;
}
