import { afterEach, describe, expect, it, vi } from "vitest";
import { createFileTransfer } from "@/features/workspace/adapters/fileTransfer";
afterEach(() => vi.restoreAllMocks());

describe("fileTransfer", () => {
  it("settles native picker cancellation as an empty selection", async () => {
    const click = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});

    const selection = createFileTransfer({ saveTextFile: vi.fn() }).pickText(
      "application/json,.json",
    );
    const observed = observe(selection);
    const picker = click.mock.instances[0] as HTMLInputElement;
    expect(picker).toBeDefined();

    picker.dispatchEvent(new Event("cancel"));
    await drainMicrotasks();
    const settledAtCancel = observed.settled();

    picker.dispatchEvent(new Event("change"));
    await expect(selection).resolves.toBeNull();
    expect(settledAtCancel).toBe(true);
    expect(observed.value()).toBeNull();
  });

  it("hands the exact text to the composed host and waits for the user's choice", async () => {
    const choice = Promise.withResolvers<boolean>();
    const saveTextFile = vi.fn(() => choice.promise);

    const saving = createFileTransfer({ saveTextFile }).saveText(
      "flame-session-trajectory.json",
      '{"title":"Résumé 🔥"}',
      "application/json;charset=utf-8",
    );
    const observed = observe(saving);
    await drainMicrotasks();

    expect(saveTextFile).toHaveBeenCalledExactlyOnceWith(
      "flame-session-trajectory.json",
      '{"title":"Résumé 🔥"}',
      "application/json;charset=utf-8",
    );
    expect(observed.settled()).toBe(false);
    choice.resolve(true);
    await expect(saving).resolves.toBeUndefined();
  });

  it("treats a cancelled save as an ordinary outcome", async () => {
    const files = createFileTransfer({ saveTextFile: vi.fn().mockResolvedValue(false) });
    await expect(files.saveText("flame.md", "# Title", "text/markdown")).resolves.toBeUndefined();
  });

  it("surfaces a failed save to its caller", async () => {
    const failure = new Error("desktop host text file save failed: disk full");
    const files = createFileTransfer({ saveTextFile: vi.fn().mockRejectedValue(failure) });
    await expect(files.saveText("flame.md", "# Title", "text/markdown")).rejects.toBe(failure);
  });
});

function observe<T>(operation: Promise<T>): {
  settled(): boolean;
  value(): T | undefined;
} {
  let settled = false;
  let value: T | undefined;
  void operation.then(
    (result) => {
      value = result;
      settled = true;
    },
    () => {
      settled = true;
    },
  );
  return {
    settled: () => settled,
    value: () => value,
  };
}

async function drainMicrotasks(): Promise<void> {
  for (let index = 0; index < 8; index++) await Promise.resolve();
}
