import { DatabaseSync } from "node:sqlite";
import { afterEach, expect, it, vi } from "vitest";
import { Database } from "./sqlite.ts";

const connections: DatabaseSync[] = [];
afterEach(() => {
  vi.restoreAllMocks();
  for (const connection of connections.splice(0))
    if (connection.isOpen) connection.close();
});

function fixture() {
  const connection = new DatabaseSync(":memory:");
  connections.push(connection);
  connection.exec("CREATE TABLE values_table (value TEXT NOT NULL)");
  const database = new Database(connection);
  return { connection, database };
}

it("commits synchronous decisions and rolls back a failed mutation without losing its cause", () => {
  const { database } = fixture();
  expect(
    database.transaction(() => {
      expect(database.isTransaction).toBe(true);
      database.prepare("INSERT INTO values_table VALUES (?)").run("original");
      return "accepted";
    }),
  ).toBe("accepted");
  const failure = new Error("mutation failed");
  expect(() =>
    database.transaction(() => {
      database.prepare("DELETE FROM values_table").run();
      throw failure;
    }),
  ).toThrow(failure);
  expect(database.prepare("SELECT * FROM values_table").all()).toEqual([
    { value: "original" },
  ]);
  expect(database.isTransaction).toBe(false);
  database.close();
  database.close();
  expect(() => database.prepare("SELECT 1")).toThrow("closed");
  expect(() => database.transaction(() => true)).toThrow("closed");
});

it.each([false, true])(
  "fences all new statements and transactions after failed rollback (close fails=%s)",
  (closeFails) => {
    const { connection, database } = fixture();
    const failure = new Error("commit failed");
    const rollback = new Error("rollback failed");
    const close = new Error("close failed");
    const execute = connection.exec.bind(connection);
    vi.spyOn(connection, "exec").mockImplementation((sql) => {
      if (sql === "COMMIT") throw failure;
      if (sql === "ROLLBACK") throw rollback;
      return execute(sql);
    });
    if (closeFails)
      vi.spyOn(connection, "close").mockImplementationOnce(() => {
        throw close;
      });
    let received: unknown;
    try {
      database.transaction(() =>
        database
          .prepare("INSERT INTO values_table VALUES (?)")
          .run("uncertain"),
      );
    } catch (error) {
      received = error;
    }
    expect(received).toBeInstanceOf(AggregateError);
    expect((received as AggregateError).errors).toEqual(
      closeFails ? [failure, rollback, close] : [failure, rollback],
    );
    expect(connection.isOpen).toBe(closeFails);
    expect(() => database.prepare("SELECT * FROM values_table")).toThrow(
      expect.objectContaining({ cause: received }),
    );
    expect(() => database.transaction(() => true)).toThrow("fenced");
    expect(() => database.isTransaction).toThrow("fenced");
    database.close();
    database.close();
    expect(connection.isOpen).toBe(false);
  },
);

it("requires original idle connection ownership", () => {
  const { connection } = fixture();
  connection.exec("BEGIN");
  expect(() => new Database(connection)).toThrow("idle open connection");
  connection.exec("ROLLBACK");
  connection.close();
  expect(() => new Database(connection)).toThrow("idle open connection");
});
