import { type VisualClient } from "./client";
import { type RuntimeServiceSnapshot } from "@/features/connection/application/runtimeService";
export function installVisualRuntimeServiceStatusPort(
  visual: VisualClient,
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

  visual.projection.store.setState({ service: snapshot });
}
