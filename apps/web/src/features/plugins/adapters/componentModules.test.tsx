import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { createElement, useEffect, useLayoutEffect, useRef, useState } from "react";
import { type Host } from "dougong";
import { type FlameClient, type MutationPromise } from "@flame/runtime-client";
import {
  type PluginBrowserSession,
  type PluginClientComponent,
  type PluginInstallation,
} from "@flame/runtime-client/wire";
import {
  defineComponent,
  type ComponentContext,
  type ComponentSource,
  type ComponentDefinition,
  type PluginComponent,
} from "@flame/component-sdk";
import { componentExport } from "@flame/component-sdk/runtime-profile";
import {
  PROTOCOL,
  type CloseProof,
  type LocalPlan,
  type ReadyReport,
  type UseGrant,
} from "@qimu/sdk/protocol";
import { definePlugin, type ContributionLifetime } from "@/extensions/definePlugin";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { captureClientTelemetry } from "@/lib/observability/clientTelemetry";
import { ComponentModules } from "@/features/plugins/adapters/componentModules";
import { ComponentBoundary, InstalledComponent } from "@/features/plugins/ui/InstalledComponent";
import { createTestClient } from "@/test/renderClient";
import { ContentVisibility } from "@flame/ui";
import { RuntimeConnectionGeneration } from "@/features/connection/public/services";

const component: PluginClientComponent = {
  id: "panel",
  title: "Panel",
  scope: "installation",
  placement: "export",
  entry: "flame/component.js",
  export: "Panel",
};
const installation: PluginInstallation = {
  id: "producer",
  source: "/isolated-test",
  enabled: true,
  inputStates: {},
  disabledServers: [],
  disabledSkills: [],
  presentation: "admitted",
  realization: { type: "available", unavailableBackends: [] },
  selected: {
    digest: "a".repeat(64),
    name: "producer",
    contributions: [],
    points: [],
    pointValues: [],
    servers: [],
    inputs: [],
    skills: [],
    locales: [],
    themes: [],
    diagnostics: [],
    components: [component],
  },
};
let kernel: Host;
let scope: ContributionLifetime;
const owners: ComponentModules[] = [];

function mutation<T>(
  create: () => Promise<T>,
  key: string = crypto.randomUUID(),
): MutationPromise<T> {
  return Object.assign(Promise.resolve().then(create), {
    idempotencyKey: key,
    retry: () => mutation(create, key),
  });
}

beforeEach(async () => {
  vi.spyOn(HTMLScriptElement, "supports").mockReturnValue(true);
  kernel = await startKernel([
    definePlugin({
      name: "test.component-modules",
      setup(ctx) {
        scope = ctx.lifetime("presentation");
      },
    }),
  ]);
});

