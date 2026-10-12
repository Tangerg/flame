import { afterEach, expect, it, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { createElement } from "react";
import { PopoverPrimitive } from "@/ui/primitives";
import { desktopPluginCarrier } from "./desktopPluginCarrier";
import { pluginMessageFrames } from "./pluginMessageFrames";
import type { DesktopHostBinding } from "./desktopHost";

afterEach(() => {
  cleanup();
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

function fixture(open: () => Promise<string>) {
  let listener: ((value: unknown) => void) | undefined;
  const unlisten = vi.fn(() => {
    listener = undefined;
  });
  const frames = new Map<number, FrameRequestCallback>();
  let sequence = 0;
  vi.stubGlobal(
    "requestAnimationFrame",
    vi.fn((callback: FrameRequestCallback) => {
      frames.set(++sequence, callback);
      return sequence;
    }),
  );
  const cancelFrame = vi.fn((id: number) => {
    frames.delete(id);
  });
  vi.stubGlobal("cancelAnimationFrame", cancelFrame);
  const binding: DesktopHostBinding = {
    call: vi.fn(async (method: string) =>
      method.endsWith("OpenPluginPage")
        ? open().then((id) => ({ id, maxFrameBytes: 4 << 20 }))
        : undefined,
    ),
    on: async (_event, receive) => {
      listener = receive;
      return unlisten;
    },
  };
  const container = document.createElement("div");
  document.body.append(container);
  vi.spyOn(container, "getBoundingClientRect").mockReturnValue(new DOMRect(12, 20, 300, 400));
  return {
    carrier: desktopPluginCarrier(binding),
    binding,
    container,
    unlisten,
    cancelFrame,
    tick: () => {
      const pending = [...frames.values()];
      frames.clear();
      for (const callback of pending) callback(performance.now());
    },
    emit: (id: string, message: unknown) => listener?.({ id, message }),
  };
}

it("binds early readiness and later traffic to the native instance, then retires that instance", async () => {
  let complete!: (id: string) => void;
  const f = fixture(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  const controller = new AbortController();
  const receive = vi.fn();
  const opening = f.carrier.open(f.container, controller.signal, receive);
  await vi.waitFor(() => expect(f.binding.call).toHaveBeenCalled());
  f.emit("other", { type: "ready" });
  f.emit("page-1", { type: "request", request: { cursor: "unaccepted" } });
  f.emit("page-1", { type: "ready" });
  complete("page-1");
  const page = await opening;
  expect(receive.mock.calls).toEqual([
    [{ type: "request", request: { cursor: "unaccepted" } }],
    [{ type: "ready" }],
  ]);
  f.emit("other", { type: "request" });
  f.emit("page-1", { type: "connected" });
  expect(receive).toHaveBeenLastCalledWith({ type: "connected" });
  await page.send({ type: "boot" });
  expect(f.binding.call).toHaveBeenCalledWith(
    "main.DesktopHost.SendPluginPage",
    "page-1",
    [...pluginMessageFrames({ type: "boot" }, 4 << 20)][0],
  );
  controller.abort();
  await page.close();
  expect(f.unlisten).toHaveBeenCalledTimes(1);
  expect(f.cancelFrame).toHaveBeenCalledTimes(1);
  expect(f.binding.call).toHaveBeenCalledWith("main.DesktopHost.ClosePluginPage", "page-1");
  expect(
    vi.mocked(f.binding.call).mock.calls.filter(([method]) => method.endsWith("ClosePluginPage")),
  ).toHaveLength(1);
  await expect(page.send({ type: "boot" })).rejects.toThrow("retired");
});

it("joins a native allocation that returns after cancellation and closes its exact ID", async () => {
  let complete!: (id: string) => void;
  const f = fixture(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  const controller = new AbortController();
  const receive = vi.fn();
  const opening = f.carrier.open(f.container, controller.signal, receive);
  const settled = opening.then(
    () => undefined,
    (error) => error,
  );
  await vi.waitFor(() => expect(f.binding.call).toHaveBeenCalled());
  controller.abort();
  f.emit("page-late", { type: "ready" });
  complete("page-late");
  expect(await settled).toMatchObject({ name: "AbortError" });
  expect(receive).not.toHaveBeenCalled();
  expect(f.binding.call).toHaveBeenCalledWith("main.DesktopHost.ClosePluginPage", "page-late");
  expect(f.unlisten).toHaveBeenCalledTimes(1);
});

it("does not allocate a view when listener setup completes after cancellation", async () => {
  const f = fixture(async () => "unused");
  let complete!: () => void;
  f.binding.on = () =>
    new Promise((resolve) => {
      complete = () => resolve(f.unlisten);
    });
  const controller = new AbortController();
  const opening = f.carrier.open(f.container, controller.signal, vi.fn());
  const settled = opening.then(
    () => undefined,
    (error) => error,
  );
  controller.abort();
  complete();
  expect(await settled).toMatchObject({ name: "AbortError" });
  expect(f.binding.call).not.toHaveBeenCalled();
  expect(f.unlisten).toHaveBeenCalledTimes(1);
});

it("yields the native surface to the actual workbench popup and restores its bounds after dismissal", async () => {
  const f = fixture(async () => "page-popup");
  const view = render(
    createElement(
      PopoverPrimitive.Root,
      { open: false },
      createElement(
        PopoverPrimitive.Portal,
        null,
        createElement(
          PopoverPrimitive.Positioner,
          null,
          createElement(PopoverPrimitive.Popup, null, "workbench popup"),
        ),
      ),
    ),
  );
  const page = await f.carrier.open(f.container, new AbortController().signal, vi.fn());
  view.rerender(
    createElement(
      PopoverPrimitive.Root,
      { open: true },
      createElement(
        PopoverPrimitive.Portal,
        null,
        createElement(
          PopoverPrimitive.Positioner,
          null,
          createElement(PopoverPrimitive.Popup, null, "workbench popup"),
        ),
      ),
    ),
  );
  await screen.findByText("workbench popup");
  f.tick();
  await vi.waitFor(() =>
    expect(f.binding.call).toHaveBeenLastCalledWith(
      "main.DesktopHost.PositionPluginPage",
      "page-popup",
      {
        frame: { x: 12, y: 20, width: 300, height: 400 },
        clip: { x: 12, y: 20, width: 0, height: 0 },
      },
    ),
  );
  for (const element of document.querySelectorAll(
    "[data-base-ui-portal] [data-open], [data-base-ui-portal] [data-ending-style]",
  )) {
    element.removeAttribute("data-open");
    element.removeAttribute("data-ending-style");
  }
  f.tick();
  expect(f.binding.call).toHaveBeenLastCalledWith(
    "main.DesktopHost.PositionPluginPage",
    "page-popup",
    {
      frame: { x: 12, y: 20, width: 300, height: 400 },
      clip: { x: 12, y: 20, width: 0, height: 0 },
    },
  );
  view.unmount();
  await Promise.resolve();
  f.tick();
  await vi.waitFor(() =>
    expect(f.binding.call).toHaveBeenLastCalledWith(
      "main.DesktopHost.PositionPluginPage",
      "page-popup",
      {
        frame: { x: 12, y: 20, width: 300, height: 400 },
        clip: { x: 12, y: 20, width: 300, height: 400 },
      },
    ),
  );
  await page.close();
});

it("joins the same physical closure when abort and lifetime cleanup overlap", async () => {
  const f = fixture(async () => "page-close");
  const closed = Promise.withResolvers<void>();
  const invoke = f.binding.call;
  f.binding.call = vi.fn((method, ...args) =>
    method.endsWith("ClosePluginPage") ? closed.promise : invoke(method, ...args),
  );
  const controller = new AbortController();
  const page = await f.carrier.open(f.container, controller.signal, vi.fn());
  controller.abort();
  const first = page.close();
  expect(page.close()).toBe(first);
  let completed = false;
  void first.then(() => {
    completed = true;
  });
  await Promise.resolve();
  expect(completed).toBe(false);
  closed.resolve();
  await first;
  expect(completed).toBe(true);
});

it.each([
  [{ type: "failure", reason: "native isolation failed" }, { type: "ready" }],
  [{ type: "ready" }, { type: "ready" }],
])("preserves early control messages in delivery order: %j then %j", async (first, second) => {
  const allocated = Promise.withResolvers<string>();
  const f = fixture(() => allocated.promise);
  const receive = vi.fn();
  const opening = f.carrier.open(f.container, new AbortController().signal, receive);
  await vi.waitFor(() => expect(f.binding.call).toHaveBeenCalled());
  f.emit("page-early", first);
  f.emit("page-early", second);
  allocated.resolve("page-early");
  const page = await opening;
  await page.close();
  expect(receive.mock.calls).toEqual([[first], [second]]);
});

it("keeps buffering through initial positioning so later messages cannot overtake earlier ones", async () => {
  const allocated = Promise.withResolvers<string>();
  const positioned = Promise.withResolvers<void>();
  const f = fixture(() => allocated.promise);
  const invoke = f.binding.call;
  f.binding.call = vi.fn((method, ...args) =>
    method.endsWith("PositionPluginPage") ? positioned.promise : invoke(method, ...args),
  );
  const receive = vi.fn();
  const opening = f.carrier.open(f.container, new AbortController().signal, receive);
  await vi.waitFor(() => expect(f.binding.call).toHaveBeenCalled());
  f.emit("page-order", { type: "ready" });
  allocated.resolve("page-order");
  await vi.waitFor(() =>
    expect(f.binding.call).toHaveBeenLastCalledWith(
      "main.DesktopHost.PositionPluginPage",
      "page-order",
      expect.any(Object),
    ),
  );
  f.emit("page-order", { type: "failure", reason: "native process terminated" });
  expect(receive).not.toHaveBeenCalled();
  positioned.resolve();
  const page = await opening;
  await page.close();
  expect(receive.mock.calls).toEqual([
    [{ type: "ready" }],
    [{ type: "failure", reason: "native process terminated" }],
  ]);
});

it.each(["boot", "reply"])(
  "transfers oversized %s results completely within the native capacity",
  async (type) => {
    const f = fixture(async () => "large");
    const page = await f.carrier.open(f.container, new AbortController().signal, vi.fn());
    for (const data of [
      Array.from({ length: 100 }, (_, index) => ({ index, body: "x".repeat(50 << 10) })),
      [{ body: "汉😀".repeat(1 << 20) }],
    ]) {
      vi.mocked(f.binding.call).mockClear();
      const message = { type, data };
      await page.send(message);
      const calls = vi.mocked(f.binding.call).mock.calls;
      expect(calls.length).toBeGreaterThan(1);
      const frames = calls.map(([method, id, frame]) => {
        expect(method).toBe("main.DesktopHost.SendPluginPage");
        expect(id).toBe("large");
        expect(new TextEncoder().encode(String(frame)).byteLength).toBeLessThanOrEqual(4 << 20);
        return JSON.parse(String(frame)) as { index: number; last: boolean; data: string };
      });
      expect(frames.map((frame) => frame.index)).toEqual(frames.map((_, index) => index));
      expect(frames.at(-1)!.last).toBe(true);
      expect(JSON.parse(frames.map((frame) => frame.data).join(""))).toEqual(message);
    }
    await page.close();
  },
);

it("retires a transfer between frames and joins its publication acknowledgement", async () => {
  const f = fixture(async () => "partial");
  const page = await f.carrier.open(f.container, new AbortController().signal, vi.fn());
  const ack = Promise.withResolvers<void>();
  const invoke = f.binding.call;
  f.binding.call = vi.fn((method, ...args) =>
    method.endsWith("SendPluginPage") ? ack.promise : invoke(method, ...args),
  );
  const sending = page.send({ type: "boot", body: "x".repeat(5 << 20) });
  const failure = sending.catch((error) => error);
  await vi.waitFor(() => expect(f.binding.call).toHaveBeenCalledOnce());
  let settled = false;
  const closing = page.close().then(() => {
    settled = true;
  });
  await Promise.resolve();
  expect(settled).toBe(false);
  ack.resolve();
  await closing;
  expect(await failure).toMatchObject({ name: "AbortError" });
  expect(
    vi.mocked(f.binding.call).mock.calls.filter(([method]) => method.endsWith("SendPluginPage")),
  ).toHaveLength(1);
});

it("clips an inline page to its scrollport without shrinking the guest viewport", async () => {
  const f = fixture(async () => "inline");
  const scroller = document.createElement("div");
  scroller.style.overflowY = "auto";
  document.body.append(scroller);
  scroller.append(f.container);
  vi.spyOn(scroller, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 80, 500, 200));
  Object.defineProperty(scroller, "clientHeight", { value: 200 });
  Object.defineProperty(scroller, "offsetHeight", { value: 200 });
  vi.mocked(f.container.getBoundingClientRect).mockReturnValue(new DOMRect(12, -20, 300, 400));
  const page = await f.carrier.open(f.container, new AbortController().signal, vi.fn());
  expect(f.binding.call).toHaveBeenLastCalledWith("main.DesktopHost.PositionPluginPage", "inline", {
    frame: { x: 12, y: -20, width: 300, height: 400 },
    clip: { x: 12, y: 80, width: 300, height: 200 },
  });
  vi.mocked(f.container.getBoundingClientRect).mockReturnValue(new DOMRect(12, 300, 300, 400));
  f.tick();
  await vi.waitFor(() =>
    expect(f.binding.call).toHaveBeenLastCalledWith(
      "main.DesktopHost.PositionPluginPage",
      "inline",
      {
        frame: { x: 12, y: 300, width: 300, height: 400 },
        clip: { x: 12, y: 300, width: 300, height: 0 },
      },
    ),
  );
  await page.close();
});

it("follows a layout shift and releases a detached native surface without waiting for a resize", async () => {
  const f = fixture(async () => "shifted");
  const page = await f.carrier.open(f.container, new AbortController().signal, vi.fn());
  vi.mocked(f.container.getBoundingClientRect).mockReturnValue(new DOMRect(12, 120, 300, 400));
  f.tick();
  await vi.waitFor(() =>
    expect(f.binding.call).toHaveBeenLastCalledWith(
      "main.DesktopHost.PositionPluginPage",
      "shifted",
      {
        frame: { x: 12, y: 120, width: 300, height: 400 },
        clip: { x: 12, y: 120, width: 300, height: 400 },
      },
    ),
  );
  f.container.remove();
  f.tick();
  await vi.waitFor(() =>
    expect(f.binding.call).toHaveBeenLastCalledWith("main.DesktopHost.ClosePluginPage", "shifted"),
  );
  await page.close();
  expect(
    vi.mocked(f.binding.call).mock.calls.filter(([method]) => method.endsWith("ClosePluginPage")),
  ).toHaveLength(1);
});
