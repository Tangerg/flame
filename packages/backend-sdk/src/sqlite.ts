import type { DatabaseSync, StatementSync } from "node:sqlite";

export class Database {
  readonly #connection: DatabaseSync;
  #failure?: AggregateError;

  constructor(connection: DatabaseSync) {
    if (!connection.isOpen || connection.isTransaction)
      throw new Error(
        "plugin: private database requires an idle open connection",
      );
    this.#connection = connection;
  }

  #available(): DatabaseSync {
    if (this.#failure)
      throw new Error("plugin: private database is fenced", {
        cause: this.#failure,
      });
    if (!this.#connection.isOpen)
      throw new Error("plugin: private database is closed");
    return this.#connection;
  }

  prepare(sql: string): StatementSync {
    return this.#available().prepare(sql);
  }

  get isTransaction(): boolean {
    return this.#available().isTransaction;
  }

  transaction<T>(action: () => T extends PromiseLike<unknown> ? never : T): T {
    const connection = this.#available();
    connection.exec("BEGIN IMMEDIATE");
    try {
      const result = action();
      connection.exec("COMMIT");
      return result;
    } catch (cause) {
      try {
        connection.exec("ROLLBACK");
      } catch (rollback) {
        // An uncertain transaction cannot supply receipts or read projections.
        // Fence admission before cleanup, which can itself fail and be retried.
        const failures = [cause, rollback];
        this.#failure = new AggregateError(
          failures,
          "plugin: private database rollback is unconfirmed",
        );
        try {
          this.close();
        } catch (close) {
          failures.push(close);
          this.#failure = new AggregateError(
            failures,
            "plugin: private database rollback and close are unconfirmed",
          );
        }
        throw this.#failure;
      }
      throw cause;
    }
  }

  close(): void {
    if (this.#connection.isOpen) this.#connection.close();
  }
}
