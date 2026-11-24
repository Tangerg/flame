import { DatabaseSync, type StatementSync } from "node:sqlite";

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

export interface PrivateDatabaseSchema {
  // Each owned schema object's exact CREATE statement, keyed by object name.
  readonly objects: Readonly<Record<string, string>>;
  readonly pragmas?: readonly string[];
}

// Opens a SQLite file that one plugin process owns alone. EXCLUSIVE keeps the
// file lock between transactions, so admission survives a lost callback without
// holding a transaction over an await. An empty file receives the schema; any
// other file must already carry exactly that schema.
export function openPrivateDatabase(
  path: string,
  owner: string,
  { objects, pragmas = [] }: PrivateDatabaseSchema,
): Database {
  const db = new DatabaseSync(path, {
    allowExtension: false,
    enableDoubleQuotedStringLiterals: false,
    readBigInts: true,
    timeout: 0,
  });
  try {
    db.exec(
      [
        ...pragmas,
        "PRAGMA locking_mode=EXCLUSIVE",
        "PRAGMA synchronous=FULL",
        "BEGIN EXCLUSIVE",
      ].join("; "),
    );
    const stored = db
      .prepare(
        "SELECT name,sql FROM sqlite_schema WHERE sql IS NOT NULL AND name NOT GLOB 'sqlite_*'",
      )
      .all();
    if (stored.length) {
      if (
        stored.length !== Object.keys(objects).length ||
        stored.some(
          (row) =>
            typeof row.name !== "string" ||
            !Object.hasOwn(objects, row.name) ||
            row.sql !== objects[row.name],
        )
      )
        throw new Error(
          `${owner}: incompatible private database schema; preserve the existing data directory`,
        );
    } else for (const sql of Object.values(objects)) db.exec(sql);
    db.exec("COMMIT");
  } catch (cause) {
    const failures = [cause];
    try {
      if (db.isTransaction) db.exec("ROLLBACK");
    } catch (rollback) {
      failures.push(rollback);
    }
    try {
      db.close();
    } catch (close) {
      failures.push(close);
    }
    if (failures.length === 1) throw cause;
    throw new AggregateError(
      failures,
      `${owner}: private database startup cleanup failed`,
    );
  }
  return new Database(db);
}
