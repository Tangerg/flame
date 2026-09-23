import { createSingletonPort } from "@/lib/ports/singletonPort";

export interface RuntimeMutationJournalStorage {
  get(key: string): unknown;
  set(key: string, value: unknown): void;
  remove(key: string): void;
  keys(): string[];
}

const port = createSingletonPort<RuntimeMutationJournalStorage>(
  "Runtime mutation journal storage is not installed",
);

export const configureRuntimeMutationJournalStorage = port.configure;
export const installedRuntimeMutationJournalStorage = port.peek;
