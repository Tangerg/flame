import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { type Host } from "dougong";
import { StrictMode, useEffect, useState, type ComponentProps } from "react";
import { type ComponentContext, type PluginComponent } from "@flame/component-sdk";
import { type FlameClient } from "@flame/runtime-client";
import { type PluginInstallation, type PluginPointBinding } from "@flame/runtime-client/wire";
import { definePlugin } from "@/extensions/definePlugin";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { queryClient } from "@/lib/queryClient";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { usePluginErrorStore } from "@/extensions/errors";
import { PACKAGES_KEY } from "@/features/plugins/adapters/packages";
import { ComponentModules } from "@/features/plugins/adapters/componentModules";
import { ComponentCompositions } from "@/features/plugins/ui/ComponentCompositions";
import { RenderingComponent } from "@/features/plugins/ui/InstalledComponent";
import { RuntimeConnectionGeneration } from "@/features/connection/public/services";
let host: Host;

let lifetime: ContributionLifetime;

let installations: PluginInstallation[];

let values: PluginPointBinding[];

let contexts: ComponentContext[];

let composeHost: ComponentCompositions;

let parents: ComponentContext[];

const resolvePoint = vi.fn();

const modules = new ComponentModules();

const resources = new AbortController();

const consumer = installation("consumer");

const producer = installation("producer");

const root = { installationId: consumer.id, digest: consumer.selected.digest, componentId: "root" };

const client = { plugins: { resolvePoint } } as unknown as FlameClient;

const generation = RuntimeConnectionGeneration.forProcess("composition-test");

function installation(name: string, digest = "a".repeat(64)): PluginInstallation {
  return {
    id: name,
    source: "/isolated-test",
    enabled: true,
    inputStates: {},
    disabledServers: [],
    disabledSkills: [],
    realization: { type: "available", unavailableBackends: [] },
    presentation: "admitted",
    selected: {
      digest,
      name,
      contributions: [],
      points: [],
      pointValues: [],
      servers: [],
      inputs: [],
      skills: [],
      themes: [],
      diagnostics: [],
      components: [
        {
          id: "panel",
          title: "Panel",
          scope: "installation",
          placement: "export",
          entry: "component.js",
          export: "Panel",
        },
      ],
    },
  };
}

function value(
  contribution = "panel",
  props = "Extension",
  digest = producer.selected.digest,
): PluginPointBinding {
  return {
    installationId: producer.id,
    digest,
    contribution,
    value: JSON.stringify({ component: "panel", props }),
  };
}

function Panel({ context, input, contribution, children }: ComponentProps<PluginComponent>) {
  contexts.push(context);
  const [clicks, setClicks] = useState(0);
  return (
    <section aria-label={String(contribution)}>
      <button onClick={() => setClicks(clicks + 1)}>
        {String(contribution)}:{String(input)}:{clicks}
      </button>
      {children}
    </section>
  );
}

function Root({ context }: { context: ComponentContext }) {
  parents.push(context);
  return context.compose({
    contract: JSON.stringify({ composition: "collection" }),
    target: { scope: "installation", reference: context.component.installationId },
    attributes: {},
    input: "inspected",
    children: <p>Consumer content</p>,
  });
}

async function publish(rows: PluginInstallation[], bindings: PluginPointBinding[]) {
  values = bindings;
  installations = rows;
  await act(async () => {
    queryClient.setQueryData([PACKAGES_KEY], rows);
  });
}

function mount(entry: PluginComponent = Root, strict = false) {
  const scene = (
    <QueryClientProvider client={queryClient}>
      <RenderingComponent
        entry={entry}
        component={root}
        generation={generation}
        compose={composeHost.renderer(lifetime, [root])}
        saveFile={async () => "requested"}
        runtime={client}
        binding={undefined}
        lifetime={lifetime}
        resources={resources.signal}
        sessionId="captured-session"
        workspace={{ ref: { path: "/captured" } }}
      />
    </QueryClientProvider>
  );
  return testClient.render(strict ? <StrictMode>{scene}</StrictMode> : scene);
}

beforeEach(async () => {
  contexts = [];
  parents = [];
  installations = [consumer, producer];
  values = [value()];
  resolvePoint.mockImplementation(async () => ({ data: values }));
  vi.spyOn(modules, "component").mockImplementation(async () => ({
    component: Panel,
    signal: resources.signal,
    reload: vi.fn(async () => {}),
  }));
  host = await startKernel([
    definePlugin({
      name: "test.composition",
      setup(ctx) {
        lifetime = ctx.lifetime("installed-consumer");
        ctx.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher: async () => installations });
      },
    }),
  ]);
  queryClient.setQueryData([PACKAGES_KEY], installations);
  composeHost = new ComponentCompositions(client, modules, "http://isolated", generation, lifetime);
});

