import { expect, it } from "vitest";
import { openResourceLease } from "./resources";
import type { WireCall } from "./wireCallPath";

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
