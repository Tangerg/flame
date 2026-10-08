import { expect, it, vi } from "vitest";
import { mountTrajectoryFrame } from "./trajectoryFrame";
import type { PluginCarrier } from "@/foundation/pluginCarrier";

function fixture() {
  let receive: (message: unknown) => void = () => {};
  const send = vi.fn(async (message: unknown) => {
    if ((message as { type: string }).type === "boot") receive({ type: "connected" });
  });
  const close = vi.fn(async () => {});
  const carrier: PluginCarrier = {
    async open(_container, _signal, listener) {
      receive = listener;
      listener({ type: "ready" });
      return { send, close };
    },
  };
  const reads = {
    load: vi.fn(async () => ({ html: "<!doctype html>", initial: { data: [] } })),
    read: vi.fn(async (_cursor: string | undefined, _signal: AbortSignal) => ({ data: [] })),
  };
  const signal = new AbortController();
  const fail = vi.fn();
  return {
    carrier,
    reads,
    signal,
    fail,
    send,
    close,
    publish: (message: unknown) => receive(message),
    mount: () =>
      mountTrajectoryFrame({
        container: document.createElement("div"),
        carrier,
        reads,
        signal: signal.signal,
        fail,
      }),
  };
}
it("reuses the initial read and permits only the bound cursor operation", async () => {
  const f = fixture();
  const dispose = await f.mount();
  expect(f.reads.load).toHaveBeenCalledOnce();
  expect(f.reads.read).not.toHaveBeenCalled();
  f.publish({ type: "request", request: { type: "read", cursor: "next" } });
  await vi.waitFor(() => expect(f.send).toHaveBeenCalledTimes(2));
  expect(f.reads.read.mock.calls[0]?.[0]).toBe("next");
  f.publish({ type: "request", request: { type: "read", sessionId: "other" } });
  expect(f.fail).toHaveBeenCalledWith(expect.stringContaining("unsupported"));
  await dispose();
  expect(f.close).toHaveBeenCalledOnce();
});
it("retires pending reads and refuses their late publication", async () => {
  const f = fixture();
  const pending = Promise.withResolvers<{ data: [] }>();
  f.reads.read.mockImplementation(() => pending.promise);
  const dispose = await f.mount();
  f.publish({ type: "request", request: { type: "read" } });
  f.signal.abort();
  expect(f.reads.read.mock.calls[0]?.[1].aborted).toBe(true);
  pending.resolve({ data: [] });
  await dispose();
  expect(f.send).toHaveBeenCalledOnce();
  expect(f.close).toHaveBeenCalledOnce();
  f.publish({ type: "request", request: { type: "read" } });
  expect(f.reads.read).toHaveBeenCalledOnce();
});
it("shows a failed read without replacing recorded data with empty success", async () => {
  const f = fixture();
  f.reads.read.mockRejectedValue(new Error("unavailable"));
  const dispose = await f.mount();
  f.publish({ type: "request", request: { type: "read" } });
  await vi.waitFor(() =>
    expect(f.send).toHaveBeenLastCalledWith({
      type: "reply",
      reply: { type: "error", reason: expect.any(String) },
    }),
  );
  expect(f.fail).not.toHaveBeenCalled();
  await dispose();
});
it("joins a carrier that finishes opening after retirement", async () => {
  const f = fixture();
  const opened = Promise.withResolvers<{ send: typeof f.send; close: typeof f.close }>();
  f.carrier.open = async () => opened.promise;
  const mount = f.mount();
  f.signal.abort();
  opened.resolve({ send: f.send, close: f.close });
  await expect(mount).rejects.toThrow(/abort/i);
  expect(f.close).toHaveBeenCalledOnce();
  expect(f.reads.load).not.toHaveBeenCalled();
});