afterEach(async () => {
  cleanup();
  await stopKernel(host);
  queryClient.clear();
  vi.restoreAllMocks();
  resolvePoint.mockReset();
  usePluginErrorStore.getState().clearAll();
});

it("composes after StrictMode replays mount effects and retires the final captured contexts", async () => {
  const scene = mount(Root, true);
  await screen.findByRole("button", { name: "Extension:inspected:0" });
  const parent = parents.at(-1)!;
  const child = contexts.at(-1)!;
  expect(parent.signal.aborted).toBe(false);
  expect(child.signal.aborted).toBe(false);
  scene.unmount();
  await waitFor(() => expect(child.signal.aborted).toBe(true));
  expect(parent.signal.aborted).toBe(true);
});

it("opens a composition after its parent has committed under StrictMode", async () => {
  function Scene({ context }: { context: ComponentContext }) {
    const [open, setOpen] = useState(false);
    return (
      <>
        <button onClick={() => setOpen(true)}>Open composition</button>
        {open && <Root context={context} />}
      </>
    );
  }
  mount(Scene, true);
  fireEvent.click(screen.getByRole("button", { name: "Open composition" }));
  expect(await screen.findByRole("button", { name: "Extension:inspected:0" })).toBeTruthy();
});

it.each(["consumer", "producer"])(
  "composes with %s installed first and uses the contributor's own identity",
  async (first) => {
    await publish(first === "consumer" ? [consumer] : [producer], []);
    if (first === "consumer") {
      mount();
      await screen.findByText("Consumer content");
    }
    await publish([consumer, producer], [value()]);
    if (first === "producer") mount();
    await screen.findByRole("button", { name: "Extension:inspected:0" });
    expect(contexts.at(-1)?.component.installationId).toBe(producer.id);
    expect(contexts.at(-1)?.binding).toBeUndefined();
    expect(contexts.at(-1)?.workspace?.ref.path).toBe("/captured");
    expect(contexts.at(-1)?.sessionId).toBe("captured-session");
    expect(resolvePoint).toHaveBeenLastCalledWith(
      expect.objectContaining({ target: { scope: "installation", reference: consumer.id } }),
      expect.any(AbortSignal),
    );
  },
);

it("withdraws disabled/uninstalled contributors, supports reenablement and replaces an upgraded release", async () => {
  mount();
  await screen.findByRole("button", { name: "Extension:inspected:0" });
  const initial = contexts.at(-1)!;
  await publish([consumer, { ...producer, enabled: false, presentation: "withheld" }], []);
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "Extension:inspected:0" })).toBeNull(),
  );
  expect(initial.signal.aborted).toBe(true);
  await publish([consumer, producer], [value()]);
  await screen.findByRole("button", { name: "Extension:inspected:0" });
  const reenabling = contexts.at(-1)!;
  const upgraded = installation("producer", "b".repeat(64));
  await publish([consumer, upgraded], [value("panel", "Upgraded", upgraded.selected.digest)]);
  await screen.findByRole("button", { name: "Upgraded:inspected:0" });
  expect(reenabling.signal.aborted).toBe(true);
  expect(contexts.at(-1)?.component.digest).toBe(upgraded.selected.digest);
  const replaced = contexts.at(-1)!;
  await publish([consumer], []);
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "Upgraded:inspected:0" })).toBeNull(),
  );
  expect(replaced.signal.aborted).toBe(true);
  expect(screen.getByText("Consumer content")).toBeTruthy();
});

it("preserves component state and captured context when staging or unrelated installations leave bindings unchanged", async () => {
  mount();
  fireEvent.click(await screen.findByRole("button", { name: "Extension:inspected:0" }));
  const captured = contexts.at(-1)!;
  const calls = resolvePoint.mock.calls.length;
  await publish(
    [
      consumer,
      { ...producer, staged: { ...producer.selected, digest: "c".repeat(64) } },
      installation("unrelated"),
    ],
    [value()],
  );
  await waitFor(() => expect(resolvePoint.mock.calls.length).toBeGreaterThan(calls));
  expect(screen.getByRole("button", { name: "Extension:inspected:1" })).toBeTruthy();
  expect(contexts.at(-1)).toBe(captured);
  expect(captured.signal.aborted).toBe(false);
});

