export class MutationJournalError extends Error {
  override readonly name: string = "MutationJournalError";
}

export class MutationJournalStorageError extends MutationJournalError {
  override readonly name = "MutationJournalStorageError";
}

export type MutationConfirmation<T> =
  | { readonly type: "accepted"; readonly result: T }
  | { readonly type: "refused"; readonly error: unknown };

export class MutationJournalCleanupError<T = unknown> extends MutationJournalError {
  override readonly name = "MutationJournalCleanupError";
  readonly #confirmation: MutationConfirmation<T>;
  readonly #retryCleanup: () => void;

  constructor(confirmation: MutationConfirmation<T>, retryCleanup: () => void, cause: unknown) {
    super(`Runtime ${confirmation.type} the command and journal cleanup failed`, { cause });
    this.#confirmation = confirmation;
    this.#retryCleanup = retryCleanup;
  }

  get confirmation(): MutationConfirmation<T> {
    return this.#confirmation;
  }

  retryCleanup(): void {
    this.#retryCleanup();
  }
}

export class MutationJournalOwnershipError extends MutationJournalError {
  override readonly name = "MutationJournalOwnershipError";
}

export class MutationJournalScopeUnavailableError extends MutationJournalError {
  override readonly name = "MutationJournalScopeUnavailableError";
}

export class MutationJournalCapacityError extends MutationJournalError {
  override readonly name = "MutationJournalCapacityError";
}
