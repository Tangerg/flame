export class MutationJournalError extends Error {
  override readonly name: string = "MutationJournalError";
}

export class MutationJournalStorageError extends MutationJournalError {
  override readonly name = "MutationJournalStorageError";
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