it("retires a two-way nested graph without registering cleanup in its disposed parent", async () => {
  resolvePoint.mockImplementation(async (request: { target: { reference: string } }) => ({
    data:
      request.target.reference === producer.id
        ? [
            {
              ...value("inspector", "Nested inspector", consumer.selected.digest),
              installationId: consumer.id,
            },
          ]
        : values,
  }));
  vi.mocked(modules.component).mockImplementation(
    async (_scope, _client, _endpoint, _generation, installation) => ({
      component:
        installation.id === consumer.id
          ? Panel
          : ({ context }) =>
              context.compose({
                contract: '{"composition":"collection"}',
                target: { scope: "installation", reference: producer.id },
                attributes: {},
                input: "nested",
              }),
      signal: resources.signal,
      reload: vi.fn(async () => {}),
    }),
  );
  mount();
  await screen.findByRole("button", { name: "Nested inspector:nested:0" });
  const nested = contexts.at(-1)!;
  await publish([consumer], []);
  await screen.findByText("Consumer content");
  expect(nested.signal.aborted).toBe(true);
  expect(parents.at(-1)?.signal.aborted).toBe(false);
  expect(screen.queryByRole("alert")).toBeNull();
  expect(usePluginErrorStore.getState().log).toEqual([]);
});

it.each(["selection", "decoration", "collection"])(
  "executes the Runtime's %s composition with exact order and props",
  async (composition) => {
    values =
      composition === "selection"
        ? [value("chosen", "Chosen")]
        : [value("outer", "Outer"), value("inner", "Inner")];
    mount(({ context }) =>
      context.compose({
        contract: JSON.stringify({ composition }),
        target: { scope: "installation", reference: consumer.id },
        attributes: {},
        order: [{ installationId: producer.id, contribution: "chosen" }],
        children: <p>Body</p>,
      }),
    );
    await screen.findByRole("region", { name: composition === "selection" ? "Chosen" : "Outer" });
    const outer = screen.queryByRole("region", { name: "Outer" });
    const inner = screen.queryByRole("region", { name: "Inner" });
    expect(Boolean(outer?.contains(inner))).toBe(composition === "decoration");
    expect(screen.queryByText("Body") !== null).toBe(composition !== "selection");
  },
);

it("reports a failed resolution and recovers when installation facts change", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  resolvePoint.mockRejectedValueOnce(new Error("required contributor is unavailable"));
  mount();
  await screen.findByRole("alert");
  expect(screen.queryByText("Consumer content")).toBeNull();
  await publish([consumer, producer, installation("new-peer")], [value()]);
  await screen.findByRole("button", { name: "Extension:inspected:0" });
});

it("refuses stale releases, missing exports and failed module loading without rendering partial success", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  values = [value("old", "Old", "f".repeat(64))];
  mount();
  await screen.findByRole("alert");
  expect(modules.component).not.toHaveBeenCalled();
  await publish([consumer, producer], [{ ...value(), value: '{"component":"missing"}' }]);
  await screen.findByRole("alert");
  expect(modules.component).not.toHaveBeenCalled();
  vi.mocked(modules.component).mockRejectedValueOnce(new Error("invalid module"));
  await publish([consumer, producer], [value()]);
  await screen.findByRole("alert");
  expect(screen.queryByRole("button", { name: "Extension:inspected:0" })).toBeNull();
});

it("retires observation of an unbounded import and never publishes late exports", async () => {
  const pending = Promise.withResolvers<Awaited<ReturnType<ComponentModules["component"]>>>();
  vi.mocked(modules.component).mockReturnValueOnce(pending.promise);
  mount();
  await waitFor(() => expect(modules.component).toHaveBeenCalledOnce());
  await publish([consumer], []);
  await screen.findByText("Consumer content");
  await act(async () =>
    pending.resolve({ component: Panel, signal: resources.signal, reload: vi.fn(async () => {}) }),
  );
  expect(screen.queryByRole("button", { name: "Extension:inspected:0" })).toBeNull();
});

