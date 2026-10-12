import { captureTheme } from "@flame/ui/theme";
import { afterEach, expect, it, vi } from "vitest";
import { RpcError } from "@flame/runtime-contract/client";
import { usePluginErrorStore } from "@/plugins/sdk/errors";
import { openPluginViewFrame } from "./pluginViewFrame";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import type { PluginViewStatus } from "../application/pluginView";

afterEach(() => {
  vi.useRealTimers();
  usePluginErrorStore.getState().clearAll();
});

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
    binding: { installationId: "installation", digest: "release", viewId: "view" },
    resource: vi.fn(async (_signal: AbortSignal) => "<!doctype html>"),
    initial: vi.fn(async (_signal: AbortSignal) => ({ data: [] })),
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
      openPluginViewFrame({
        container: document.createElement("div"),
        carrier,
        reads,
        signal: signal.signal,
        status,
        theme: captureTheme(document.documentElement, "light", 14),
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
  expect(f.reads.resource).toHaveBeenCalledOnce();
  expect(f.send).toHaveBeenCalledWith({
    type: "boot",
    html: expect.stringContaining("<!doctype html>"),
    initial: { data: [] },
    theme: captureTheme(document.documentElement, "light", 14),
  });
  expect(f.reads.read).not.toHaveBeenCalled();
  f.publish({ type: "request", request: { type: "read", cursor: "next" } });
  await vi.waitFor(() => expect(f.send).toHaveBeenCalledTimes(2));
  expect(f.reads.read.mock.calls[0]?.[0]).toBe("next");
  f.publish({ type: "request", request: { type: "read", sessionId: "other" } });
  expect(f.status).toHaveBeenLastCalledWith({
    type: "failure",
    stage: "protocol",
    reason: expect.stringContaining("unsupported"),
  });
  await dispose.close();
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
  await dispose.close();
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
  await dispose.close();
});
it("joins a carrier that finishes opening after retirement", async () => {
  const f = fixture();
  const opened = Promise.withResolvers<{ send: typeof f.send; close: typeof f.close }>();
  f.carrier.open = vi.fn(async () => opened.promise);
  const dispose = f.mount();
  await vi.waitFor(() => expect(f.carrier.open).toHaveBeenCalledOnce());
  f.signal.abort();
  opened.resolve({ send: f.send, close: f.close });
  await dispose.close();
  expect(f.close).toHaveBeenCalledOnce();
  expect(f.reads.resource).not.toHaveBeenCalled();
});

