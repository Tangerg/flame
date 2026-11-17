import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { createFlameClient } from "@flame/runtime-client";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import { type ComponentContext } from "@flame/component-sdk";
import { GenUIResult } from "../../../../plugins/genui/component/component";
import { Presentation } from "../../../../plugins/genui/contracts/document";
import { interfaceRequest } from "../../../../plugins/genui/contracts/fixture";
import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
afterEach(cleanup);
it("retains the official draft model outside a remounted row and dispatches only explicit captured context", async () => {
  const transport = createMemoryTransport();
  const runtime = createFlameClient(transport);
  const source = new AbortController();
  const component = { installationId: "renderer", digest: "a".repeat(64), componentId: "report" };
  const View = GenUIResult.create({ runtimeIdentity: runtime, component, signal: source.signal });
  const submit = vi.fn(async () => ({
    status: "accepted" as const,
    sessionId: "original-session",
    runId: "next-run",
    userItemId: "next-item",
  }));
  const owner = new AbortController();
  const context: ComponentContext = {
    component,
    runtime,
    signal: owner.signal,
    locale: "en",
    telemetry: { read: () => ({ spans: [], logs: [], metrics: [] }), clear() {} },
    toolResult: {
      id: "original-item",
      runId: "original-run",
      name: "ui_present",
      fn: "ui_present",
      args: "{}",
      status: "ok",
      result: Presentation.accept(interfaceRequest(true), []).json(),
    },
    interaction: { source: { runId: "original-run", itemId: "original-item" }, submit },
    track: (run) => run(owner.signal),
    notifications: unavailableComponentNotifications,
    saveFile: async () => "requested",
    subscribeRootRunSettlements() {
      throw new Error("unexpected observation");
    },
    registerCommand() {
      throw new Error("unexpected command");
    },
    compose: () => null,
  };
  try {
    const view = render(<View context={context} />);
    fireEvent.change(screen.getByRole("textbox", { name: "Review note" }), {
      target: { value: "local 9007199254740993" },
    });
    view.unmount();
    owner.abort();
    const next = new AbortController();
    render(
      <View context={{ ...context, signal: next.signal, track: (run) => run(next.signal) }} />,
    );
    await waitFor(() =>
      expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Review note" }).value).toBe(
        "local 9007199254740993",
      ),
    );
    expect(transport.outbox()).toEqual([]);
    expect(submit).not.toHaveBeenCalled();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Send review" })));
    expect(submit).toHaveBeenCalledExactlyOnceWith({
      action: "Review this",
      payloadJSON: '{"note":"local 9007199254740993"}',
    });
    next.abort();
  } finally {
    source.abort();
    await runtime.close();
  }
});
