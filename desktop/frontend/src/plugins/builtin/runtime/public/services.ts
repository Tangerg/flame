// What this context PUBLISHES to other plugins, as dougong Services — see
// `agent/public/services` for when a capability is a Service and when it is an
// `application/ports/` inversion instead.

import { service } from "dougong";

/** Process-local capability for one admitted Runtime connection.
 *
 * Identity is deliberately object identity: it is never serialized, counted,
 * or reconstructed from display text. A healthy inspection of the same
 * process keeps the same instance; reconnecting creates a successor instance.
 */
export class RuntimeConnectionGeneration {
  private constructor(readonly processGeneration: string) {}

  static forProcess(processGeneration: string): RuntimeConnectionGeneration {
    return new RuntimeConnectionGeneration(processGeneration);
  }

  belongsTo(processGeneration: string): boolean {
    return this.processGeneration === processGeneration;
  }
}

export interface RuntimeStream {
  connectionGeneration: () => RuntimeConnectionGeneration | null;
  subscribeConnection: (onChange: () => void) => () => void;
  reportConnectionLoss: (expectedGeneration: RuntimeConnectionGeneration) => Promise<void>;
}

export const RUNTIME_STREAM = service<RuntimeStream>("flame.runtime.stream");

/**
 * Calls `onAdvance` only when the generation actually changed.
 *
 * `subscribeConnection` fires on connection activity, not only on replacement, so acting on
 * every notification retires in-flight mutations against a generation that never moved.
 */
export function followRuntimeGeneration(
  ports: RuntimeStream,
  onAdvance: (generation: RuntimeConnectionGeneration | null) => void,
): () => void {
  let current = ports.connectionGeneration();
  return ports.subscribeConnection(() => {
    const next = ports.connectionGeneration();
    if (next === current) return;
    current = next;
    onAdvance(next);
  });
}

/** A configured endpoint change replaces the product's one server scope. */
export interface RuntimeServerScope {
  subscribeReplacement: (onReplace: () => void) => () => void;
}

export const RUNTIME_SERVER_SCOPE = service<RuntimeServerScope>("flame.runtime.serverScope");
