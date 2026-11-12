import type { Database } from "@flame/backend-sdk/sqlite";

export function synchronousTransactions(database: Database): void {
  const result: string = database.transaction(() => "accepted");
  void result;
  // @ts-expect-error A promise can outlive the transaction's commit boundary.
  database.transaction(async () => "uncommitted");
  // @ts-expect-error Returning a promise also escapes from a non-async callback.
  database.transaction(() => Promise.resolve("uncommitted"));
}
