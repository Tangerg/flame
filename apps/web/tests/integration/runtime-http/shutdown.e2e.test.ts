import { once } from "node:events";
import { createConnection } from "node:net";
import { RpcConnectionError } from "@flame/runtime-client";
import { describe, expect, it } from "vitest";
import { useRuntimeFixture } from "./fixture";
import { within } from "./observation";

describe("Runtime process shutdown over HTTP", () => {
  const fixture = useRuntimeFixture("http-drain");

  it("ends an admitted subscription and closes a socket waiting for its first request", async () => {
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
    try {
      await within(once(socket, "connect"), "a TCP connection with no request headers");
      await fixture.stopRuntimeProcess();
      await within(closed, "the unstarted HTTP connection closing");
      await expect(within(events.next(), "the admitted subscription ending")).rejects.toBeInstanceOf(
        RpcConnectionError,
      );
    } finally {
      socket.destroy();
      controller.abort();
      await events.return?.();
    }
  }, 30_000);
});
