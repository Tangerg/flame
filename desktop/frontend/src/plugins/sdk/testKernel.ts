import type { AnyPlugin, Host } from "dougong";
import { definePlugin, type PluginContext } from "./definePlugin";
import { startKernel, stopKernel } from "./bootstrap";

let running: Host | undefined;

export async function loadPluginsForTest(...plugins: AnyPlugin[]): Promise<Host> {
  if (running) {
    await addPluginsForTest(running, plugins);
    return running;
  }
  running = await startKernel(plugins);
  return running;
}

export async function addPluginsForTest(
  host: Host,
  plugins: ReadonlyArray<AnyPlugin>,
): Promise<void> {
  if (!plugins.length) return;
  const change = host.change();
  for (const plugin of plugins) change.install(plugin);
  await change.commit();
}

export async function contributeForTest(
  setup: (ctx: PluginContext) => void,
  name = "test.contributor",
): Promise<Host> {
  return loadPluginsForTest(definePlugin({ name, setup }));
}

export async function resetKernelForTest(): Promise<void> {
  if (!running) return;
  const host = running;
  running = undefined;
  await stopKernel(host);
}