afterEach(async () => {
  cleanup();
  for (const owner of owners.splice(0)) await owner.close();
  await stopKernel(kernel);
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function fixture(
  renderComponent: PluginComponent | ComponentDefinition = () =>
    createElement("p", null, "Native panel"),
  resourcesReady: Promise<void> = Promise.resolve(),
) {
  const modules = new ComponentModules();
  owners.push(modules);
  const globalName = `qimuModule${crypto.randomUUID().replaceAll("-", "")}`;
  const source = { evaluations: 0, render: renderComponent };
  vi.stubGlobal(globalName, source);
  const endpoint = `data:text/javascript,${encodeURIComponent(`globalThis["${globalName}"].evaluations++; export const Panel = globalThis["${globalName}"].render;`)}#`;
  const session: PluginBrowserSession = {
    authority: { id: "authority", epoch: "1" },
    realm: { id: "window", epoch: "1" },
    credential: "private-session",
    controlEndpoint: "https://runtime.test/v2/qimu",
    maximumMessageBytes: 1 << 20,
    proofCapacity: 64,
    pollIntervalMilliseconds: 1,
    componentContract: {
      name: "io.github.tangerg.flame/component",
      version: "1",
      digest: "b".repeat(64),
      kind: "service",
      mode: "native",
    },
  };
  const plan: LocalPlan = {
    protocol: PROTOCOL,
    approvalToken: "approved",
    authority: session.authority,
    realm: session.realm,
    execution: { id: "execution", generation: "1" },
    scopeId: "scope",
    scopeGeneration: "1",
    units: [
      {
        instance: {
          id: "instance",
          generation: "1",
          source: {
            installationId: installation.id,
            release: installation.selected.digest,
            unitId: "component/panel",
          },
          configuration: "c".repeat(64),
        },
        configurationJson: JSON.stringify({
          export: component.export,
          component: { id: component.id },
          sharedModules: [],
        }),
        profile: "browser-js",
        module: { path: component.entry, digest: "d".repeat(64) },
        imports: [],
        exports: [{ name: componentExport, contract: session.componentContract }],
      },
    ],
  };
  const prepared = Promise.withResolvers<ReadyReport>();
  const ended = Promise.withResolvers<void>();
  const proofs: CloseProof[] = [];
  const grants = new Map<string, UseGrant>();
  let ready: ReadyReport | undefined;
  let requested = false;
  let sealed = false;
  let loseProof = false;
  let cleanupOnly = false;
  const closeResources = vi.fn(async () => {
    ended.resolve();
    await ended.promise;
  });
  const openResources = vi.fn<FlameClient["plugins"]["openResources"]>(async () => {
    await resourcesReady;
    return {
      result: { token: "lease", entry: component.entry },
      closed: ended.promise,
      close: closeResources,
    };
  });
  const openBrowserRealm = vi.fn<FlameClient["plugins"]["openBrowserRealm"]>(() =>
    mutation(async () => session),
  );
  const acquireComponent = vi.fn<FlameClient["plugins"]["acquireComponent"]>((request) =>
    mutation(async () => {
      if (sealed || cleanupOnly) throw new Error("realm fenced");
      expect(request).toMatchObject({
        installationId: installation.id,
        digest: installation.selected.digest,
        componentId: component.id,
        realm: session.realm,
      });
      requested = true;
      const report = await prepared.promise;
      const grant: UseGrant = {
        id: request.operationId,
        approvalToken: `use:${request.operationId}`,
        authority: plan.authority,
        realm: plan.realm,
        execution: plan.execution,
        scopeId: plan.scopeId,
        scopeGeneration: plan.scopeGeneration,
        objects: report.objects,
      };
      grants.set(grant.id, grant);
      return structuredClone(grant) as Awaited<
        ReturnType<FlameClient["plugins"]["acquireComponent"]>
      >;
    }, request.operationId),
  );
  const closeBrowserRealm = vi.fn<FlameClient["plugins"]["closeBrowserRealm"]>(() =>
    mutation(async () => {
      sealed = true;
    }),
  );
  const client = {
    plugins: { openBrowserRealm, acquireComponent, closeBrowserRealm, openResources },
  } as unknown as FlameClient;
  const validatePlan = (input: unknown) => expect(input).toEqual(plan);
  const validateUse = (grant: UseGrant) => expect(grant).toEqual(grants.get(grant.id));
  const validateProof = (proof: CloseProof) => expect(proof.instances).toEqual(["instance"]);
  const fetchControl = vi.fn<typeof fetch>(async (url, init) => {
    expect(String(url)).toBe(session.controlEndpoint);
    expect(new Headers(init!.headers).get("authorization")).toBe(`Bearer ${session.credential}`);
    const message = JSON.parse(String(init!.body)) as { method: string; input: unknown };
    let response: unknown = null;
    switch (message.method) {
      case "poll":
        response = {
          authority: session.authority,
          realm: session.realm,
          revision: "1",
          plans: requested && !ready && !sealed ? [plan] : [],
          fences: sealed ? [{ scopeId: plan.scopeId, scopeGeneration: plan.scopeGeneration }] : [],
        };
        break;
      case "validatePlan":
        validatePlan(message.input);
        break;
      case "ready":
        ready = message.input as ReadyReport;
        prepared.resolve(ready);
        response = { decision: "accepted" };
        break;
      case "validateUse": {
        const grant = message.input as UseGrant;
        validateUse(grant);
        break;
      }
      case "prove": {
        const proof = message.input as CloseProof;
        validateProof(proof);
        proofs.push(proof);
        if (proof.scopeKind === "execution" && !ready) {
          requested = false;
          prepared.reject(new Error("component execution retired before publication"));
        }
        if (loseProof) {
          loseProof = false;
          throw new Error("proof acknowledgement lost");
        }
        response = {
          authority: session.authority,
          closeId: proof.closeId,
          proofId: proof.proofId,
          recorded: true,
        };
        break;
      }
      case "reconnect":
        response = {
          authority: session.authority,
          realm: session.realm,
          mode: cleanupOnly ? "cleanupOnly" : "revalidated",
          acceptedProofs: [],
          executions: cleanupOnly
            ? []
            : [
                {
                  execution: plan.execution,
                  scopeId: plan.scopeId,
                  scopeGeneration: plan.scopeGeneration,
                },
              ],
        };
        break;
      default:
        throw new Error(`unexpected control method: ${message.method}`);
    }
    return new Response(JSON.stringify(response), {
      headers: { "Content-Type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", fetchControl);
  const context: ComponentContext = {
    component: {
      installationId: installation.id,
      digest: installation.selected.digest,
      componentId: component.id,
    },
    runtime: client,
    telemetry: captureClientTelemetry(scope.signal),
    locale: "en",
    signal: scope.signal,
    track: (operation) => operation(scope.signal),
    saveFile: async () => "requested",
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("This fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose: () => null,
  };
  const load = (generation: object = originalGeneration, nextEndpoint = endpoint) =>
    modules.component(scope, client, nextEndpoint, generation, installation, component);
  const originalGeneration = {};
  return {
    modules,
    load,
    context,
    source,
    proofs,
    openResources,
    closeResources,
    openBrowserRealm,
    acquireComponent,
    closeBrowserRealm,
    loseProof() {
      loseProof = true;
    },
    replaceAuthority() {
      cleanupOnly = true;
      session.authority = { ...session.authority, epoch: "2" };
    },
  };
}

it("uses one approved execution and evaluation for two independently owned component Uses", async () => {
  const f = fixture();
  const loaded = await f.load();
  expect(f.openResources).not.toHaveBeenCalled();
  const Native = loaded.component;
  const scene = render(
    <>
      <Native context={f.context} />
      <Native context={f.context} />
    </>,
  );
  await screen.findAllByText("Native panel");
  await waitFor(() => expect(screen.getAllByText("Native panel")).toHaveLength(2));
  expect(f.acquireComponent).toHaveBeenCalledTimes(2);
  expect(f.openResources).toHaveBeenCalledOnce();
  expect(f.source.evaluations).toBe(1);
  scene.unmount();
  await waitFor(() =>
    expect(f.proofs.filter((proof) => proof.scopeKind === "use")).toHaveLength(2),
  );
  expect(f.closeResources).not.toHaveBeenCalled();
  await f.modules.close();
  expect(f.closeResources).toHaveBeenCalledOnce();
  expect(f.proofs.some((proof) => proof.scopeKind === "execution")).toBe(true);
});

it("constructs one explicit source owner across view closes and clears it when its resources close", async () => {
  let source!: ComponentSource;
  let retained = "";
  const signals: AbortSignal[] = [];
  const create = vi.fn((captured: ComponentSource): PluginComponent => {
    source = captured;
    source.signal.addEventListener(
      "abort",
      () => {
        retained = "";
      },
      { once: true },
    );
    return function DraftView({ context }) {
      const [draft, setDraft] = useState(retained);
      useEffect(() => {
        signals.push(context.signal);
      }, [context.signal]);
      return createElement("input", {
        "aria-label": "Source draft",
        value: draft,
        onChange: (event) => {
          retained = event.currentTarget.value;
          setDraft(retained);
        },
      });
    };
  });
  const f = fixture(defineComponent(create));
  const Native = (await f.load()).component;
  expect(create).not.toHaveBeenCalled();
  const first = render(<Native context={f.context} />);
  const draft = await screen.findByRole("textbox", { name: "Source draft" });
  fireEvent.change(draft, { target: { value: "private-source-draft" } });
  first.unmount();
  await waitFor(() => expect(signals[0]?.aborted).toBe(true));
  expect(source.signal.aborted).toBe(false);
  expect(retained).toBe("private-source-draft");
  const reopened = render(<Native context={{ ...f.context, locale: "zh" }} />);
  expect(await screen.findByRole("textbox", { name: "Source draft" })).toHaveProperty(
    "value",
    "private-source-draft",
  );
  expect(create).toHaveBeenCalledOnce();
  expect(source.runtimeIdentity).toBe(f.context.runtime);
  expect(source.component).toEqual(f.context.component);
  reopened.unmount();
  await f.closeResources();
  await waitFor(() => expect(source.signal.aborted).toBe(true));
  expect(scope.signal.aborted).toBe(false);
  expect(retained).toBe("");
});

it("retains the current reader when its independently mounted component fails", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const renderFailure = new Error("captured result reader failed");
  function NativeReader() {
    const [failed, setFailed] = useState(false);
    if (failed) throw renderFailure;
    return <button onClick={() => setFailed(true)}>Read another section</button>;
  }
  const f = fixture(NativeReader);
  const Native = (await f.load()).component;
  const client = createTestClient();
  try {
    client.render(
      <div role="group" aria-label="Captured result" tabIndex={-1}>
        <ComponentBoundary
          scope="captured tool result"
          fallback={<p>Recorded result stays readable.</p>}
          reload={async () => {}}
        >
          <Native context={f.context} />
        </ComponentBoundary>
      </div>,
    );
    const reader = await screen.findByRole("button", { name: "Read another section" });
    reader.focus();
    expect(document.activeElement).toBe(reader);

    // act queues root errors instead of delivering Qimu's onUncaughtError callback.
    reader.click();

    await screen.findByRole("alert");
    expect(reader.isConnected).toBe(false);
    expect(screen.getByText("Recorded result stays readable.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Reload window" })).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByRole("group", { name: "Captured result" }));
    fireEvent.click(screen.getByRole("button", { name: "Details" }));
    expect(screen.getByText(/scope: captured tool result/).textContent).toContain(
      renderFailure.message,
    );
    await waitFor(() => expect(f.proofs.some((proof) => proof.scopeKind === "use")).toBe(true));
  } finally {
    await client.dispose();
  }
});

it.each(["owned", "outside", "blurred", "hidden", "inert", "aria-hidden", "visibility", "no-host"])(
  "keeps native failure focus with its current owner through %s",
  async (mode) => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const read = Promise.withResolvers<void>();
    function NativeReader() {
      const [failed, setFailed] = useState(false);
      useEffect(() => {
        void read.promise.then(() => setFailed(true));
      }, []);
      if (failed) throw new Error("captured result reader failed");
      return <button type="button">Read another section</button>;
    }
    const f = fixture(NativeReader);
    const Native = (await f.load()).component;
    const client = createTestClient();
    const scene = (retired: boolean) => (
      <>
        <input aria-label="Later draft" defaultValue="Continue reading — 保留" />
        <ContentVisibility visible={mode !== "visibility" || !retired}>
          <div
            role="group"
            aria-label="Captured result"
            tabIndex={mode === "no-host" ? undefined : -1}
            hidden={mode === "hidden" && retired}
            inert={mode === "inert" && retired}
            aria-hidden={mode === "aria-hidden" && retired ? true : undefined}
          >
            <ComponentBoundary scope="captured tool result" fallback={<p>Recorded source</p>}>
              <Native context={f.context} />
            </ComponentBoundary>
          </div>
        </ContentVisibility>
      </>
    );
    try {
      const mounted = client.render(scene(false));
      const host = screen.getByRole("group", { name: "Captured result" });
      const draft = screen.getByRole("textbox", { name: "Later draft" }) as HTMLInputElement;
      const reader = await screen.findByRole("button", { name: "Read another section" });
      reader.focus();
      draft.setSelectionRange(8, 15);
      if (mode === "outside") draft.focus();
      if (mode === "blurred") reader.blur();
      mounted.rerender(scene(true));

      read.resolve();

      await screen.findByText("Recorded source");
      const expected = mode === "owned" ? host : mode === "outside" ? draft : document.body;
      expect(reader.isConnected).toBe(false);
      expect(document.activeElement).toBe(expected);
      expect(draft.value).toBe("Continue reading — 保留");
      expect([draft.selectionStart, draft.selectionEnd]).toEqual([8, 15]);
      mounted.rerender(scene(false));
      expect(document.activeElement).toBe(expected);
      expect(f.acquireComponent).toHaveBeenCalledOnce();
      await waitFor(() => expect(f.proofs.some((proof) => proof.scopeKind === "use")).toBe(true));
    } finally {
      await client.dispose();
    }
  },
);

it("retires focus when an inner update already removed the control", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const read = Promise.withResolvers<void>();
  function NativeReader() {
    const [stage, setStage] = useState(0);
    useEffect(() => {
      void read.promise.then(() => setStage(2));
    }, []);
    if (stage === 2) throw new Error("later native result failure");
    return stage === 0 ? (
      <button onClick={() => setStage(1)}>Continue reading</button>
    ) : (
      <p>Another native section</p>
    );
  }
  const f = fixture(NativeReader);
  const Native = (await f.load()).component;
  const client = createTestClient();
  try {
    client.render(
      <>
        <input aria-label="Later draft" />
        <div role="group" aria-label="Captured result" tabIndex={-1}>
          <ComponentBoundary scope="captured tool result" fallback={<p>Recorded source</p>}>
            <Native context={f.context} />
          </ComponentBoundary>
        </div>
      </>,
    );
    const reader = await screen.findByRole("button", { name: "Continue reading" });
    reader.focus();
    reader.click();
    await screen.findByText("Another native section");
    const draft = screen.getByRole("textbox", { name: "Later draft" });
    draft.focus();
    draft.blur();
    expect(document.activeElement).toBe(document.body);

    read.resolve();

    await screen.findByText("Recorded source");
    expect(document.activeElement).toBe(document.body);
    expect(f.acquireComponent).toHaveBeenCalledOnce();
  } finally {
    await client.dispose();
  }
});

it.each([
  "owned",
  "outside",
  "blurred",
  "hidden",
  "inert",
  "aria-hidden",
  "visibility",
  "no-host",
  "unmount",
  "replacement",
  "fallback",
])("keeps native resource retirement focus with its current owner through %s", async (mode) => {
  const f = fixture(() => <button>Read captured result</button>);
  const Native = (await f.load()).component;
  const client = createTestClient();
  const resources = new AbortController();
  const replacement = new AbortController();
  function RecordedAction() {
    const action = useRef<HTMLButtonElement>(null);
    useLayoutEffect(() => action.current?.focus(), []);
    return <button ref={action}>Recorded result action</button>;
  }
  const sources = {
    entry: Native,
    component: f.context.component,
    generation: RuntimeConnectionGeneration.forProcess("captured-reader"),
    compose: () => null,
    saveFile: f.context.saveFile,
    runtime: f.context.runtime,
    binding: undefined,
    lifetime: scope,
    fallback: (
      <>
        <p>Recorded source remains available</p>
        {mode === "fallback" && <RecordedAction />}
      </>
    ),
  };
  const scene = (retired: boolean) => (
    <>
      <input aria-label="Later draft" defaultValue="Continue reading — 保留" />
      <ContentVisibility visible={mode !== "visibility" || !retired}>
        <div
          role="group"
          aria-label="Captured result"
          tabIndex={mode === "no-host" ? undefined : -1}
          hidden={mode === "hidden" && retired}
          inert={mode === "inert" && retired}
          aria-hidden={mode === "aria-hidden" && retired ? true : undefined}
        >
          {(mode !== "unmount" || !retired) && (
            <InstalledComponent
              scope="installation"
              {...sources}
              resources={mode === "replacement" && retired ? replacement.signal : resources.signal}
            />
          )}
        </div>
      </ContentVisibility>
    </>
  );
  try {
    const mounted = client.render(scene(false));
    const host = screen.getByRole("group", { name: "Captured result" });
    const draft = screen.getByRole("textbox", { name: "Later draft" }) as HTMLInputElement;
    const reader = await screen.findByRole("button", { name: "Read captured result" });
    reader.focus();
    draft.setSelectionRange(8, 15);
    if (mode === "outside") draft.focus();
    if (mode === "blurred") reader.blur();
    mounted.rerender(scene(true));

    await act(() => resources.abort());

    const remainsMounted = mode === "replacement";
    const unavailable = !remainsMounted && mode !== "unmount";
    if (unavailable) await screen.findByText("Recorded source remains available");
    expect(reader.isConnected).toBe(remainsMounted);
    expect(screen.queryByRole("alert", { hidden: true })?.textContent ?? null).toBe(
      unavailable ? "The plugin is unavailable." : null,
    );
    const expected =
      mode === "owned"
        ? host
        : mode === "outside"
          ? draft
          : mode === "fallback"
            ? screen.getByRole("button", { name: "Recorded result action" })
            : remainsMounted
              ? reader
              : document.body;
    expect(document.activeElement).toBe(expected);
    if (unavailable) mounted.rerender(scene(false));
    expect(document.activeElement).toBe(expected);
    await waitFor(() =>
      expect(f.proofs.some((proof) => proof.scopeKind === "use")).toBe(!remainsMounted),
    );
    expect(draft.value).toBe("Continue reading — 保留");
    expect([draft.selectionStart, draft.selectionEnd]).toEqual([8, 15]);
    expect(f.acquireComponent).toHaveBeenCalledOnce();
  } finally {
    await client.dispose();
  }
});

it("leaves newly mounted fallback focus with the fallback", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  function NativeReader() {
    const [failed, setFailed] = useState(false);
    if (failed) throw new Error("captured result reader failed");
    return <button onClick={() => setFailed(true)}>Read another section</button>;
  }
  function RecordedAction() {
    const action = useRef<HTMLButtonElement>(null);
    useLayoutEffect(() => action.current?.focus(), []);
    return <button ref={action}>Recorded result action</button>;
  }
  const f = fixture(NativeReader);
  const Native = (await f.load()).component;
  const client = createTestClient();
  try {
    client.render(
      <div role="group" aria-label="Captured result" tabIndex={-1}>
        <ComponentBoundary scope="captured tool result" fallback={<RecordedAction />}>
          <Native context={f.context} />
        </ComponentBoundary>
      </div>,
    );
    const reader = await screen.findByRole("button", { name: "Read another section" });
    reader.focus();
    reader.click();

    const recorded = await screen.findByRole("button", { name: "Recorded result action" });
    expect(document.activeElement).toBe(recorded);
  } finally {
    await client.dispose();
  }
});

it("leaves normal native unmount focus to its navigation owner", async () => {
  const f = fixture(() => <button>Native result action</button>);
  const Native = (await f.load()).component;
  const scene = (shown: boolean) => (
    <div role="group" aria-label="Captured result" tabIndex={-1}>
      {shown && <Native context={f.context} />}
    </div>
  );
  const mounted = render(scene(true));
  const action = await screen.findByRole("button", { name: "Native result action" });
  action.focus();

  mounted.rerender(scene(false));

  expect(action.isConnected).toBe(false);
  expect(document.activeElement).toBe(document.body);
  await waitFor(() => expect(f.proofs.some((proof) => proof.scopeKind === "use")).toBe(true));
});

it("unmounts the native root but keeps tracked component work owned until it settles", async () => {
  const work = Promise.withResolvers<void>();
  let tracked: Promise<void> | undefined;
  let context: ComponentContext | undefined;
  let unmounted = false;
  const f = fixture((props) => {
    useEffect(() => {
      context = props.context;
      tracked = context.track(() => work.promise);
      return () => {
        unmounted = true;
      };
    }, [props.context]);
    return createElement("p", null, "Working panel");
  });
  const Native = (await f.load()).component;
  const scene = render(<Native context={f.context} />);
  try {
    await screen.findByText("Working panel");
    await waitFor(() => expect(tracked).toBe(work.promise));
    scene.unmount();
    await waitFor(() => expect(unmounted).toBe(true));
    expect(context!.signal.aborted).toBe(true);
    const startAfterClose = vi.fn(() => Promise.resolve());
    expect(() => context!.track(startAfterClose)).toThrow("lifetime is closing");
    expect(startAfterClose).not.toHaveBeenCalled();
    expect(f.proofs.filter((proof) => proof.scopeKind === "use")).toHaveLength(0);
    let closed = false;
    const closing = f.modules.close().then(() => {
      closed = true;
    });
    await Promise.resolve();
    expect(closed).toBe(false);
    work.resolve();
    await closing;
    expect(f.proofs.some((proof) => proof.scopeKind === "use")).toBe(true);
  } finally {
    work.resolve();
  }
});

it("retains a component Use until its native file save settles", async () => {
  const choice = Promise.withResolvers<"saved">();
  let context: ComponentContext | undefined;
  let saving: Promise<"requested" | "saved" | "cancelled"> | undefined;
  const file = { filename: "report.pdf", mime: "application/pdf", bytes: new Uint8Array([0, 255]) };
  const f = fixture((props) => {
    useEffect(() => {
      context = props.context;
      saving = context.saveFile(file);
    }, [props.context]);
    return createElement("p", null, "Saving panel");
  });
  const saveFile = vi.fn(() => choice.promise);
  const Native = (await f.load()).component;
  const scene = render(<Native context={{ ...f.context, saveFile }} />);
  try {
    await screen.findByText("Saving panel");
    await waitFor(() => expect(saving).toBe(choice.promise));
    scene.unmount();
    await waitFor(() => expect(context!.signal.aborted).toBe(true));
    expect(() => context!.saveFile(file)).toThrow(context!.signal.reason);
    expect(saveFile).toHaveBeenCalledExactlyOnceWith(file);
    expect(f.proofs.filter((proof) => proof.scopeKind === "use")).toHaveLength(0);
    let closed = false;
    const closing = f.modules.close().then(() => {
      closed = true;
    });
    await Promise.resolve();
    expect(closed).toBe(false);
    choice.resolve("saved");
    await closing;
    await expect(saving).resolves.toBe("saved");
    expect(f.proofs.some((proof) => proof.scopeKind === "use")).toBe(true);
  } finally {
    choice.resolve("saved");
  }
});

it("retains the original exit proof after lost acknowledgement and does not seal a second Realm", async () => {
  const f = fixture();
  const Native = (await f.load()).component;
  const scene = render(<Native context={f.context} />);
  await screen.findByText("Native panel");
  scene.unmount();
  await waitFor(() => expect(f.proofs.some((proof) => proof.scopeKind === "use")).toBe(true));
  f.loseProof();
  await expect(f.modules.close()).rejects.toMatchObject({ code: "outcome_unknown" });
  const original = f.proofs.at(-1)!;
  await f.modules.close();
  expect(f.proofs.filter((proof) => proof.proofId === original.proofId)).toEqual([
    original,
    original,
  ]);
  expect(f.closeBrowserRealm).toHaveBeenCalledOnce();
  expect(f.closeResources).toHaveBeenCalledOnce();
});

it("reconnects the original Realm as cleanup-only after authority replacement", async () => {
  const f = fixture();
  const Native = (await f.load()).component;
  const scene = render(<Native context={f.context} />);
  await screen.findByText("Native panel");
  scene.unmount();
  f.modules.disconnect();
  f.replaceAuthority();
  await expect(f.load({})).rejects.toMatchObject({ code: "reload_required" });
  const calls = f.openBrowserRealm.mock.calls;
  expect(calls[1]![0]).toMatchObject({
    operationId: calls[0]![0].operationId,
    resumeToken: calls[0]![0].resumeToken,
    resume: { id: "window", epoch: "1" },
  });
  expect(f.acquireComponent).toHaveBeenCalledOnce();
  expect(f.source.evaluations).toBe(1);
  await act(() => f.modules.close());
});

it("retains the artifact origin captured before a pending lease crosses reconnection", async () => {
  const resourcesReady = Promise.withResolvers<void>();
  const f = fixture(undefined, resourcesReady.promise);
  const Native = (await f.load()).component;
  const scene = render(<Native context={f.context} />);
  await waitFor(() => expect(f.openResources).toHaveBeenCalledOnce());
  const replacement = f.load(
    {},
    `data:text/javascript,${encodeURIComponent('throw new Error("resource crossed Runtime origin")')}#`,
  );
  scene.unmount();
  resourcesReady.resolve();
  await replacement;
  expect(f.source.evaluations).toBe(1);
  expect(f.closeResources).toHaveBeenCalledOnce();
  expect(f.proofs.some((proof) => proof.scopeKind === "execution")).toBe(true);
});