it("cancels and joins a contributor's tracked reads before publishing the replacement composition", async () => {
  const release = Promise.withResolvers<void>();
  const cancelled = vi.fn();
  vi.mocked(modules.component).mockResolvedValue({
    component: function PendingRead({ context }) {
      useEffect(() => {
        const retire = () => cancelled();
        context.signal.addEventListener("abort", retire, { once: true });
        void context.track(() => release.promise);
        return () => context.signal.removeEventListener("abort", retire);
      }, [context]);
      return <p>Pending contributor read</p>;
    },
    signal: resources.signal,
    reload: vi.fn(async () => {}),
  });
  try {
    mount();
    await screen.findByText("Pending contributor read");
    await publish([consumer], []);
    await waitFor(() => expect(cancelled).toHaveBeenCalledOnce());
    expect(screen.queryByText("Consumer content")).toBeNull();
    await act(async () => release.resolve());
    await screen.findByText("Consumer content");
  } finally {
    release.resolve();
  }
});

it("joins the previous render owner when the consumer changes its composition context", async () => {
  const release = Promise.withResolvers<void>();
  vi.mocked(modules.component).mockResolvedValue({
    component: function PendingContext({ context }) {
      contexts.push(context);
      useEffect(() => {
        void context.track(() => release.promise);
      }, [context]);
      return <p>Contextual contribution</p>;
    },
    signal: resources.signal,
    reload: vi.fn(async () => {}),
  });
  function Scene({ context }: { context: ComponentContext }) {
    const [mode, setMode] = useState(0);
    return (
      <>
        <button onClick={() => setMode(mode + 1)}>Change context</button>
        {context.compose({
          contract: '{"composition":"collection"}',
          target: { scope: "installation", reference: consumer.id },
          attributes: { mode },
        })}
      </>
    );
  }
  try {
    mount(Scene);
    await screen.findByText("Contextual contribution");
    const previous = contexts.at(-1)!;
    fireEvent.click(screen.getByRole("button", { name: "Change context" }));
    await waitFor(() => expect(previous.signal.aborted).toBe(true));
    expect(screen.queryByText("Contextual contribution")).toBeNull();
    expect(resolvePoint).toHaveBeenCalledOnce();
    await act(async () => release.resolve());
    await screen.findByText("Contextual contribution");
    expect(contexts.at(-1)).not.toBe(previous);
  } finally {
    release.resolve();
  }
});

it("binds callable exports to their own contribution instead of the consumer", async () => {
  const provider = {
    ...producer,
    selected: {
      ...producer.selected,
      contributions: [
        {
          id: "operations",
          server: "backend",
          export: "operations",
          contract: "provider-contract",
          background: false,
          requires: [],
        },
      ],
      components: producer.selected.components.map((entry) => ({
        ...entry,
        contribution: "operations",
      })),
    },
  };
  await publish([consumer, provider], [value()]);
  mount();
  await screen.findByRole("button", { name: "Extension:inspected:0" });
  expect(contexts.at(-1)?.binding).toEqual({
    installationId: producer.id,
    digest: producer.selected.digest,
    contribution: "operations",
    contract: "provider-contract",
  });
  expect(parents.at(-1)?.binding).toBeUndefined();
});

it("detects recursive plugin composition and cancels a component that fails rendering", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  vi.mocked(modules.component).mockResolvedValue({
    component: ({ context }) => {
      contexts.push(context);
      return context.compose({
        contract: '{"composition":"collection"}',
        target: { scope: "installation", reference: consumer.id },
        attributes: {},
      });
    },
    signal: resources.signal,
    reload: vi.fn(async () => {}),
  });
  mount();
  await screen.findByRole("alert");
  expect(modules.component).toHaveBeenCalledOnce();
  expect(parents.at(-1)!.signal.aborted).toBe(false);
});

it("cancels failed rendering and contains the failure inside the contribution", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  vi.mocked(modules.component).mockResolvedValue({
    component: ({ context }) => {
      contexts.push(context);
      throw new Error("broken extension");
    },
    signal: resources.signal,
    reload: vi.fn(async () => {}),
  });
  mount();
  await screen.findByRole("alert");
  expect(contexts.at(-1)?.signal.aborted).toBe(true);
  expect(screen.getByText("Consumer content")).toBeTruthy();
  expect(parents.at(-1)!.signal.aborted).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "Details" }));
  const diagnostics = screen.getByText(/scope: installed component/).textContent;
  expect(diagnostics).toContain(`installation: ${producer.id}`);
  expect(diagnostics).toContain(`release: ${producer.selected.digest}`);
  expect(diagnostics).toContain("component: panel");
  expect(diagnostics).toContain(`runtime generation: ${generation.processGeneration}`);
});
