import {
  configureRuntimeServiceStatusPort,
  type RuntimeServiceSnapshot,
} from "@/plugins/builtin/runtime/application/ports/serviceStatus";

export function installVisualRuntimeServiceStatusPort(): void {
  const snapshot = {
    phase: "ready",
    observation: {
      server: { name: "flame-runtime", version: "0.0.0-visual" },
      protocolVersion: "2",
      health: "ready",
      checks: {},
    },
    failure: null,
  } as const satisfies RuntimeServiceSnapshot;

  configureRuntimeServiceStatusPort({
    useSnapshot: () => snapshot,
    snapshot: () => snapshot,
    refresh: () => Promise.resolve(),
  });
}
