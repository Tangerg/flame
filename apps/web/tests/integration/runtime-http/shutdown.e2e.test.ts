import { once } from "node:events";
import { request } from "node:http";
import { createConnection } from "node:net";
import { RpcConnectionError } from "@flame/runtime-client";
import { HTTP_ENDPOINTS, HTTP_HEADERS, RPC_HTTP } from "@flame/runtime-client/wire";
import { describe, expect, it } from "vitest";
import { useRuntimeFixture } from "./fixture";
import { within } from "./observation";

describe("Runtime process shutdown over HTTP", () => {
  const fixture = useRuntimeFixture("http-drain");

  it("drains subscriptions and incomplete HTTP requests", async () => {
    const controller = new AbortController();
    const opened = await fixture.client.runtimeEvents.subscribe(
      { topics: ["taskWorktrees.changed"] },
      controller.signal,
    );
    const events = opened.events[Symbol.asyncIterator]();
    const url = new URL(fixture.baseUrl);
    const socket = createConnection({ host: url.hostname, port: Number(url.port) });
    const closed = once(socket, "close");
    void closed.catch(() => {});
    const body = JSON.stringify({ jsonrpc: "2.0", id: 1, method: "runtime.discover", params: {} });
    const upload = request(new URL(HTTP_ENDPOINTS.rpc.path, url), {
      method: HTTP_ENDPOINTS.rpc.method,
      headers: {
        [HTTP_HEADERS.contentType]: RPC_HTTP.documentMediaType,
        "content-length": Buffer.byteLength(body),
        expect: "100-continue",
      },
    });
    const awaitingBody = once(upload, "continue");
    const uploadClosed = once(upload, "close").catch((error: NodeJS.ErrnoException) => {
      if (error.code !== "ECONNRESET") throw error;
    });
    void uploadClosed.catch(() => {});
    upload.on("response", (response) => response.resume());
    try {
      await within(once(socket, "connect"), "a TCP connection with no request headers");
      upload.flushHeaders();
      await within(awaitingBody, "the Runtime starting to read the request body");
      await fixture.stopRuntimeProcess();
      await within(closed, "the unstarted HTTP connection closing");
      await within(uploadClosed, "the incomplete HTTP upload closing");
      await expect(within(events.next(), "the admitted subscription ending")).rejects.toBeInstanceOf(
        RpcConnectionError,
      );
    } finally {
      socket.destroy();
      upload.destroy();
      controller.abort();
      await events.return?.();
    }
  }, 30_000);
});
