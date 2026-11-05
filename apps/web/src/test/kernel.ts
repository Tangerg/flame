import { shellServices } from "@/app/shellServices";
import { type AnyPlugin, type Host } from "dougong";
import { definePlugin, type PluginContext } from "@/extensions/definePlugin";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
let running: Host | undefined;

export async function loadPluginsForTest(...plugins: AnyPlugin[]): Promise<Host> {
  if (running) {
    await addPluginsForTest(running, plugins);
    return running;
  }
  running = await startKernel([shellServices, ...plugins]);
  return running;
}

export async function addPluginsForTest(
  host: Host,
  plugins: ReadonlyArray<AnyPlugin>,
): Promise<void> {
  if (!plugins.length) return;
  const change = host.change();
  for (const plugin of plugins) change.install(plugin, { id: crypto.randomUUID() });
  await host.apply(await host.plan(change));
}

export async function contributeForTest(
  setup: (ctx: PluginContext) => void,
  name = "test.contributor",
): Promise<Host> {
  return loadPluginsForTest(
    definePlugin({
      name,
      setup(ctx) {
        setup(ctx);
      },
    }),
  );
}

export async function resetKernelForTest(): Promise<void> {
  if (!running) return;
  const host = running;
  running = undefined;
  await stopKernel(host);
}
