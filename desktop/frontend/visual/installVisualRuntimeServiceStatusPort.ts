import {
  configureRuntimeServiceStatusPort,
  type RuntimeServiceSnapshot,
} from "@/plugins/builtin/runtime/application/ports/serviceStatus";

export function installVisualRuntimeServiceStatusPort(
  phase: RuntimeServiceSnapshot["phase"] = "ready",
): void {
  const snapshot = {
    phase,
    observation: {
      server: { name: "flame-runtime", version: "0.0.0-visual" },
      protocolVersion: "2",
      health: "ready",
      checks: {},
    },
    failure: null,
  } satisfies RuntimeServiceSnapshot;

  configureRuntimeServiceStatusPort({
    useSnapshot: () => snapshot,
    snapshot: () => snapshot,
    refresh: () => Promise.resolve(),
  });
}
