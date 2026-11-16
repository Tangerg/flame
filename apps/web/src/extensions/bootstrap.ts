import {
  DougongError,
  isCancellationReason,
  createResourceAuthority,
  createHost,
  type AnyPlugin,
  type Host,
} from "dougong";
import { reportPluginError } from "@/extensions/errors";
import { kernelLogger } from "@/extensions/hostLog";
import { contributionsTo, publishKernel, retractKernel } from "@/extensions/kernel";
import { READY_HANDLER } from "@/extensions/lifecycle";
const resourceAuthority = createResourceAuthority();

export function createKernel(plugins: ReadonlyArray<AnyPlugin>): Host {
  const host = createHost({
    resourceAuthority,
    name: "flame",
    logger: kernelLogger,
    onError: (error) => reportPluginError("kernel", "setup", error),
    installations: plugins.map((plugin) => ({ id: plugin.name, plugin })),
  });
  return host;
}

export async function startKernel(
  plugins: ReadonlyArray<AnyPlugin>,
  signal?: AbortSignal,
): Promise<Host> {
  const host = createKernel(plugins);
  try {
    signal?.throwIfAborted();
    await host.start(signal);
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
    if (
      signal &&
      error instanceof DougongError &&
      error.details.outcome === "restored" &&
      isCancellationReason(signal, error.cause)
    ) {
      throw signal.reason;
    }
    throw error;
  }
}

export async function stopKernel(host: Host): Promise<void> {
  await retractKernel(host);
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
