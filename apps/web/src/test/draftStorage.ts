import { onTestFinished, vi } from "vitest";

export function refuseDraftStorage(operation: "read" | "save", cause: unknown) {
  const original = IDBDatabase.prototype.transaction;
  const transaction = vi.spyOn(IDBDatabase.prototype, "transaction").mockImplementation(function (
    this: IDBDatabase,
    ...args
  ) {
    if (this.name === "flame.composer" && (args[1] === "readwrite") === (operation === "save"))
      throw cause;
    return original.apply(this, args);
  });
  const restore = () => transaction.mockRestore();
  onTestFinished(restore);
  return { transaction, restore };
}