it("rejects connection before publishing boot", async () => {
  const f = fixture();
  const loaded = Promise.withResolvers<Awaited<ReturnType<typeof f.reads.resource>>>();
  f.reads.resource.mockImplementation(() => loaded.promise);
  const dispose = f.mount();
  await vi.waitFor(() => expect(f.reads.resource).toHaveBeenCalledOnce());
  f.publish({ type: "connected" });
  loaded.resolve("<!doctype html>");
  await dispose.close();
  expect(f.status).toHaveBeenLastCalledWith({
    type: "failure",
    stage: "protocol",
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
      stage: "publication",
      reason: "The plugin channel could not publish its result.",
    }),
  );
  await dispose.close();
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
  const disposed = dispose.close().catch((error) => {
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
  await dispose.close();
  expect(f.status.mock.calls).toEqual([
    [{ type: "loading" }],
    [{ type: "ready" }],
    [{ type: "failure", stage: "carrier", reason: "native page terminated" }],
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
  expect(f.reads.resource).not.toHaveBeenCalled();
  f.publish({ type: "failure", reason: "carrier unavailable" });
  await dispose.close();
  expect(f.reads.resource).not.toHaveBeenCalled();
  expect(f.status).toHaveBeenLastCalledWith({
    type: "failure",
    stage: "carrier",
    reason: "carrier unavailable",
  });
});

it("joins resource loading and an allocated page when retired during startup", async () => {
  const f = fixture();
  const loaded = Promise.withResolvers<Awaited<ReturnType<typeof f.reads.resource>>>();
  f.reads.resource.mockImplementation(() => loaded.promise);
  const dispose = f.mount();
  await vi.waitFor(() => expect(f.reads.resource).toHaveBeenCalledOnce());
  const disposed = dispose.close();
  await vi.waitFor(() => expect(f.close).toHaveBeenCalledOnce());
  expect(f.reads.resource.mock.calls[0]?.[0].aborted).toBe(true);
  let completed = false;
  void disposed.then(() => (completed = true));
  await Promise.resolve();
  expect(completed).toBe(false);
  loaded.resolve("<!doctype html>");
  await disposed;
  expect(f.send).not.toHaveBeenCalled();
  expect(f.status).toHaveBeenLastCalledWith({ type: "loading" });
});

it("settles a renderer format failure and joins its admitted read", async () => {
  const f = fixture();
  const pending = Promise.withResolvers<{ data: [] }>();
  f.reads.read.mockImplementation(() => pending.promise);
  const dispose = await f.ready();
  f.publish({ type: "request", request: { type: "read" } });
  f.publish({
    type: "request",
    request: { type: "failure", reason: "Unsupported recorded result format." },
  });
  expect(f.status).toHaveBeenLastCalledWith({
    type: "failure",
    stage: "guest",
    reason: "Unsupported recorded result format.",
  });
  expect(f.reads.read.mock.calls[0]?.[1].aborted).toBe(true);
  pending.resolve({ data: [] });
  await dispose.close();
  expect(f.close).toHaveBeenCalledOnce();
  expect(f.send).toHaveBeenCalledOnce();
});

it("queues a successor whose predecessor reply was delivered before publication acknowledgement", async () => {
  const f = fixture();
  const acknowledged = Promise.withResolvers<void>();
  let firstReply = true;
  f.send.mockImplementation(async (message: unknown) => {
    if ((message as { type: string }).type === "boot") f.publish({ type: "connected" });
    else if (firstReply) {
      firstReply = false;
      f.publish({ type: "request", request: { type: "read", cursor: "successor" } });
      await acknowledged.promise;
    }
  });
  const dispose = await f.ready();
  try {
    f.publish({ type: "request", request: { type: "read", cursor: "first" } });
    await vi.waitFor(() => expect(f.send).toHaveBeenCalledTimes(2));
    expect(f.status).toHaveBeenLastCalledWith({ type: "ready" });
    expect(f.reads.read).toHaveBeenCalledTimes(1);
    acknowledged.resolve();
    await vi.waitFor(() => expect(f.reads.read).toHaveBeenCalledTimes(2));
    expect(f.reads.read.mock.calls[1]?.[0]).toBe("successor");
    expect(f.close).not.toHaveBeenCalled();
  } finally {
    acknowledged.resolve();
    await dispose.close();
  }
});

it.each(["resource", "initial"] as const)(
  "does not spend the guest handshake deadline while %s is still loading",
  async (stage) => {
    const f = fixture();
    vi.useFakeTimers();
    const loaded = Promise.withResolvers<void>();
    if (stage === "resource")
      f.reads.resource.mockImplementation(async () => {
        await loaded.promise;
        return "<!doctype html>";
      });
    else
      f.reads.initial.mockImplementation(async () => {
        await loaded.promise;
        return { data: [] };
      });
    const dispose = f.mount();
    try {
      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(10001);
      expect(f.send).not.toHaveBeenCalled();
      expect(f.status).toHaveBeenLastCalledWith({ type: "loading" });
      expect(f.reads[stage].mock.calls[0]?.[0].aborted).toBe(false);
    } finally {
      loaded.resolve();
      await dispose.close();
    }
  },
);

it("does not let predecessor cleanup erase an active successor", async () => {
  const f = fixture();
  const sent = Promise.withResolvers<void>();
  const next = Promise.withResolvers<{ data: [] }>();
  f.reads.read.mockResolvedValueOnce({ data: [] }).mockImplementationOnce(() => next.promise);
  f.send.mockImplementation(async (message: unknown) => {
    if ((message as { type: string }).type === "boot") f.publish({ type: "connected" });
    else if (f.reads.read.mock.calls.length === 1) await sent.promise;
  });
  const dispose = await f.ready();
  f.publish({ type: "request", request: { type: "read", cursor: "first" } });
  await vi.waitFor(() => expect(f.send).toHaveBeenCalledTimes(2));
  f.publish({ type: "request", request: { type: "read", cursor: "next" } });
  sent.resolve();
  await vi.waitFor(() => expect(f.reads.read).toHaveBeenCalledTimes(2));
  f.publish({ type: "request", request: { type: "read", cursor: "overlapping" } });
  expect(f.status).toHaveBeenLastCalledWith({
    type: "failure",
    stage: "protocol",
    reason: expect.stringContaining("pending read"),
  });
  next.resolve({ data: [] });
  await dispose.close();
  expect(f.send).toHaveBeenCalledTimes(2);
});

it("retires a failed publication without starting its queued successor", async () => {
  const f = fixture();
  const sent = Promise.withResolvers<void>();
  f.send.mockImplementation(async (message: unknown) => {
    if ((message as { type: string }).type === "boot") f.publish({ type: "connected" });
    else await sent.promise;
  });
  const dispose = await f.ready();
  f.publish({ type: "request", request: { type: "read" } });
  await vi.waitFor(() => expect(f.send).toHaveBeenCalledTimes(2));
  f.publish({ type: "request", request: { type: "read", cursor: "next" } });
  sent.reject(new Error("lost publication"));
  await vi.waitFor(() =>
    expect(f.status).toHaveBeenLastCalledWith({
      type: "failure",
      stage: "publication",
      reason: expect.any(String),
    }),
  );
  await dispose.close();
  expect(f.reads.read).toHaveBeenCalledTimes(1);
});

it("joins the initial read after resource failure and preserves the failing stage", async () => {
  const f = fixture();
  const initial = Promise.withResolvers<{ data: [] }>();
  f.reads.initial.mockImplementation(() => initial.promise);
  f.reads.resource.mockRejectedValue(new Error("missing resource"));
  const dispose = f.mount();
  await vi.waitFor(() =>
    expect(f.status).toHaveBeenLastCalledWith({
      type: "failure",
      stage: "resource",
      reason: expect.any(String),
    }),
  );
  expect(f.reads.initial.mock.calls[0]![0].aborted).toBe(true);
  let settled = false;
  const disposing = dispose.close().then(() => {
    settled = true;
  });
  await Promise.resolve();
  expect(settled).toBe(false);
  initial.resolve({ data: [] });
  await disposing;
  expect(f.send).not.toHaveBeenCalled();
  expect(f.close).toHaveBeenCalledOnce();
});

it("starts the guest deadline after boot publication settles", async () => {
  const f = fixture();
  vi.useFakeTimers();
  const sent = Promise.withResolvers<void>();
  f.send.mockImplementation(() => sent.promise);
  const dispose = f.mount();
  await vi.advanceTimersByTimeAsync(0);
  expect(f.send).toHaveBeenCalledOnce();
  await vi.advanceTimersByTimeAsync(10001);
  expect(f.status).toHaveBeenLastCalledWith({ type: "loading" });
  sent.resolve();
  await vi.advanceTimersByTimeAsync(0);
  await vi.advanceTimersByTimeAsync(10001);
  expect(f.status).toHaveBeenLastCalledWith({
    type: "failure",
    stage: "handshake",
    reason: expect.any(String),
  });
  await dispose.close();
});

it("retains safe host diagnostics while withholding integration details from the guest", async () => {
  const f = fixture();
  const secret = "credential-and-private-content";
  f.reads.read.mockRejectedValue(
    new RpcError(
      { message: secret, data: { type: "plugin_unavailable", detail: secret } },
      "request-42",
    ),
  );
  const dispose = await f.ready();
  f.publish({ type: "request", request: { type: "read" } });
  await vi.waitFor(() => expect(f.send).toHaveBeenCalledTimes(2));
  const log = usePluginErrorStore.getState().log.at(-1)!;
  expect(JSON.parse(log.detail!)).toMatchObject({
    binding: f.reads.binding,
    stage: "read",
    problem: { category: "plugin_unavailable", requestId: "request-42" },
  });
  expect(JSON.stringify(log)).not.toContain(secret);
  expect(JSON.stringify(f.send.mock.calls)).not.toContain(secret);
  expect(f.status).toHaveBeenLastCalledWith({ type: "ready" });
  await dispose.close();
});

it("updates appearance on the existing port without rereading business data", async () => {
  const f = fixture();
  const frame = await f.ready();
  const theme = captureTheme(document.documentElement, "dark", 18);
  frame.updateTheme(theme);
  await vi.waitFor(() =>
    expect(f.send).toHaveBeenLastCalledWith({ type: "reply", reply: { type: "theme", theme } }),
  );
  expect(f.reads.resource).toHaveBeenCalledOnce();
  expect(f.reads.initial).toHaveBeenCalledOnce();
  expect(f.reads.read).not.toHaveBeenCalled();
  await frame.close();
  const sent = f.send.mock.calls.length;
  frame.updateTheme(captureTheme(document.documentElement, "light", 14));
  expect(f.send).toHaveBeenCalledTimes(sent);
});

it("publishes the latest projection when appearance changes during handshake", async () => {
  const f = fixture();
  const boot = Promise.withResolvers<void>();
  f.send.mockImplementationOnce(() => boot.promise);
  const frame = f.mount();
  await vi.waitFor(() =>
    expect(f.send).toHaveBeenCalledWith(expect.objectContaining({ type: "boot" })),
  );
  const theme = captureTheme(document.documentElement, "dark", 18);
  frame.updateTheme(theme);
  f.publish({ type: "connected" });
  await vi.waitFor(() =>
    expect(f.send).toHaveBeenLastCalledWith({ type: "reply", reply: { type: "theme", theme } }),
  );
  boot.resolve();
  await frame.close();
});
