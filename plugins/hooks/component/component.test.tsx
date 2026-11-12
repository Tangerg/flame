// @vitest-environment happy-dom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import {
  createFlameClient,
  RpcTransportError,
  type MutationPromise,
} from "@flame/runtime-client";
import {
  createMemoryTransport,
  respondSuccess,
} from "@flame/runtime-client/testing";
import {
  NOTIFICATIONS_RUNTIME_EVENT,
  type HooksListResult,
} from "@flame/runtime-client/wire";
import { Hooks } from "./component";

afterEach(cleanup);

const globalHook = {
  event: "PreToolUse",
  command: "guard",
  failurePolicy: "advisory",
  scope: "global",
  source: "/captured/hooks.json",
  active: true,
} as const;
const empty: HooksListResult = { hooks: [], projectTrusted: false };

it("shows the captured command failure policy before project trust is granted", async () => {
  const f = fixture(
    vi.fn<() => Promise<HooksListResult>>().mockResolvedValue({
      hooks: [
        {
          ...globalHook,
          scope: "project",
          active: false,
          failurePolicy: "required",
        },
      ],
      projectTrusted: false,
    }),
  );
  render(<Hooks context={f.context} />);
  expect(await screen.findByText("Required: errors block")).toBeTruthy();
  expect(screen.getByText("inactive")).toBeTruthy();
  f.controller.abort();
});

function fixture(
  list = vi.fn<() => Promise<HooksListResult>>().mockResolvedValue(empty),
) {
  const controller = new AbortController();
  const setTrust = vi.fn();
  const open = vi.fn(async () => ({ ref: { path: "/repo" }, hooks: { list } }));
  const subscribe = vi.fn(
    (_request: unknown, signal: AbortSignal) =>
      new Promise<never>((_resolve, reject) => {
        signal.addEventListener("abort", () => reject(signal.reason), {
          once: true,
        });
      }),
  );
  const context = {
    runtime: {
      workspaces: { open },
      hooks: { setTrust },
      runtimeEvents: { subscribe },
    },
    locale: "en",
    signal: controller.signal,

    workspace: { ref: { path: "/repo" } },
    track: <P extends Promise<unknown>>(operation: () => P): P => operation(),
  } as unknown as ComponentContext;
  return { context, controller, list, setTrust, open, subscribe };
}

function watchedFixture(
  list: ReturnType<typeof fixture>["list"],
  path = "/repo",
) {
  const f = fixture(list);
  f.open.mockResolvedValue({ ref: { path }, hooks: { list } });
  const transport = createMemoryTransport();
  const watching =
    Promise.withResolvers<Parameters<typeof transport.send>[0]>();
  const send = transport.send;
  transport.send = async (...args) => {
    await send(...args);
    if (args[0].method === "runtime.subscribe") watching.resolve(args[0]);
  };
  const client = createFlameClient(transport);
  const subscribe = vi.fn(client.runtimeEvents.subscribe);
  const open = vi.fn(f.context.runtime.workspaces.open);
  const tracked: Promise<unknown>[] = [];
  const context: ComponentContext = Object.freeze({
    ...f.context,
    runtime: {
      ...f.context.runtime,
      workspaces: { ...f.context.runtime.workspaces, open },
      runtimeEvents: { subscribe },
    },
    workspace: Object.freeze({ ref: Object.freeze({ path }) }),
    track: <P extends Promise<unknown>>(
      operation: (signal: AbortSignal) => P,
    ): P => {
      const result = operation(f.controller.signal);
      tracked.push(result);
      return result;
    },
  });
  return {
    ...f,
    context,
    open,
    subscribe,
    transport,
    client,
    tracked,
    watching: watching.promise,
  };
}

