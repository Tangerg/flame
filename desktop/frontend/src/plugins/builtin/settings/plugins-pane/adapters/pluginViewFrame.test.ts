import { afterEach, expect, it, vi } from "vitest";
import { mountPluginViewFrame } from "./pluginViewFrame";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import type { PluginViewStatus } from "../application/pluginView";

afterEach(() => vi.useRealTimers());

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
    load: vi.fn(async (_signal: AbortSignal) => ({
      html: "<!doctype html>",
      initial: { data: [] },
    })),
    read: vi.fn(async (_cursor: string | undefined, _signal: AbortSignal) => ({ data: [] })),
  };
  const signal = new AbortController();
  const status = vi.fn<(value: PluginViewStatus) => void>();
  const f = {
    carrier,
    reads,
    signal,
    status,
    send,
    close,
    publish: (message: unknown) => receive(message),
    mount: () =>
      mountPluginViewFrame({
        container: document.createElement("div"),
        carrier,
        reads,
        signal: signal.signal,
        status,
        scheme: "light",
      }),
    async ready() {
      const dispose = f.mount();
      await vi.waitFor(() => expect(status).toHaveBeenLastCalledWith({ type: "ready" }));
      return dispose;
    },
  };
  return f;
}
it("reuses the initial read and permits only the bound cursor operation", async () => {
  const f = fixture();
  const dispose = await f.ready();
  expect(f.reads.load).toHaveBeenCalledOnce();
  expect(f.send).toHaveBeenCalledWith({
    type: "boot",
    html: "<!doctype html>",
    initial: { data: [] },
    scheme: "light",
  });
  expect(f.reads.read).not.toHaveBeenCalled();
  f.publish({ type: "request", request: { type: "read", cursor: "next" } });
  await vi.waitFor(() => expect(f.send).toHaveBeenCalledTimes(2));
  expect(f.reads.read.mock.calls[0]?.[0]).toBe("next");
  f.publish({ type: "request", request: { type: "read", sessionId: "other" } });
  expect(f.status).toHaveBeenLastCalledWith({
    type: "failure",
    reason: expect.stringContaining("unsupported"),
  });
  await dispose();
  expect(f.close).toHaveBeenCalledOnce();
});
it("retires pending reads and refuses their late publication", async () => {
  const f = fixture();
  const pending = Promise.withResolvers<{ data: [] }>();
  f.reads.read.mockImplementation(() => pending.promise);
  const dispose = await f.ready();
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
  const dispose = await f.ready();
  f.publish({ type: "request", request: { type: "read" } });
  await vi.waitFor(() =>
    expect(f.send).toHaveBeenLastCalledWith({
      type: "reply",
      reply: { type: "error", reason: expect.any(String) },
    }),
  );
  expect(f.status).toHaveBeenLastCalledWith({ type: "ready" });
  await dispose();
});
it("joins a carrier that finishes opening after retirement", async () => {
  const f = fixture();
  const opened = Promise.withResolvers<{ send: typeof f.send; close: typeof f.close }>();
  f.carrier.open = vi.fn(async () => opened.promise);
  const dispose = f.mount();
  await vi.waitFor(() => expect(f.carrier.open).toHaveBeenCalledOnce());
  f.signal.abort();
  opened.resolve({ send: f.send, close: f.close });
  await dispose();
  expect(f.close).toHaveBeenCalledOnce();
  expect(f.reads.load).not.toHaveBeenCalled();
});

it("rejects connection before publishing boot", async () => {
  const f = fixture();
  const loaded = Promise.withResolvers<Awaited<ReturnType<typeof f.reads.load>>>();
  f.reads.load.mockImplementation(() => loaded.promise);
  const dispose = f.mount();
  await vi.waitFor(() => expect(f.reads.load).toHaveBeenCalledOnce());
  f.publish({ type: "connected" });
  loaded.resolve({ html: "<!doctype html>", initial: { data: [] } });
  await dispose();
  expect(f.status).toHaveBeenLastCalledWith({
    type: "failure",
    reason: "Invalid plugin initialization.",
  });
  expect(f.send).not.toHaveBeenCalled();
});

