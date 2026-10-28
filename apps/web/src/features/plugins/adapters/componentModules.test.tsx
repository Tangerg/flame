import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { createElement, useEffect, type ComponentProps } from "react";
import { type Host } from "dougong";
import { type FlameClient, type MutationPromise } from "@flame/runtime-client";
import {
  type PluginBrowserSession,
  type PluginClientComponent,
  type PluginInstallation,
} from "@flame/runtime-client/wire";
import { type ComponentContext, type PluginComponent } from "@flame/component-sdk";
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
  renderComponent: (
    props: ComponentProps<PluginComponent>,
  ) => ReturnType<typeof createElement> = () => createElement("p", null, "Native panel"),
  resourcesReady: Promise<void> = Promise.resolve(),
) {
  const modules = new ComponentModules();
  owners.push(modules);
  const globalName = `qimuModule${crypto.randomUUID().replaceAll("-", "")}`;
  const source = { evaluations: 0, render: renderComponent };
  vi.stubGlobal(globalName, source);
  const endpoint = `data:text/javascript,${encodeURIComponent(`globalThis["${globalName}"].evaluations++; export function Panel(props) { return globalThis["${globalName}"].render(props); }`)}#`;
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
  await screen.findByText("Working panel");
  expect(tracked).toBe(work.promise);
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
  resourcesReady.resolve();
  await replacement;
  await screen.findByText("Native panel");
  expect(f.source.evaluations).toBe(1);
  scene.unmount();
});
