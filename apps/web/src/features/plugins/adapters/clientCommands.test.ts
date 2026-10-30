import { asyncDisposeSymbol } from "dougong";
import { expect, it, onTestFinished, vi } from "vitest";
import { type ClientCommandInvocation } from "@flame/component-sdk";
import { definePlugin, type ContributionLifetime } from "@/extensions/definePlugin";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { contributionsTo } from "@/extensions/kernel";
import { usePluginErrorStore } from "@/extensions/errors";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { notifyFrom } from "@/features/notifications/public/notifications";
import { createClientCommandRegistration } from "./clientCommands";

vi.mock("@/features/notifications/public/notifications", async (original) => ({
  ...(await original<typeof import("@/features/notifications/public/notifications")>()),
  notifyFrom: vi.fn(),
}));

async function fixture(captureSessionId: () => string | undefined = () => "captured") {
  let lifetime!: ContributionLifetime;
  const host = await startKernel([
    definePlugin({
      name: "test.installed-commands",
      setup(scope) {
        lifetime = scope.lifetime("component");
      },
    }),
  ]);
  onTestFinished(() => stopKernel(host));
  vi.mocked(notifyFrom).mockClear();
  usePluginErrorStore.getState().clearAll();
  const controller = new AbortController();
  const signal = AbortSignal.any([controller.signal, lifetime.signal]);
  const register = createClientCommandRegistration(
    lifetime,
    signal,
    {
      installationId: "installed",
      digest: "a".repeat(64),
      componentId: "exports",
    },
    captureSessionId,
  );
  return { lifetime, register, signal, controller };
}

it("captures the invocation's Session synchronously and joins accepted work on withdrawal", async () => {
  let selected = "original";
  const { register, lifetime, signal } = await fixture(() => selected);
  const response = Promise.withResolvers<void>();
  let captured!: ClientCommandInvocation;
  register({
    id: "export",
    label: "Export",
    run(invocation) {
      captured = invocation;
      return response.promise;
    },
  });
  const command = contributionsTo(COMMAND)[0]!.item;
  const exporting = command.run();
  selected = "replacement";
  expect(captured.sessionId).toBe("original");
  expect(captured.signal).toBe(signal);
  let closed = false;
  const closing = lifetime[asyncDisposeSymbol]().then(() => {
    closed = true;
  });
  try {
    await vi.waitFor(() => expect(contributionsTo(COMMAND)).toHaveLength(0));
    expect(signal.aborted).toBe(true);
    expect(closed).toBe(false);
    await expect(command.run()).rejects.toBe(signal.reason);
  } finally {
    response.resolve();
    await Promise.all([exporting, closing]);
  }
  expect(notifyFrom).not.toHaveBeenCalled();
});

it("namespaces commands by installed component and removes only their own registration", async () => {
  const { register, lifetime, signal } = await fixture();
  const other = createClientCommandRegistration(
    lifetime,
    signal,
    {
      installationId: "other",
      digest: "b".repeat(64),
      componentId: "exports",
    },
    () => undefined,
  );
  const run = vi.fn();
  const stop = register({ id: "json", label: "Export JSON", combo: "Mod+Shift+E", run });
  other({ id: "json", label: "Other export", run });
  const [first, second] = contributionsTo(COMMAND).map((entry) => entry.item);
  expect(first!.id).not.toBe(second!.id);
  expect(first!.combo).toBe("Mod+Shift+E");
  stop();
  expect(contributionsTo(COMMAND).map((entry) => entry.item.label)).toEqual(["Other export"]);
  await expect(first!.run()).rejects.toThrow("withdrawn");
  expect(run).not.toHaveBeenCalled();
  register({ id: "json", label: "Export JSON again", run });
  expect(contributionsTo(COMMAND).map((entry) => entry.item.label)).toContain("Export JSON again");
  await expect(first!.run()).rejects.toThrow("withdrawn");
  expect(run).not.toHaveBeenCalled();
});

it("rejects invalid or duplicate local registrations without replacing a live command", async () => {
  const { register } = await fixture();
  const run = vi.fn();
  register({ id: "export", label: "Original", run });
  expect(() => register({ id: "export", label: "Replacement", run })).toThrow("already registered");
  expect(() => register({ id: "", label: "Invalid", run })).toThrow("must not be empty");
  expect(() => register({ id: "empty", label: " ", run })).toThrow("must not be empty");
  expect(contributionsTo(COMMAND).map((entry) => entry.item.label)).toEqual(["Original"]);
});

it("reports a synchronous or asynchronous command failure while preserving its cause", async () => {
  const { register } = await fixture();
  for (const asynchronous of [false, true]) {
    const failure = new Error(`export failed: ${asynchronous}`);
    register({
      id: asynchronous ? "async" : "sync",
      label: "Export",
      run() {
        if (asynchronous) return Promise.reject(failure);
        throw failure;
      },
    });
    const command = contributionsTo(COMMAND).at(-1)!.item;
    await expect(command.run()).rejects.toBe(failure);
    expect(usePluginErrorStore.getState().log.at(-1)).toMatchObject({
      source: "command",
      message: failure.message,
    });
    expect(notifyFrom).toHaveBeenLastCalledWith(
      "package:installed",
      `Export: ${failure.message}`,
      "error",
    );
  }
});

it("does not publish a retired command's late failure into the current client", async () => {
  const { register, controller } = await fixture();
  const response = Promise.withResolvers<void>();
  register({ id: "export", label: "Export", run: () => response.promise });
  const work = contributionsTo(COMMAND)[0]!.item.run();
  controller.abort();
  const failure = new Error("old native save failed");
  response.reject(failure);
  await expect(work).rejects.toBe(failure);
  expect(notifyFrom).not.toHaveBeenCalled();
  expect(usePluginErrorStore.getState().log).toEqual([]);
});
