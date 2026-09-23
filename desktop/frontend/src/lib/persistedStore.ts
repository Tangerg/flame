import type { ZodType } from "zod";

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
