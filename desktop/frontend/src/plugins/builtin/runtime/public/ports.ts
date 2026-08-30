// The Runtime context's setup-time contract — see `agent/public/ports` for why
// only setup-time readers need one.

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

export interface RuntimeStreamPorts {
  connectionGeneration: () => RuntimeConnectionGeneration | null;
  subscribeConnection: (onChange: () => void) => () => void;
  reportConnectionLoss: (expectedGeneration: RuntimeConnectionGeneration) => Promise<void>;
}

export const RUNTIME_STREAM_PORTS = service<RuntimeStreamPorts>("flame.runtime.streamPorts");

/** A configured endpoint change replaces the product's one server scope. */
export interface RuntimeServerScopePorts {
  subscribeReplacement: (onReplace: () => void) => () => void;
}

export const RUNTIME_SERVER_SCOPE_PORTS = service<RuntimeServerScopePorts>(
  "flame.runtime.serverScopePorts",
);