it("treats publication failure as terminal rather than a failed read", async () => {
  const f = fixture();
  f.send.mockImplementation(async (message: unknown) => {
    const value = message as { type: string; reply?: { type: string } };
    if (value.type === "boot") f.publish({ type: "connected" });
    else if (value.reply?.type === "data") throw new Error("channel unavailable");
  });
  const dispose = await f.ready();
  f.publish({ type: "request", request: { type: "read" } });
  await vi.waitFor(() =>
    expect(f.status).toHaveBeenLastCalledWith({
      type: "failure",
      reason: "The plugin channel could not publish its result.",
    }),
  );
  await dispose();
  expect(f.send).toHaveBeenCalledTimes(2);
});

it("joins the pending read even when physical closure fails", async () => {
  const f = fixture();
  const pending = Promise.withResolvers<{ data: [] }>();
  f.reads.read.mockImplementation(() => pending.promise);
  f.close.mockRejectedValue(new Error("physical close failed"));
  const dispose = await f.ready();
  vi.useFakeTimers();
  f.publish({ type: "request", request: { type: "read" } });
  let completed = false;
  const disposed = dispose().catch((error) => {
    completed = true;
    return error;
  });
  await vi.advanceTimersByTimeAsync(0);
  expect(f.close).toHaveBeenCalledOnce();
  const completedBeforeRead = completed;
  pending.resolve({ data: [] });
  const error = await disposed;
  expect(completedBeforeRead).toBe(false);
  expect(error).toMatchObject({
    name: "AggregateError",
    errors: [expect.objectContaining({ message: "physical close failed" })],
  });
  expect(f.send).toHaveBeenCalledOnce();
});

it("keeps failure terminal when the carrier fails immediately after connecting", async () => {
  const f = fixture();
  f.send.mockImplementation(async () => {
    f.publish({ type: "connected" });
    f.publish({ type: "failure", reason: "native page terminated" });
  });
  const dispose = f.mount();
  await vi.waitFor(() => expect(f.close).toHaveBeenCalledOnce());
  await dispose();
  expect(f.status.mock.calls).toEqual([
    [{ type: "loading" }],
    [{ type: "ready" }],
    [{ type: "failure", reason: "native page terminated" }],
  ]);
});

it("waits for qualification before loading any package resource", async () => {
  const f = fixture();
  f.carrier.open = vi.fn(async (_container, _signal, receive) => {
    f.publish = receive;
    return { send: f.send, close: f.close };
  });
  const dispose = f.mount();
  await vi.waitFor(() => expect(f.carrier.open).toHaveBeenCalledOnce());
  expect(f.reads.load).not.toHaveBeenCalled();
  f.publish({ type: "failure", reason: "carrier unavailable" });
  await dispose();
  expect(f.reads.load).not.toHaveBeenCalled();
  expect(f.status).toHaveBeenLastCalledWith({ type: "failure", reason: "carrier unavailable" });
});

it("joins resource loading and an allocated page when retired during startup", async () => {
  const f = fixture();
  const loaded = Promise.withResolvers<Awaited<ReturnType<typeof f.reads.load>>>();
  f.reads.load.mockImplementation(() => loaded.promise);
  const dispose = f.mount();
  await vi.waitFor(() => expect(f.reads.load).toHaveBeenCalledOnce());
  const disposed = dispose();
  await vi.waitFor(() => expect(f.close).toHaveBeenCalledOnce());
  expect(f.reads.load.mock.calls[0]?.[0].aborted).toBe(true);
  let completed = false;
  void disposed.then(() => (completed = true));
  await Promise.resolve();
  expect(completed).toBe(false);
  loaded.resolve({ html: "<!doctype html>", initial: { data: [] } });
  await disposed;
  expect(f.send).not.toHaveBeenCalled();
  expect(f.status).toHaveBeenLastCalledWith({ type: "loading" });
});