it("retains an accepted catalog and explicit read failure across display locales until retry", async () => {
  const catalog = { hooks: [globalHook], projectTrusted: false };
  const next = {
    hooks: [{ ...globalHook, source: "/captured/new-hooks.json" }],
    projectTrusted: false,
  };
  const reason = "catalog unavailable\n  原始原因: policy store refused";
  const f = watchedFixture(
    vi
      .fn<() => Promise<HooksListResult>>()
      .mockResolvedValueOnce(catalog)
      .mockResolvedValueOnce(catalog)
      .mockRejectedValueOnce(new Error(reason))
      .mockResolvedValue(next),
  );
  const view = render(<Hooks context={f.context} />);
  try {
    await act(async () => {
      await f.tracked[0];
    });
    const watch = await f.watching;
    await act(async () => respondSuccess(f.transport, watch.id, {}));
    expect(f.list).toHaveBeenCalledTimes(2);
    const source = screen.getByText(globalHook.source);
    const refresh = screen.getByRole("button", { name: "Refresh" });
    await act(async () => fireEvent.click(refresh));
    expect(
      screen.getByText(reason, { normalizer: (text) => text }),
    ).toBeTruthy();
    expect(
      screen.getByText("The last accepted read is retained."),
    ).toBeTruthy();
    act(() => refresh.focus());

    await act(async () =>
      view.rerender(
        <Hooks context={Object.freeze({ ...f.context, locale: "zh" })} />,
      ),
    );

    expect(f.list).toHaveBeenCalledTimes(3);
    expect(f.open).toHaveBeenCalledTimes(3);
    expect(f.subscribe).toHaveBeenCalledOnce();
    expect(screen.getByText(globalHook.source)).toBe(source);
    expect(screen.getByText("guard")).toBeTruthy();
    expect(screen.getByText("全局")).toBeTruthy();
    expect(
      screen.getByText(reason, { normalizer: (text) => text }),
    ).toBeTruthy();
    expect(screen.getByText("保留上次接受的读取。")).toBeTruthy();
    expect(screen.getByRole("button", { name: "刷新" })).toBe(refresh);
    expect(document.activeElement).toBe(refresh);

    await act(async () =>
      fireEvent.click(screen.getByRole("button", { name: "重试" })),
    );
    expect(f.list).toHaveBeenCalledTimes(4);
    expect(screen.getByText(next.hooks[0].source)).toBeTruthy();
    expect(screen.queryByText(globalHook.source)).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(f.setTrust).not.toHaveBeenCalled();
  } finally {
    await act(async () => {
      f.controller.abort();
      view.unmount();
      await Promise.allSettled(f.tracked);
      await f.client.close();
    });
  }
});

it("accepts the original held manual read in the current locale without replacing its watch or focus", async () => {
  const catalog = { hooks: [globalHook], projectTrusted: false };
  const reply = {
    hooks: [{ ...globalHook, source: "/captured/refresh-reply.json" }],
    projectTrusted: false,
  };
  const held = Promise.withResolvers<HooksListResult>();
  const f = watchedFixture(
    vi
      .fn<() => Promise<HooksListResult>>()
      .mockResolvedValueOnce(catalog)
      .mockResolvedValueOnce(catalog)
      .mockReturnValueOnce(held.promise)
      .mockResolvedValue(catalog),
  );
  const view = render(
    <>
      <Hooks context={f.context} />
      <button type="button">Later focus</button>
    </>,
  );
  try {
    await act(async () => {
      await f.tracked[0];
    });
    const watch = await f.watching;
    await act(async () => respondSuccess(f.transport, watch.id, {}));
    expect(f.list).toHaveBeenCalledTimes(2);
    await act(async () =>
      fireEvent.click(screen.getByRole("button", { name: "Refresh" })),
    );
    expect(f.list).toHaveBeenCalledTimes(3);
    const originalSignal = f.open.mock.calls[2][1]!;
    const watchSignal = f.subscribe.mock.calls[0][1]!;
    const later = screen.getByRole("button", { name: "Later focus" });
    act(() => later.focus());

    await act(async () =>
      view.rerender(
        <>
          <Hooks context={Object.freeze({ ...f.context, locale: "zh" })} />
          <button type="button">Later focus</button>
        </>,
      ),
    );

    expect(originalSignal.aborted).toBe(false);
    expect(watchSignal.aborted).toBe(false);
    expect(f.list).toHaveBeenCalledTimes(3);
    expect(f.subscribe).toHaveBeenCalledOnce();
    expect(screen.getByText(globalHook.source)).toBeTruthy();
    expect(screen.getByText("全局")).toBeTruthy();
    expect(document.activeElement).toBe(later);
    await act(async () => held.resolve(reply));
    expect(screen.getByText(reply.hooks[0].source)).toBeTruthy();
    expect(screen.queryByText(globalHook.source)).toBeNull();
    expect(
      screen
        .getByRole("button", { name: "刷新" })
        .getAttribute("aria-disabled"),
    ).not.toBe("true");
    expect(f.list).toHaveBeenCalledTimes(3);
    expect(document.activeElement).toBe(later);
  } finally {
    await act(async () => {
      held.resolve(reply);
      f.controller.abort();
      view.unmount();
      await Promise.allSettled(f.tracked);
      await f.client.close();
    });
  }
});

