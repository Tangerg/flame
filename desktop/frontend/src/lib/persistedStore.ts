import type { ZodType } from "zod";
import type { StateStorage } from "zustand/middleware";

interface ScopedStore<State> {
  getState(): State;
  setState(state: Partial<State>): void;
  persist: {
    setOptions(options: { name: string }): void;
    rehydrate(): void | Promise<void>;
  };
}

// Zustand persists every set, including hydration and scope reset. Suspend
// those writes while replacing the active projection so neither bucket can be
// overwritten by the other target's state. In-memory snapshots also retain
// material, such as images, which the owning codec intentionally does not save.
export class ScopedPersistence<State> {
  readonly #snapshots = new Map<string, State>();
  #scope: string | null = null;
  #restoring = false;

  constructor(private readonly name: string) {}

  readonly storage: StateStorage = {
    getItem: (name) => (this.#scope === null ? null : localStorage.getItem(name)),
    setItem: (name, value) => {
      if (this.#scope !== null && !this.#restoring) localStorage.setItem(name, value);
    },
    removeItem: (name) => localStorage.removeItem(name),
  };

  isActive(scope: string): boolean {
    return this.#scope === scope;
  }

  activate(scope: string, store: ScopedStore<State>): boolean {
    if (this.isActive(scope)) return false;
    if (this.#scope === null) localStorage.removeItem(this.name);
    else this.#snapshots.set(this.#scope, store.getState());
    this.#scope = scope;
    this.#restoring = true;
    store.persist.setOptions({ name: `${this.name}:${encodeURIComponent(scope)}` });
    try {
      const retained = this.#snapshots.get(scope);
      this.#snapshots.delete(scope);
      if (retained) store.setState(retained);
      else void store.persist.rehydrate();
    } finally {
      this.#restoring = false;
    }
    store.setState({});
    return true;
  }
}

export const discardOlderVersions = () => undefined as never;

export function rehydrateOrDefault<Persisted, Restored extends object = Persisted & object>(
  storageKey: string,
  schema: ZodType<Persisted>,
  project: (persisted: Persisted) => Restored = (persisted) => persisted as unknown as Restored,
) {
  return <State extends Restored>(persisted: unknown, current: State): State => {
    if (persisted === undefined) return current;
    const parsed = schema.safeParse(persisted);
    if (!parsed.success) {
      console.warn(`[${storageKey}] discarding corrupted payload:`, parsed.error.issues);
      return current;
    }
    return { ...current, ...project(parsed.data) };
  };
}

type DataKeys<State> = {
  [K in keyof State]-?: State[K] extends (...args: never[]) => unknown ? never : K;
}[keyof State];

export type Paired<State, Persisted> = [DataKeys<State>] extends [keyof Persisted]
  ? [keyof Persisted] extends [DataKeys<State>]
    ? true
    : { schemaNamesWhatTheStateHasNot: Exclude<keyof Persisted, DataKeys<State>> }
  : { stateHasWhatTheSchemaWillStrip: Exclude<DataKeys<State>, keyof Persisted> };
