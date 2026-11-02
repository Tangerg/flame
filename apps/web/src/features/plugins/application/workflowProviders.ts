import { tupleKey } from "@/lib/tupleKey";

export interface WorkflowAuthor {
  readonly installationId: string;
  readonly release: string;
  readonly contribution: string;
}

interface AvailableAuthor {
  readonly source: WorkflowAuthor;
  readonly roles: readonly string[];
}

interface ProviderSelection {
  readonly role: string;
  readonly installationId: string;
  readonly contribution: string;
}

export { GOAL_WORKFLOW } from "@flame/runtime-client/wire";

function sourceKey(source: WorkflowAuthor) {
  return tupleKey(source.installationId, source.release, source.contribution);
}

export class WorkflowProviders {
  #roles = new Map<string, Set<string>>();
  #selections = new Map<string, string>();
  #listeners = new Set<() => void>();

  readonly subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => {
      this.#listeners.delete(listener);
    };
  };

  selected(role: string) {
    return this.#selections.get(role);
  }

  admits(role: string, source: WorkflowAuthor | undefined) {
    return source !== undefined && this.#roles.get(role)?.has(sourceKey(source)) === true;
  }

  publish(authors: readonly AvailableAuthor[], selections: readonly ProviderSelection[]) {
    const roles = new Map<string, Set<string>>();
    const selected = new Map<string, string>();
    for (const { source, roles: declared } of authors) {
      const key = sourceKey(source);
      for (const role of declared) {
        const candidates = roles.get(role) ?? new Set<string>();
        candidates.add(key);
        roles.set(role, candidates);
        if (
          selections.some(
            (choice) =>
              choice.role === role &&
              choice.installationId === source.installationId &&
              choice.contribution === source.contribution,
          )
        )
          selected.set(role, key);
      }
    }
    this.#roles = roles;
    this.#selections = selected;
    for (const listener of this.#listeners) listener();
  }
}