it("refreshes after actual subscription establishment and a delivered hook change", async () => {
  const f = watchedFixture(
    vi
      .fn<() => Promise<HooksListResult>>()
      .mockResolvedValueOnce({ hooks: [globalHook], projectTrusted: false })
      .mockResolvedValueOnce({
        hooks: [{ ...globalHook, command: "gap accepted" }],
        projectTrusted: false,
      })
      .mockResolvedValue({
        hooks: [{ ...globalHook, command: "event accepted" }],
        projectTrusted: false,
      }),
  );
  const view = render(<Hooks context={f.context} />);
  try {
    await act(async () => {
      await f.tracked[0];
    });
    const watch = await f.watching;
    expect(f.list).toHaveBeenCalledOnce();
    expect(screen.getByText("guard")).toBeTruthy();
    expect(watch.params).toEqual({
      topics: ["hooks.changed", "plugins.changed", "files.changed"],
      watches: [{ watchId: "hook-settings", workspace: { path: "/repo" } }],
    });
    await act(async () => respondSuccess(f.transport, watch.id, {}));
    expect(f.list).toHaveBeenCalledTimes(2);
    expect(screen.getByText("gap accepted")).toBeTruthy();

    await act(async () =>
      f.transport.inject(
        {
          jsonrpc: "2.0",
          method: NOTIFICATIONS_RUNTIME_EVENT,
          params: { event: { type: "hooks.changed", sequence: 1 } },
        },
        undefined,
        watch.id,
      ),
    );

    expect(f.list).toHaveBeenCalledTimes(3);
    expect(f.subscribe).toHaveBeenCalledOnce();
    expect(screen.getByText("event accepted")).toBeTruthy();
    expect(screen.queryByText("gap accepted")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  } finally {
    await act(async () => {
      f.controller.abort();
      view.unmount();
      await Promise.allSettled(f.tracked);
      await f.client.close();
    });
  }
});

it("retires the original read and established watch before a keyed target successor", async () => {
  const catalog = { hooks: [globalHook], projectTrusted: false };
  const held = Promise.withResolvers<HooksListResult>();
  const previous = watchedFixture(
    vi
      .fn<() => Promise<HooksListResult>>()
      .mockResolvedValueOnce(catalog)
      .mockResolvedValueOnce(catalog)
      .mockReturnValue(held.promise),
  );
  const next = {
    hooks: [
      { ...globalHook, source: "/next/hooks.json", command: "next guard" },
    ],
    projectTrusted: false,
  };
  const successor = watchedFixture(vi.fn().mockResolvedValue(next), "/next");
  const view = render(<Hooks key="previous" context={previous.context} />);
  try {
    await act(async () => {
      await previous.tracked[0];
    });
    const oldWatch = await previous.watching;
    await act(async () => respondSuccess(previous.transport, oldWatch.id, {}));
    await act(async () =>
      fireEvent.click(screen.getByRole("button", { name: "Refresh" })),
    );
    expect(previous.list).toHaveBeenCalledTimes(3);
    const originalSignal = previous.open.mock.calls[2][1]!;
    const watchSignal = previous.subscribe.mock.calls[0][1]!;

    await act(async () => {
      previous.controller.abort();
      view.rerender(<Hooks key="successor" context={successor.context} />);
    });
    const newWatch = await successor.watching;
    await act(async () => respondSuccess(successor.transport, newWatch.id, {}));
    expect(originalSignal.aborted).toBe(true);
    expect(watchSignal.aborted).toBe(true);
    expect(successor.open.mock.calls[0]).toEqual([
      { path: "/next" },
      expect.any(AbortSignal),
    ]);
    await act(async () => {
      held.resolve(catalog);
      await Promise.allSettled(previous.tracked);
    });
    expect(screen.getByText(next.hooks[0].source)).toBeTruthy();
    expect(screen.getByText("next guard")).toBeTruthy();
    expect(screen.queryByText(globalHook.source)).toBeNull();
    expect(previous.list).toHaveBeenCalledTimes(3);
    expect(successor.list).toHaveBeenCalledTimes(2);
    expect(successor.subscribe).toHaveBeenCalledOnce();
    expect(screen.queryByRole("alert")).toBeNull();
  } finally {
    await act(async () => {
      held.resolve(catalog);
      previous.controller.abort();
      successor.controller.abort();
      view.unmount();
      await Promise.allSettled([...previous.tracked, ...successor.tracked]);
      await previous.client.close();
      await successor.client.close();
    });
  }
});

it.each([{ hooks: [] }, { hooks: [globalHook] }])(
  "waits for the first result and retains the accepted catalog during reread: $hooks",
  async ({ hooks }) => {
    const first = Promise.withResolvers<HooksListResult>();
    const second = Promise.withResolvers<HooksListResult>();
    const f = fixture(
      vi
        .fn<() => Promise<HooksListResult>>()
        .mockReturnValueOnce(first.promise)
        .mockReturnValueOnce(second.promise),
    );
    render(<Hooks context={f.context} />);
    expect(screen.getByRole("status").textContent).toBe("Loading");
    expect(screen.queryByText("No hooks configured")).toBeNull();
    await act(async () => first.resolve({ hooks, projectTrusted: false }));
    const visible = hooks.length ? "PreToolUse" : "No hooks configured";
    expect(screen.getByText(visible)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(screen.getByText(visible)).toBeTruthy();
    await act(async () => second.reject(new Error("catalog unavailable")));
    expect(screen.getByText(visible)).toBeTruthy();
    expect(screen.getByRole("alert").textContent).toContain(
      "The last accepted read is retained.",
    );
    expect(f.open.mock.calls[0]).toEqual([
      { path: "/repo" },
      expect.any(AbortSignal),
    ]);
    f.controller.abort();
  },
);

it("shows each exact source, scope and literal injected payload", async () => {
  const payload = "Reminder:  keep   the\nrelease notes current.";
  const f = fixture(
    vi.fn().mockResolvedValue({
      hooks: [
        globalHook,
        {
          ...globalHook,
          command: undefined,
          failurePolicy: undefined,
          inject: payload,
          scope: "project",
          source: "/我的 项目/.flame/hooks.json",
          active: false,
        },
      ],
      projectRoot: "/我的 项目",
      projectTrusted: false,
    }),
  );
  render(<Hooks context={f.context} />);
  const global = (await screen.findByText(globalHook.source)).closest(
    "article",
  )!;
  const project = screen
    .getByText("/我的 项目/.flame/hooks.json")
    .closest("article")!;
  expect(within(global).getByText("Global")).toBeTruthy();
  expect(within(global).getByText("guard")).toBeTruthy();
  expect(within(project).getByText("Project")).toBeTruthy();
  expect(
    within(project).getByText(payload, { normalizer: (text) => text }),
  ).toBeTruthy();
  expect(within(project).getByText("inactive")).toBeTruthy();
  f.controller.abort();
});

it("recovers the original trust mutation before permitting another intent", async () => {
  const f = fixture(
    vi.fn().mockResolvedValue({
      hooks: [{ ...globalHook, scope: "project" }],
      projectRoot: "/repo",
      projectTrusted: false,
    }),
  );
  const recovery = Promise.withResolvers<void>();
  const retry = vi.fn(() =>
    Object.assign(recovery.promise, {
      idempotencyKey: "original-trust",
      retry,
    }),
  );
  const original: MutationPromise<void> = Object.assign(
    Promise.reject<void>(new RpcTransportError("response lost")),
    { idempotencyKey: "original-trust", retry },
  );
  // The rejection is observed only once the user dispatches the command.
  void original.catch(() => undefined);
  f.setTrust.mockReturnValue(original);
  render(<Hooks context={f.context} />);
  fireEvent.click(await screen.findByRole("switch"));
  await screen.findByText("response lost");
  expect(screen.getByRole("switch").getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(
    screen.getByRole("button", { name: "Reconcile original command" }),
  );
  expect(f.setTrust).toHaveBeenCalledExactlyOnceWith("/repo", true);
  expect(retry).toHaveBeenCalledExactlyOnceWith({ signal: f.context.signal });
  await act(async () => recovery.resolve());
  await waitFor(() =>
    expect(screen.getByRole("switch").getAttribute("aria-disabled")).not.toBe(
      "true",
    ),
  );
  expect(screen.queryByText("Reconcile original command")).toBeNull();
  f.controller.abort();
});

it("ignores a retired read and waits for a session's resolved workspace", async () => {
  const result = Promise.withResolvers<HooksListResult>();
  const f = fixture(vi.fn().mockReturnValue(result.promise));
  const view = render(
    <Hooks
      context={{ ...f.context, sessionId: "ses_active", workspace: undefined }}
    />,
  );
  expect(f.open).not.toHaveBeenCalled();
  view.rerender(<Hooks context={{ ...f.context, sessionId: "ses_active" }} />);
  await waitFor(() => expect(f.open).toHaveBeenCalledOnce());
  f.controller.abort();
  await act(async () =>
    result.resolve({ hooks: [globalHook], projectTrusted: false }),
  );
  expect(screen.queryByText("PreToolUse")).toBeNull();
});

it("settles a retired trust command without refreshing or mutating its successor", async () => {
  const catalog = {
    hooks: [{ ...globalHook, scope: "project" }],
    projectRoot: "/repo",
    projectTrusted: false,
  };
  const previous = fixture(vi.fn().mockResolvedValue(catalog));
  const successor = fixture(
    vi.fn().mockResolvedValue({ ...catalog, projectRoot: "/next" }),
  );
  const accepted = Promise.withResolvers<void>();
  previous.setTrust.mockReturnValue(
    Object.assign(accepted.promise, {
      idempotencyKey: "original-trust",
      retry: vi.fn(),
    }),
  );
  const view = render(<Hooks key="previous" context={previous.context} />);
  fireEvent.click(await screen.findByRole("switch"));
  previous.controller.abort();
  view.rerender(<Hooks key="successor" context={successor.context} />);
  await screen.findByText("/next");
  await act(async () => accepted.resolve());
  expect(previous.setTrust).toHaveBeenCalledExactlyOnceWith("/repo", true);
  expect(previous.list).toHaveBeenCalledOnce();
  expect(successor.setTrust).not.toHaveBeenCalled();
  expect(screen.getByRole("switch").getAttribute("aria-checked")).toBe("false");
  successor.controller.abort();
});
