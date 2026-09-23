import { service } from "dougong";

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
  reportConnectionLoss: (expectedGeneration: RuntimeConnectionGeneration) => void;
}

export const RUNTIME_STREAM = service<RuntimeStream>("flame.runtime.stream");

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

export interface RuntimeServerScope {
  subscribeReplacement: (onReplace: () => void) => () => void;
}

export const RUNTIME_SERVER_SCOPE = service<RuntimeServerScope>("flame.runtime.serverScope");
