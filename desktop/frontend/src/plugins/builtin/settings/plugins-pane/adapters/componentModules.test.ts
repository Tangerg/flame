import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { asyncDisposeSymbol, type Host } from "dougong";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { PluginInstallation } from "@flame/runtime-contract/wire";
import { definePlugin, startKernel, stopKernel } from "@/plugins/sdk";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { ComponentModules } from "./componentModules";

type Resources = Awaited<ReturnType<FlameClient["plugins"]["openResources"]>>;
let host: Host;
let scope: ContributionLifetime;
const openResources = vi.fn<FlameClient["plugins"]["openResources"]>();
const client = { plugins: { openResources } } as unknown as FlameClient;
const entry = {
  id: "panel",
  title: "Panel",
  scope: "installation",
  placement: "export",
  entry: "component.js",
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
    components: [entry],
  },
};

function endpoint(source = "export const Panel = () => null;") {
  return `data:text/javascript,${encodeURIComponent(source)}#`;
}

function resources(failure?: Error): Resources {
  const ended = Promise.withResolvers<void>();
  return {
    result: { token: crypto.randomUUID(), entry: "component.js" },
    closed: ended.promise,
    close: vi.fn(async () => {
      ended.resolve();
      if (failure) throw failure;
    }),
  };
}

beforeEach(async () => {
  vi.spyOn(HTMLScriptElement, "supports").mockReturnValue(true);
  host = await startKernel([
    definePlugin({
      name: "test.component-modules",
      setup(ctx) {
        scope = ctx.lifetime("modules");
      },
    }),
  ]);
});

afterEach(async () => {
  await stopKernel(host);
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  openResources.mockReset();
});

it("shares one exact module acquisition and keeps staging outside evaluation identity", async () => {
  const lease = resources();
  openResources.mockResolvedValue(lease);
  const modules = new ComponentModules();
  const generation = {};
  const staged = { ...installation, staged: { ...installation.selected, digest: "c".repeat(64) } };
  const [first, second] = await Promise.all([
    modules.component(scope, client, endpoint(), generation, installation, entry),
    modules.component(scope, client, endpoint(), generation, staged, entry),
  ]);
  expect(first).toEqual(second);
  expect(openResources).toHaveBeenCalledOnce();
  expect(first.signal.aborted).toBe(false);
  await scope[asyncDisposeSymbol]();
  expect(first.signal.aborted).toBe(true);
  expect(lease.close).toHaveBeenCalledOnce();
});

it("requires reload for a different evaluated release or generation", async () => {
  openResources.mockResolvedValue(resources());
  const modules = new ComponentModules();
  const generation = {};
  await modules.component(scope, client, endpoint(), generation, installation, entry);
  const replacement = {
    ...installation,
    selected: { ...installation.selected, digest: "b".repeat(64) },
  };
  await expect(
    modules.component(scope, client, endpoint(), generation, replacement, entry),
  ).rejects.toMatchObject({
    name: "ModuleReloadRequired",
    message: expect.stringContaining(
      `The evaluated release ${installation.selected.digest} was replaced by ${replacement.selected.digest}.`,
    ),
  });
  await expect(
    modules.component(scope, client, endpoint(), {}, installation, entry),
  ).rejects.toMatchObject({
    name: "ModuleReloadRequired",
    message: expect.stringContaining("The Runtime connection generation changed."),
  });
  expect(openResources).toHaveBeenCalledOnce();
});

it("withdraws evaluated exports when the resource subscription ends unexpectedly", async () => {
  const lease = resources();
  const ended = Promise.withResolvers<void>();
  openResources.mockResolvedValue({ ...lease, closed: ended.promise });
  const modules = new ComponentModules();
  const generation = {};
  const loaded = await modules.component(
    scope,
    client,
    endpoint(),
    generation,
    installation,
    entry,
  );
  ended.resolve();
  await vi.waitFor(() => expect(loaded.signal.aborted).toBe(true));
  expect(lease.close).toHaveBeenCalledOnce();
  await expect(
    modules.component(scope, client, endpoint(), generation, installation, entry),
  ).rejects.toMatchObject({ name: "ModuleReloadRequired" });
});

it("retires resources without joining window-owned evaluation and refuses late exports", async () => {
  const gate = Promise.withResolvers<void>();
  const started = Promise.withResolvers<void>();
  const key = `flameEvaluation${crypto.randomUUID().replaceAll("-", "")}`;
  vi.stubGlobal(key, { ready: started.resolve, resume: gate.promise });
  const lease = resources();
  openResources.mockResolvedValue(lease);
  const loading = new ComponentModules().component(
    scope,
    client,
    endpoint(
      `globalThis["${key}"].ready(); await globalThis["${key}"].resume; export const Panel = () => null;`,
    ),
    {},
    installation,
    entry,
  );
  const outcome = loading.catch((error: unknown) => error);
  try {
    await started.promise;
    await scope[asyncDisposeSymbol]();
    expect(lease.close).toHaveBeenCalledOnce();
  } finally {
    gate.resolve();
    await outcome;
  }
  expect(await outcome).toBe(scope.signal.reason);
});

