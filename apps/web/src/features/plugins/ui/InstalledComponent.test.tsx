import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ComponentBoundary,
  InstalledComponent,
  InstalledToolResultComponent,
} from "@/features/plugins/ui/InstalledComponent";
import { type ComponentContext } from "@flame/component-sdk";
import { type FlameClient } from "@flame/runtime-client";
import { definePlugin, type ContributionLifetime } from "@/extensions/definePlugin";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { RuntimeConnectionGeneration } from "@/features/connection/public/services";
import { useEffect } from "react";
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

it("injects the captured interaction into an installed result without querying during render", async () => {
  let lifetime!: ContributionLifetime;
  const host = await startKernel([
    definePlugin({
      name: "test.captured-interaction",
      setup(context) {
        lifetime = context.lifetime("result");
      },
    }),
  ]);
  finishTestClient(() => stopKernel(host));
  let captured!: ComponentContext;
  function Result({ context }: { context: ComponentContext }) {
    useEffect(() => {
      captured = context;
    }, [context]);
    return <p>Captured result</p>;
  }
  const get = vi.fn(async () => ({ id: "run_original", sessionId: "ses_original" }));
  const send = vi.fn(async () => ({
    status: "accepted" as const,
    sessionId: "ses_original",
    runId: "run_followup",
    userItemId: "item_followup",
  }));
  testClient.agent.projection.store.getState().ensureSession("ses_original");
  testClient.agent.projection.store.getState().setSend("ses_original", send);
  vi.spyOn(testClient.agent.dependencies.runtime, "loadSessionSnapshot").mockResolvedValue({
    snapshot: { items: [], runs: [], pendingInterruptSets: [] },
    projectAssociatedSharedMaterial: (shared) => shared,
  });
  const mounted = testClient.render(
    <InstalledToolResultComponent
      entry={Result}
      component={{ installationId: "installed", digest: "a".repeat(64), componentId: "result" }}
      generation={RuntimeConnectionGeneration.forProcess("test")}
      compose={() => null}
      saveFile={async () => "requested"}
      runtime={{ runs: { get } } as unknown as FlameClient}
      binding={undefined}
      lifetime={lifetime}
      resources={new AbortController().signal}
      toolResult={{
        id: "item_original",
        runId: "run_original",
        name: "present",
        fn: "present",
        args: "{}",
        status: "ok",
        result: "{}",
      }}
    />,
  );
  expect(get).not.toHaveBeenCalled();
  expect(captured.interaction?.source).toEqual({ runId: "run_original", itemId: "item_original" });
  testClient.navigation.go({ session: "ses_elsewhere" });
  await act(async () => {
    await expect(
      captured.interaction!.submit({ action: "Continue", payloadJSON: "{}" }),
    ).resolves.toMatchObject({ status: "accepted", sessionId: "ses_original" });
  });
  expect(send).toHaveBeenCalledOnce();
  const original = captured.interaction!;
  mounted.unmount();
  expect(() => original.submit({ action: "Continue", payloadJSON: "{}" })).toThrow(/abort/i);
  expect(send).toHaveBeenCalledOnce();
});

it.each(["unmount", "resources", "lifetime"] as const)(
  "joins the original file save and refuses new saves after %s retirement",
  async (retirement) => {
    let lifetime!: ContributionLifetime;
    const host = await startKernel([
      definePlugin({
        name: "test.file-transfer",
        setup(context) {
          lifetime = context.lifetime("files");
        },
      }),
    ]);
    const choice = Promise.withResolvers<"saved">();
    finishTestClient(async () => {
      choice.resolve("saved");
      await stopKernel(host);
    });
    const saveFile = vi.fn<ComponentContext["saveFile"]>(() => choice.promise);
    const resources = new AbortController();
    let captured!: ComponentContext;
    function Export({ context }: { context: ComponentContext }) {
      useEffect(() => {
        captured = context;
      }, [context]);
      return <p>Captured export</p>;
    }
    const mounted = testClient.render(
      <InstalledComponent
        scope="installation"
        entry={Export}
        component={{ installationId: "installed", digest: "a".repeat(64), componentId: "export" }}
        generation={RuntimeConnectionGeneration.forProcess("file-transfer")}
        compose={() => null}
        saveFile={saveFile}
        runtime={{} as FlameClient}
        binding={undefined}
        lifetime={lifetime}
        resources={resources.signal}
      />,
    );
    const file = {
      filename: "report.pdf",
      mime: "application/pdf",
      bytes: new Uint8Array([0, 255]),
    };
    const saving = captured.saveFile(file);
    expect(saving).toBe(choice.promise);
    expect(saveFile).toHaveBeenCalledExactlyOnceWith(file);
    if (retirement === "unmount") mounted.unmount();
    else if (retirement === "resources") await act(async () => resources.abort());
    let closed = false;
    const closing = stopKernel(host).then(() => {
      closed = true;
    });
    await Promise.resolve();
    expect(captured.signal.aborted).toBe(true);
    expect(closed).toBe(false);
    expect(() => captured.saveFile(file)).toThrow(captured.signal.reason);
    expect(saveFile).toHaveBeenCalledOnce();
    choice.resolve("saved");
    await expect(saving).resolves.toBe("saved");
    await closing;
    expect(closed).toBe(true);
  },
);

