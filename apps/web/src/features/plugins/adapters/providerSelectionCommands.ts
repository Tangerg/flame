import { mutationSettlementIsUnknown, type MutationPromise } from "@flame/runtime-client";
import type {
  PluginProviderSelection,
  SelectPluginProviderRequest,
} from "@flame/runtime-client/wire";

type SelectionMutation = MutationPromise<PluginProviderSelection>;
type UnconfirmedSelection = Readonly<{
  kind: "unconfirmed";
  idempotencyKey: string;
  error: unknown;
}>;
type SelectionObservation =
  | { readonly view: Readonly<{ kind: "observing" }> }
  | { readonly view: Readonly<{ kind: "failed"; error: unknown }> }
  | { readonly view: UnconfirmedSelection; readonly original: SelectionMutation };

interface ProviderSelectionOperations {
  readonly signal: AbortSignal;
  select(request: SelectPluginProviderRequest): SelectionMutation;
  retry(original: SelectionMutation): SelectionMutation;
  refreshCatalog(): void;
}

export class ProviderSelectionCommands {
  readonly #roles = new Map<string, SelectionObservation>();
  readonly #listeners = new Set<() => void>();

  constructor(private readonly operations: ProviderSelectionOperations) {}

  snapshot(role: string): SelectionObservation["view"] | undefined {
    return this.#roles.get(role)?.view;
  }

  subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  select(request: SelectPluginProviderRequest): Promise<boolean> {
    const current = this.#roles.get(request.role);
    if (current && current.view.kind !== "failed") return Promise.resolve(false);
    return this.#observe(request.role, () => this.operations.select(request));
  }

  check(role: string, displayed: UnconfirmedSelection): Promise<boolean> {
    const current = this.#roles.get(role);
    if (!current || !("original" in current) || current.view !== displayed)
      return Promise.resolve(false);
    return this.#observe(role, () => this.operations.retry(current.original));
  }

  async #observe(role: string, execute: () => SelectionMutation): Promise<boolean> {
    if (this.operations.signal.aborted) return false;
    this.#publish(role, { view: { kind: "observing" } });
    let original: SelectionMutation | undefined;
    try {
      original = execute();
      await original;
    } catch (error) {
      if (this.operations.signal.aborted) return false;
      if (original && mutationSettlementIsUnknown(error)) {
        this.#publish(role, {
          view: { kind: "unconfirmed", idempotencyKey: original.idempotencyKey, error },
          original,
        });
      } else {
        this.#publish(role, { view: { kind: "failed", error } });
      }
      return false;
    }
    if (this.operations.signal.aborted) return false;
    this.#publish(role, undefined);
    this.operations.refreshCatalog();
    return true;
  }

  #publish(role: string, observation: SelectionObservation | undefined): void {
    if (observation === undefined) this.#roles.delete(role);
    else this.#roles.set(role, observation);
    for (const listener of this.#listeners) listener();
  }
}