it("allows a fresh acquisition after failure before ESM evaluation", async () => {
  const modules = new ComponentModules();
  const generation = {};
  const first = new Error("resource transport unavailable");
  const second = new Error("new acquisition evidence");
  openResources.mockRejectedValueOnce(first).mockRejectedValueOnce(second);
  await expect(
    modules.component(scope, client, endpoint(), generation, installation, entry),
  ).rejects.toBe(first);
  await expect(
    modules.component(scope, client, endpoint(), generation, installation, entry),
  ).rejects.toBe(second);
  expect(openResources).toHaveBeenCalledTimes(2);
});

it("joins acquisition and closes a late lease when its generation retires", async () => {
  const pending = Promise.withResolvers<Resources>();
  openResources.mockReturnValue(pending.promise);
  const modules = new ComponentModules();
  const loading = modules.component(scope, client, endpoint(), {}, installation, entry);
  const result = loading.catch((error: unknown) => error);
  const lease = resources();
  let retired = false;
  const retiring = scope[asyncDisposeSymbol]().then(() => {
    retired = true;
  });
  try {
    await Promise.resolve();
    await Promise.resolve();
    expect(retired).toBe(false);
  } finally {
    pending.resolve(lease);
    await result;
    await retiring;
  }
  expect(lease.close).toHaveBeenCalledOnce();
  expect(await result).toBe(scope.signal.reason);
});

it("does not reuse a pending acquisition from a retired generation", async () => {
  const pending = Promise.withResolvers<Resources>();
  const lease = resources();
  const nextLease = resources();
  openResources.mockReturnValueOnce(pending.promise).mockResolvedValueOnce(nextLease);
  const modules = new ComponentModules();
  const old = modules
    .component(scope, client, endpoint(), {}, installation, entry)
    .catch((error: unknown) => error);
  const retiring = scope[asyncDisposeSymbol]();
  let nextScope!: ContributionLifetime;
  const nextHost = await startKernel([
    definePlugin({
      name: "test.next-modules",
      setup(ctx) {
        nextScope = ctx.lifetime("next");
      },
    }),
  ]);
  const nextGeneration = {};
  try {
    const next = modules.component(
      nextScope,
      client,
      endpoint(),
      nextGeneration,
      installation,
      entry,
    );
    pending.resolve(lease);
    await retiring;
    expect((await next).component).toBeTypeOf("function");
    expect(await old).toBe(scope.signal.reason);
    expect(lease.close).toHaveBeenCalledOnce();
    expect(nextLease.close).not.toHaveBeenCalled();
    expect(
      await modules.component(nextScope, client, endpoint(), nextGeneration, installation, entry),
    ).toEqual(await next);
    expect(openResources).toHaveBeenCalledTimes(2);
  } finally {
    pending.resolve(lease);
    await old;
    await stopKernel(nextHost);
  }
});

it("fences overlapping releases at evaluation after asynchronous acquisition", async () => {
  const pending = Promise.withResolvers<Resources>();
  const originalLease = resources();
  const replacementLease = resources();
  openResources.mockReturnValueOnce(pending.promise).mockResolvedValueOnce(replacementLease);
  const modules = new ComponentModules();
  const generation = {};
  const old = modules
    .component(scope, client, endpoint(), generation, installation, entry)
    .catch((error: unknown) => error);
  const replacement = {
    ...installation,
    selected: { ...installation.selected, digest: "b".repeat(64) },
  };
  try {
    expect(
      (await modules.component(scope, client, endpoint(), generation, replacement, entry))
        .component,
    ).toBeTypeOf("function");
  } finally {
    pending.resolve(originalLease);
  }
  expect(await old).toMatchObject({ name: "ModuleReloadRequired" });
  expect(originalLease.close).toHaveBeenCalledOnce();
});

it("preserves evaluation and cleanup failures together", async () => {
  const closing = new Error("resource close failed");
  openResources.mockResolvedValue(resources(closing));
  const modules = new ComponentModules();
  const generation = {};
  const failure = await modules
    .component(
      scope,
      client,
      endpoint('throw new Error("publisher evaluation failed");'),
      generation,
      installation,
      entry,
    )
    .catch((error: unknown) => error);
  expect(failure).toMatchObject({ cause: expect.any(AggregateError) });
  expect((failure as Error & { cause: AggregateError }).cause.errors).toEqual([
    expect.objectContaining({ message: "publisher evaluation failed" }),
    closing,
  ]);
  await expect(
    modules.component(scope, client, endpoint(), generation, installation, entry),
  ).rejects.toBe(failure);
  expect(openResources).toHaveBeenCalledOnce();
});