it("keeps reload behind the original cleanup gate and exposes an unconfirmed exit", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const closing = Promise.withResolvers<void>();
  const reload = vi.fn(() => closing.promise);
  function Broken(): never {
    throw new Error("native render failed");
  }
  testClient.render(
    <ComponentBoundary scope="installed component" reload={reload}>
      <Broken />
    </ComponentBoundary>,
  );
  const button = screen.getByRole("button", { name: "Reload window" }) as HTMLButtonElement;
  fireEvent.click(button);
  expect(button.disabled).toBe(true);
  expect(reload).toHaveBeenCalledOnce();
  closing.reject(new Error("original resource release remains unconfirmed"));
  await waitFor(() => expect(button.disabled).toBe(false));
  const details = screen.getAllByRole("button", { name: "Details" });
  fireEvent.click(details[0]!);
  expect(screen.getByText(/scope: installed component reload/).textContent).toContain(
    "original resource release remains unconfirmed",
  );
});

it("retains the original component exception for inspection before reloading", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const error = new Error("schedule render evidence");
  function Broken(): never {
    throw error;
  }
  testClient.render(
    <ComponentBoundary scope="installed component">
      <Broken />
    </ComponentBoundary>,
  );
  expect(screen.getByRole("alert")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Details" }));
  expect(screen.getByText(/scope: installed component/).textContent).toContain(error.stack);
});

it("captures component identity, generation, time and React stack at the failure", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-17T00:00:00Z"));
  const scope = `installed component\ninstallation: schedule-installation\nrelease: ${"a".repeat(64)}\ncomponent: schedules\nruntime generation: original-process`;
  function BrokenSchedule(): never {
    throw new Error("original schedule exception");
  }
  const view = testClient.render(
    <ComponentBoundary scope={scope}>
      <BrokenSchedule />
    </ComponentBoundary>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Details" }));
  const diagnostics = screen.getByText(/scope: installed component/).textContent;
  expect(diagnostics).toContain("installation: schedule-installation");
  expect(diagnostics).toContain(`release: ${"a".repeat(64)}`);
  expect(diagnostics).toContain("component: schedules");
  expect(diagnostics).toContain("runtime generation: original-process");
  expect(diagnostics).toContain("time: 2026-10-17T00:00:00.000Z");
  expect(diagnostics).toContain("react component stack:");
  expect(diagnostics).toContain("BrokenSchedule");
  vi.setSystemTime(new Date("2026-10-17T01:00:00Z"));
  view.rerender(
    <ComponentBoundary scope="replacement component">
      <BrokenSchedule />
    </ComponentBoundary>,
  );
  expect(screen.getByText(/scope: installed component/).textContent).toBe(diagnostics);
});

it.each(["unmount", "resources", "contribution"] as const)(
  "opens the accepted Session through Host navigation and retires that action on %s",
  async (retirement) => {
    let lifetime!: ContributionLifetime;
    const host = await startKernel([
      definePlugin({
        name: "test.session-navigation",
        setup(context) {
          lifetime = context.lifetime("navigation");
        },
      }),
    ]);
    finishTestClient(() => stopKernel(host));
    const resources = new AbortController();
    let captured!: ComponentContext;
    function Receipt({ context }: { context: ComponentContext }) {
      useEffect(() => {
        captured = context;
      }, [context]);
      return (
        <button onClick={() => context.openSession!("accepted-session")}>Open conversation</button>
      );
    }
    testClient.navigation.go({ session: "source-session", view: "package:schedules" });
    const mounted = testClient.render(
      <InstalledComponent
        scope="installation"
        entry={Receipt}
        component={{ installationId: "installed", digest: "a".repeat(64), componentId: "receipt" }}
        generation={RuntimeConnectionGeneration.forProcess("test")}
        compose={() => null}
        saveFile={async () => "requested"}
        runtime={{} as FlameClient}
        binding={undefined}
        lifetime={lifetime}
        resources={resources.signal}
      />,
    );
    expect(testClient.agent.access.getActiveSessionId()).toBe("source-session");
    fireEvent.click(screen.getByRole("button", { name: "Open conversation" }));
    expect(testClient.agent.access.getActiveSessionId()).toBe("accepted-session");
    expect(testClient.navigation.get().view).toBeNull();
    const openSession = captured.openSession!;
    await act(async () => {
      if (retirement === "unmount") mounted.unmount();
      else if (retirement === "resources") resources.abort();
      else await stopKernel(host);
    });
    expect(captured.signal.aborted).toBe(true);
    expect(() => openSession("another-session")).toThrow(captured.signal.reason);
    expect(testClient.agent.access.getActiveSessionId()).toBe("accepted-session");
  },
);
