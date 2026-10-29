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
import { RpcTransportError, type MutationPromise } from "@flame/runtime-client";
import { type HooksListResult } from "@flame/runtime-client/wire";
import { Hooks } from "./component";

afterEach(cleanup);

const globalHook = {
  event: "PreToolUse",
  command: "guard",
  scope: "global",
  source: "/captured/hooks.json",
  active: true,
} as const;
const empty: HooksListResult = { hooks: [], projectTrusted: false };

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
