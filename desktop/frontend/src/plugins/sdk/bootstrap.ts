import { createHost, type AnyPlugin, type Host } from "dougong";
import { reportPluginError } from "./errors";
import { kernelLogger } from "./hostLog";
import { contributionsTo, publishKernel, retractKernel, trackInstalledPlugin } from "./kernel";
import { READY_HANDLER } from "./kernelPoints";
import { shellServices } from "./shellServices";

export function createKernel(plugins: ReadonlyArray<AnyPlugin>): Host {
  const host = createHost({
    name: "flame",
    logger: kernelLogger,
    onError: (error) => reportPluginError("kernel", "setup", error),
  });
  host.install(shellServices);
  for (const plugin of plugins) {
    host.install(plugin);
    trackInstalledPlugin(host, plugin.name);
  }
  return host;
}

export async function startKernel(
  plugins: ReadonlyArray<AnyPlugin>,
  signal?: AbortSignal,
): Promise<Host> {
  const host = createKernel(plugins);
  try {
    signal?.throwIfAborted();
    await host.start();
    signal?.throwIfAborted();
    publishKernel(host);
    fireReadyHandlers();
    return host;
  } catch (error) {
    try {
      await host.stop();
    } catch (stopError) {
      throw new AggregateError([error, stopError], "Kernel startup and rollback both failed");
    }
    throw error;
  }
}

export async function stopKernel(host: Host): Promise<void> {
  retractKernel(host);
  await host.stop();
}

function fireReadyHandlers(): void {
  for (const entry of contributionsTo(READY_HANDLER)) {
    try {
      entry.item();
    } catch (error) {
      reportPluginError(entry.plugin, "setup", error);
    }
  }
}
