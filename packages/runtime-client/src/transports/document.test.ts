import { expect, it, vi } from "vitest";
import { readResponseDocument } from "./document";
it("admits exact-budget documents and observes cancellation while a read waits", async () => {
  await expect(readResponseDocument(new Response("世界"), 6, "test", undefined)).resolves.toBe(
    "世界",
  );
  const cancel = vi.fn();
  const response = new Response(new ReadableStream({ cancel }));
  const owner = new AbortController();
  const waiting = readResponseDocument(response, 6, "test", undefined, owner.signal);
  owner.abort();
  await expect(waiting).rejects.toBe(owner.signal.reason);
  expect(cancel).toHaveBeenCalledOnce();
});

it.each([new Error("cancel failed"), "cancel failed", undefined])(
  "retains the first abort cancellation rejection (%s) and releases the reader",
  async (cleanup) => {
    const cancel = vi.fn(() => Promise.reject(cleanup));
    const body = new ReadableStream<Uint8Array>({ cancel });
    const owner = new AbortController();
    const reading = readResponseDocument(new Response(body), 6, "test", undefined, owner.signal);
    owner.abort();
    const failure = await reading.catch((error: unknown) => error);
    expect(failure).toBe(owner.signal.reason);
    expect(failure).toHaveProperty("cause", cleanup);
    expect(cancel).toHaveBeenCalledOnce();
    expect(body.locked).toBe(false);
  },
);

it("keeps a document's limit failure primary when cancellation also fails", async () => {
  const cleanup = new Error("cancel failed");
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new Uint8Array(7));
    },
    cancel: () => Promise.reject(cleanup),
  });
  await expect(
    readResponseDocument(new Response(body), 6, "test", "request_1"),
  ).rejects.toMatchObject({
    name: "RpcTransportError",
    requestId: "request_1",
    cause: cleanup,
  });
  expect(body.locked).toBe(false);
});
