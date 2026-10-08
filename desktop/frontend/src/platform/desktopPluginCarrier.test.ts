import { afterEach, expect, it, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { createElement } from "react";
import { PopoverPrimitive } from "@/ui/primitives";
import { desktopPluginCarrier } from "./desktopPluginCarrier";
import type { DesktopHostBinding } from "./desktopHost";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function fixture(open: () => Promise<string>) {
  let listener: ((value: unknown) => void) | undefined;
  const unlisten = vi.fn(() => {
    listener = undefined;
  });
  const disconnect = vi.fn();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect = disconnect;
    },
  );
  const binding: DesktopHostBinding = {
    call: vi.fn(async (method: string) => (method.endsWith("OpenPluginPage") ? open() : undefined)),
    on: async (_event, receive) => {
      listener = receive;
      return unlisten;
    },
  };
  const container = document.createElement("div");
  vi.spyOn(container, "getBoundingClientRect").mockReturnValue(new DOMRect(12, 20, 300, 400));
  return {
    carrier: desktopPluginCarrier(binding),
    binding,
    container,
    unlisten,
    disconnect,
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
  expect(receive.mock.calls).toEqual([[{ type: "ready" }]]);
  f.emit("other", { type: "request" });
  f.emit("page-1", { type: "connected" });
  expect(receive).toHaveBeenLastCalledWith({ type: "connected" });
  await page.send({ type: "boot" });
  expect(f.binding.call).toHaveBeenCalledWith(
    "main.DesktopHost.SendPluginPage",
    "page-1",
    '{"type":"boot"}',
  );
  controller.abort();
  await page.close();
  expect(f.unlisten).toHaveBeenCalledTimes(1);
  expect(f.disconnect).toHaveBeenCalledTimes(1);
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
  await vi.waitFor(() =>
    expect(f.binding.call).toHaveBeenLastCalledWith(
      "main.DesktopHost.PositionPluginPage",
      "page-popup",
      { x: 12, y: 20, width: 0, height: 0 },
    ),
  );
  view.unmount();
  await vi.waitFor(() =>
    expect(f.binding.call).toHaveBeenLastCalledWith(
      "main.DesktopHost.PositionPluginPage",
      "page-popup",
      { x: 12, y: 20, width: 300, height: 400 },
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
