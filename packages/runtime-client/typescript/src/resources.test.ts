import { afterEach, expect, it } from "vitest";
import { openResourceLease } from "./resources";
import type { WireCall } from "./wireCallPath";
import { createFlameClient } from "./sdk";
import { createHttpTransport } from "./transports/http";

let dispose: (() => Promise<void>) | undefined;
afterEach(async () => {
  await dispose?.();
  dispose = undefined;
});

it("joins an acknowledged HTTP resource stream after its own cancellation", async () => {
  let requestSignal: AbortSignal | undefined;
  let body: ReadableStream<Uint8Array> | undefined;
  const fetcher = (async (_input: unknown, init?: RequestInit) => {
    requestSignal = init!.signal!;
    const request = JSON.parse(init!.body as string) as { id: string };
    body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          new TextEncoder().encode(
            `data: ${JSON.stringify({
              jsonrpc: "2.0",
              id: request.id,
              result: { token: "ABCDEFGHIJKLMNOPQRSTUVWXYZ", entry: "component.js" },
            })}\n\n`,
          ),
        );
        requestSignal!.addEventListener(
          "abort",
          () => {
            controller.error(new DOMException("aborted", "AbortError"));
          },
          { once: true },
        );
      },
    });
    return new Response(body, { headers: { "Content-Type": "text/event-stream" } });
  }) as typeof fetch;
  const client = createFlameClient(
    createHttpTransport({ baseUrl: "http://resources.test", fetch: fetcher }),
  );
  dispose = () => client.close();
  const resource = await client.plugins.openResources({
    installationId: "00000000-0000-4000-8000-000000000001",
    digest: "a".repeat(64),
    componentId: "board",
  });

  await expect(resource.close()).resolves.toBeUndefined();
  await expect(resource.closed).resolves.toBeUndefined();
  expect(requestSignal!.aborted).toBe(true);
  expect(body!.locked).toBe(false);
});

it("resource ownership settles when the RPC client closes after acknowledgement", async () => {
  const connection = new AbortController();
  const owner = { closedSignal: connection.signal, onStreamEnd: () => () => undefined };
  const call = (async () => ({
    token: "ephemeral",
    entry: "io.github.tangerg.flame/component.js",
  })) as WireCall;
  const resource = await openResourceLease(owner, call, {
    installationId: "00000000-0000-4000-8000-000000000001",
    digest: "a".repeat(64),
    componentId: "board",
  });
  const observed = resource.closed;
  connection.abort(new Error("connection lost"));
  await expect(observed).rejects.toThrow("connection lost");
  await expect(resource.close()).rejects.toThrow("connection lost");
});
